package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/edu-ricardo/selfconvertercomic/internal/identify"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
)

func (d Deps) registerMetadata(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/files/{id}/candidates", d.fileCandidates)
	mux.HandleFunc("POST /api/files/{id}/identify", d.identifyFile)
	mux.HandleFunc("POST /api/files/{id}/metadata", d.applyMetadata)
	mux.HandleFunc("PUT /api/files/{id}/metadata", d.manualMetadata)
	mux.HandleFunc("POST /api/libraries/{id}/identify", d.identifyLibrary)
	mux.HandleFunc("POST /api/catalog/series/identify", d.identifySeries)
	mux.HandleFunc("POST /api/metadata/comicvine/test", d.testComicVine)
}

// fileCandidates procura nas fontes a edição do arquivo: pela série, número e ano identificados, ou
// pelo que vier em ?series=&number=&year= (busca manual).
func (d Deps) fileCandidates(w http.ResponseWriter, r *http.Request) {
	f, _, ok := d.lookupFile(w, r)
	if !ok {
		return
	}
	g := metadata.Guess{Series: f.SeriesName, Volume: f.Volume, Number: f.Number, Year: f.Year}
	if q := r.URL.Query(); q.Has("series") {
		g = metadata.Guess{Series: strings.TrimSpace(q.Get("series")), Number: strings.TrimLeft(strings.TrimSpace(q.Get("number")), "#0")}
		if g.Number == "" && strings.Trim(q.Get("number"), " #") != "" {
			g.Number = "0"
		}
		g.Year, _ = strconv.Atoi(q.Get("year"))
		if g.Series == "" {
			writeError(w, http.StatusBadRequest, "informe o nome da série")
			return
		}
	}
	res, err := d.Metadata.Search(r.Context(), g)
	switch {
	case errors.Is(err, metadata.ErrNoProvider):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"query": g, "candidates": res.Candidates, "errors": res.Errors})
	}
}

// lookupEdition carrega o arquivo e exige que ele já tenha edição (a varredura liga os dois).
func (d Deps) lookupEdition(w http.ResponseWriter, r *http.Request) (int64, bool) {
	f, _, ok := d.lookupFile(w, r)
	if !ok {
		return 0, false
	}
	if f.EditionID == 0 {
		writeError(w, http.StatusConflict, "arquivo ainda não analisado; aguarde a próxima varredura")
		return 0, false
	}
	return f.EditionID, true
}

func (d Deps) identifyFile(w http.ResponseWriter, r *http.Request) {
	id, ok := d.lookupEdition(w, r)
	if !ok {
		return
	}
	if err := d.Identifier.QueueEdition(r.Context(), id); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

// applyMetadata aplica a edição escolhida pelo usuário e fecha a tarefa de confirmação. Aceita:
//
//	{"provider", "issueId", "score"}   candidato da lista
//	{"provider", "seriesId", "number"} candidato que é só a série + o número da edição
//	{"url", "number"}                  link da edição (ou da série + número) no ComicVine ou no GCD
func (d Deps) applyMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := d.lookupEdition(w, r)
	if !ok {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		IssueID  string `json:"issueId"`
		SeriesID string `json:"seriesId"`
		Number   string `json:"number"`
		URL      string `json:"url"`
		Score    int    `json:"score"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.URL != "" {
		link, err := metadata.ParseLink(body.URL)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		body.Provider, body.IssueID, body.SeriesID, body.Score = link.Provider, "", "", identify.ManualScore
		if link.Kind == "issue" {
			body.IssueID = link.ID
		} else {
			body.SeriesID = link.ID
		}
	}
	if body.Provider != "gcd" && body.Provider != "comicvine" {
		writeError(w, http.StatusBadRequest, "informe provider (gcd ou comicvine), url ou issueId")
		return
	}
	if body.IssueID == "" && body.SeriesID != "" {
		if body.Number == "" {
			ed, err := d.Identifier.Get(r.Context(), id)
			if err != nil {
				internalError(w, err)
				return
			}
			body.Number = ed.Guess.Number
		}
		c, err := d.Metadata.IssueInSeries(r.Context(), body.Provider, body.SeriesID, body.Number)
		if errors.Is(err, metadata.ErrNoIssue) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if writeProviderError(w, err) {
			return
		}
		body.IssueID = c.IssueID
		if body.Score == 0 {
			body.Score = identify.ManualScore
		}
	}
	if body.IssueID == "" {
		writeError(w, http.StatusBadRequest, "informe issueId, seriesId + number ou url")
		return
	}
	err := d.Identifier.Apply(r.Context(), id, body.Provider, body.IssueID, body.Score, nil)
	if writeProviderError(w, err) {
		return
	}
	d.writeEdition(w, r, id)
}

// manualMetadata grava metadados preenchidos à mão.
func (d Deps) manualMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := d.lookupEdition(w, r)
	if !ok {
		return
	}
	var body metadata.Details
	if !decode(w, r, &body) {
		return
	}
	err := d.Identifier.ApplyManual(r.Context(), id, body)
	if errors.Is(err, identify.ErrManualSeries) || errors.Is(err, identify.ErrManualDate) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if writeProviderError(w, err) {
		return
	}
	d.writeEdition(w, r, id)
}

func (d Deps) writeEdition(w http.ResponseWriter, r *http.Request, id int64) {
	ed, err := d.Identifier.Get(r.Context(), id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ed)
}

func (d Deps) identifyLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := d.Libraries.Get(r.Context(), id); errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	n, err := d.Identifier.QueueLibrary(r.Context(), id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

func (d Deps) identifySeries(w http.ResponseWriter, r *http.Request) {
	lib, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	key := r.URL.Query().Get("series")
	if lib == 0 || key == "" {
		writeError(w, http.StatusBadRequest, "informe library e series")
		return
	}
	n, err := d.Identifier.QueueSeries(r.Context(), lib, key)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": n})
}

// writeProviderError escreve o erro de uma fonte com o código certo; devolve false se err == nil.
func writeProviderError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, metadata.ErrUnauthorized):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, metadata.ErrRateLimited), errors.Is(err, metadata.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, identify.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		internalError(w, err)
	}
	return true
}

func (d Deps) testComicVine(w http.ResponseWriter, r *http.Request) {
	err := d.Metadata.TestComicVine(r.Context())
	switch {
	case errors.Is(err, metadata.ErrUnauthorized):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, metadata.ErrRateLimited), errors.Is(err, metadata.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
