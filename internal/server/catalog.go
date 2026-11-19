package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/edu-ricardo/selfconvertercomic/internal/catalog"
	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
)

func (d Deps) registerCatalog(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog/series", d.listSeries)
	mux.HandleFunc("GET /api/catalog/files", d.listFiles)
	mux.HandleFunc("GET /api/catalog/publishers", d.listPublishers)
	mux.HandleFunc("GET /api/files/{id}", d.getFile)
	mux.HandleFunc("GET /api/files/{id}/thumb", d.fileThumb)
	mux.HandleFunc("POST /api/files/{id}/convert", d.convertFile)
}

// filterFrom lê os filtros comuns da query string.
func filterFrom(w http.ResponseWriter, r *http.Request) (catalog.Filter, bool) {
	q := r.URL.Query()
	f := catalog.Filter{
		Publisher: q.Get("publisher"),
		Format:    q.Get("format"),
		Status:    q.Get("status"),
		Query:     q.Get("q"),
	}
	f.LibraryID, _ = strconv.ParseInt(q.Get("library"), 10, 64)
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if err := f.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return f, false
	}
	return f, true
}

func (d Deps) listSeries(w http.ResponseWriter, r *http.Request) {
	f, ok := filterFrom(w, r)
	if !ok {
		return
	}
	page, err := d.Catalog.ListSeries(r.Context(), f)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (d Deps) listFiles(w http.ResponseWriter, r *http.Request) {
	f, ok := filterFrom(w, r)
	if !ok {
		return
	}
	var series *string
	if r.URL.Query().Has("series") {
		if f.LibraryID == 0 {
			writeError(w, http.StatusBadRequest, "series exige library")
			return
		}
		s := r.URL.Query().Get("series")
		series = &s
	}
	page, err := d.Catalog.ListFiles(r.Context(), f, series)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (d Deps) listPublishers(w http.ResponseWriter, r *http.Request) {
	lib, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	list, err := d.Catalog.Publishers(r.Context(), lib)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// lookupFile carrega o arquivo do caminho /api/files/{id}...; escreve o erro e devolve false se falhar.
func (d Deps) lookupFile(w http.ResponseWriter, r *http.Request) (catalog.File, string, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return catalog.File{}, "", false
	}
	f, abs, err := d.Catalog.Get(r.Context(), id)
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return f, "", false
	} else if err != nil {
		internalError(w, err)
		return f, "", false
	}
	return f, abs, true
}

func (d Deps) getFile(w http.ResponseWriter, r *http.Request) {
	f, abs, ok := d.lookupFile(w, r)
	if !ok {
		return
	}
	s, err := d.Settings.Get(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	readers := map[string]string{}
	if s.Readers.KavitaURL != "" {
		readers["kavita"] = strings.TrimRight(s.Readers.KavitaURL, "/")
	}
	if s.Readers.BookOrbitURL != "" {
		readers["bookOrbit"] = strings.TrimRight(s.Readers.BookOrbitURL, "/")
	}
	var edition any
	if f.EditionID != 0 {
		ed, err := d.Identifier.Get(r.Context(), f.EditionID)
		if err != nil {
			internalError(w, err)
			return
		}
		edition = ed
	}
	writeJSON(w, http.StatusOK, map[string]any{"file": f, "path": abs, "readers": readers, "edition": edition})
}

func (d Deps) fileThumb(w http.ResponseWriter, r *http.Request) {
	f, abs, ok := d.lookupFile(w, r)
	if !ok {
		return
	}
	p, err := d.Thumbs.Path(r.Context(), f, abs)
	if err != nil {
		// Sem capa (arquivo ilegível, formato estranho): a UI mostra um marcador.
		writeError(w, http.StatusNotFound, "sem capa: "+err.Error())
		return
	}
	// Com ?v=<mtime> a URL muda quando o arquivo muda: pode ficar em cache por muito tempo.
	// Sem v (capa de série), o navegador revalida (ServeFile responde 304 se nada mudou).
	if r.URL.Query().Has("v") {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	http.ServeFile(w, r, p)
}

func (d Deps) convertFile(w http.ResponseWriter, r *http.Request) {
	f, abs, ok := d.lookupFile(w, r)
	if !ok {
		return
	}
	if f.Format != "pdf" {
		writeError(w, http.StatusUnprocessableEntity, "só PDFs são convertidos")
		return
	}
	queued, err := d.Runner.EnqueueFile(r.Context(), f.LibraryID, abs, f.RelPath, f.Size, f.Mtime)
	switch {
	case errors.Is(err, jobs.ErrHasCBZ):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		internalError(w, err)
	case !queued:
		writeError(w, http.StatusConflict, "o arquivo já está na fila")
	default:
		writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
	}
}
