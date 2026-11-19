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

// applyMetadata aplica o candidato escolhido pelo usuário (fecha a tarefa de confirmação).
func (d Deps) applyMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := d.lookupEdition(w, r)
	if !ok {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		IssueID  string `json:"issueId"`
		Score    int    `json:"score"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.IssueID == "" || (body.Provider != "gcd" && body.Provider != "comicvine") {
		writeError(w, http.StatusBadRequest, "informe provider (gcd ou comicvine) e issueId")
		return
	}
	err := d.Identifier.Apply(r.Context(), id, body.Provider, body.IssueID, body.Score, nil)
	if writeProviderError(w, err) {
		return
	}
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
