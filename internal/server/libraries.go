package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

type libraryView struct {
	library.Library
	Scanning bool `json:"scanning"`
}

func (d Deps) registerLibraries(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/libraries", d.listLibraries)
	mux.HandleFunc("POST /api/libraries", d.createLibrary)
	mux.HandleFunc("PUT /api/libraries/{id}", d.updateLibrary)
	mux.HandleFunc("DELETE /api/libraries/{id}", d.deleteLibrary)
	mux.HandleFunc("POST /api/libraries/{id}/scan", d.scanLibrary)
	mux.HandleFunc("GET /api/fs", d.browse)
}

func (d Deps) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := d.Libraries.List(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	busy, watchErr := d.Scanner.Status()
	views := make([]libraryView, len(libs))
	for i, l := range libs {
		views[i] = libraryView{Library: l, Scanning: busy[l.ID]}
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": views, "watchError": watchErr})
}

func (d Deps) createLibrary(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		AutoConvert *bool  `json:"autoConvert"`
	}
	if !decode(w, r, &in) {
		return
	}
	path, err := d.Roots.Resolve(in.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, pathError(err))
		return
	}
	auto := in.AutoConvert == nil || *in.AutoConvert
	lib, err := d.Libraries.Create(r.Context(), in.Name, path, auto)
	switch {
	case errors.Is(err, library.ErrOverlap):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		d.Scanner.Enqueue(lib.ID)
		writeJSON(w, http.StatusCreated, libraryView{Library: lib, Scanning: true})
	}
}

func (d Deps) updateLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name        string `json:"name"`
		AutoConvert bool   `json:"autoConvert"`
	}
	if !decode(w, r, &in) {
		return
	}
	lib, err := d.Libraries.Update(r.Context(), id, in.Name, in.AutoConvert)
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, library.ErrNameRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, lib)
	}
}

func (d Deps) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := d.Libraries.Delete(r.Context(), id); {
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		d.Scanner.Forget(id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (d Deps) scanLibrary(w http.ResponseWriter, r *http.Request) {
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
	d.Scanner.Enqueue(id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

// browse lista subpastas para o seletor de pasta da UI (restrito às raízes permitidas).
func (d Deps) browse(w http.ResponseWriter, r *http.Request) {
	l, err := d.Roots.List(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, pathError(err))
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func pathError(err error) string {
	switch {
	case errors.Is(err, library.ErrOutsideRoots):
		return err.Error()
	case errors.Is(err, fs.ErrNotExist):
		return "pasta não encontrada"
	case errors.Is(err, fs.ErrPermission):
		return "sem permissão para ler a pasta"
	default:
		return "pasta inválida: " + err.Error()
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return false
	}
	return true
}
