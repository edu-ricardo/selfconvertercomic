package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

func (d Deps) registerTasks(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks", d.listTasks)
	mux.HandleFunc("POST /api/tasks/{id}/dismiss", d.dismissTask)
	mux.HandleFunc("POST /api/tasks/{id}/retry", d.retryTask)
	mux.HandleFunc("POST /api/notifications/test", d.testNotification)
}

func (d Deps) listTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "":
		status = tasks.Open
	case "all":
		status = ""
	case tasks.Open, tasks.Resolved, tasks.Dismissed:
	default:
		writeError(w, http.StatusBadRequest, "status deve ser open, resolved, dismissed ou all")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := d.Tasks.List(r.Context(), status, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	open, err := d.Tasks.CountOpen(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": list, "open": open})
}

func (d Deps) dismissTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	switch err := d.Tasks.Close(r.Context(), id, tasks.Dismissed); {
	case errors.Is(err, tasks.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"dismissed": true})
	}
}

// retryTask refaz a ação da tarefa (erro de conversão → volta o job para a fila) e a fecha.
func (d Deps) retryTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := d.Tasks.Get(r.Context(), id)
	if errors.Is(err, tasks.ErrNotFound) || (err == nil && t.Status != tasks.Open) {
		writeError(w, http.StatusNotFound, tasks.ErrNotFound.Error())
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	if t.Kind != tasks.KindConversionError {
		writeError(w, http.StatusUnprocessableEntity, "esta tarefa não pode ser refeita")
		return
	}
	switch err := d.Jobs.Retry(r.Context(), t.RefID); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusConflict, "a conversão não está mais com erro; use Ignorar para fechar a tarefa")
		return
	case err != nil && strings.Contains(err.Error(), "UNIQUE"):
		writeError(w, http.StatusConflict, "já existe uma conversão ativa para este arquivo")
		return
	case err != nil:
		internalError(w, err)
		return
	}
	// Se falhar de novo, uma nova tarefa é aberta.
	if err := d.Tasks.Close(r.Context(), id, tasks.Resolved); err != nil && !errors.Is(err, tasks.ErrNotFound) {
		internalError(w, err)
		return
	}
	d.Runner.Wake()
	writeJSON(w, http.StatusOK, map[string]bool{"queued": true})
}

func (d Deps) testNotification(w http.ResponseWriter, r *http.Request) {
	if err := d.Notifier.Test(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}
