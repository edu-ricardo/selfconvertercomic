package server

import (
	"errors"
	"net/http"

	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
)

func (d Deps) registerMetadata(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/files/{id}/candidates", d.fileCandidates)
	mux.HandleFunc("POST /api/metadata/comicvine/test", d.testComicVine)
}

// fileCandidates procura nos provedores a edição do arquivo (série, número e ano identificados).
func (d Deps) fileCandidates(w http.ResponseWriter, r *http.Request) {
	f, _, ok := d.lookupFile(w, r)
	if !ok {
		return
	}
	g := metadata.Guess{Series: f.SeriesName, Volume: f.Volume, Number: f.Number, Year: f.Year}
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
