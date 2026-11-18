package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

func (d Deps) registerJobs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/jobs", d.listJobs)
	mux.HandleFunc("POST /api/jobs/{id}/retry", d.retryJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", d.cancelJob)
	mux.HandleFunc("POST /api/libraries/{id}/convert", d.convertLibrary)
	mux.HandleFunc("PUT /api/turbo", d.setTurbo)
}

var jobFilters = map[string][]string{
	"active":    {jobs.Running, jobs.Queued},
	"failed":    {jobs.Failed},
	"done":      {jobs.Done},
	"cancelled": {jobs.Cancelled},
	"all":       nil,
}

func turboJSON(until time.Time) map[string]any {
	if until.IsZero() {
		return map[string]any{"enabled": false, "until": nil}
	}
	return map[string]any{"enabled": true, "until": until.UTC().Format(time.RFC3339)}
}

func (d Deps) listJobs(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("status")
	if filter == "" {
		filter = "active"
	}
	statuses, ok := jobFilters[filter]
	if !ok {
		writeError(w, http.StatusBadRequest, "status deve ser active, failed, done, cancelled ou all")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := d.Jobs.List(r.Context(), statuses, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	counts, err := d.Jobs.Counts(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": list, "counts": counts, "turbo": turboJSON(d.Runner.Turbo())})
}

func (d Deps) retryJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := d.Jobs.Retry(r.Context(), id); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "job não encontrado ou não está com falha/cancelado")
	case err != nil && strings.Contains(err.Error(), "UNIQUE"):
		writeError(w, http.StatusConflict, "já existe uma conversão ativa para este arquivo")
	case err != nil:
		internalError(w, err)
	default:
		d.Runner.Wake()
		writeJSON(w, http.StatusOK, map[string]bool{"queued": true})
	}
}

func (d Deps) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := d.Runner.Cancel(r.Context(), id); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, "job não encontrado ou já terminou")
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
	}
}

func (d Deps) convertLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	lib, err := d.Libraries.Get(r.Context(), id)
	if errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	if !lib.Available {
		writeError(w, http.StatusConflict, "biblioteca indisponível")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"queued": d.Runner.EnqueueLibrary(r.Context(), id)})
}

func (d Deps) setTurbo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	until, err := d.Runner.SetTurbo(r.Context(), in.Enabled)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, turboJSON(until))
}
