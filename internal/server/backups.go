package server

import (
	"errors"
	"net/http"

	"github.com/edu-ricardo/selfconvertercomic/internal/backup"
)

func (d Deps) registerBackups(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backups", d.listBackups)
	mux.HandleFunc("POST /api/backups", d.createBackup)
	mux.HandleFunc("GET /api/backups/{name}", d.downloadBackup)
	mux.HandleFunc("POST /api/backups/restore", d.restoreBackup)
}

func (d Deps) listBackups(w http.ResponseWriter, r *http.Request) {
	list, err := d.Backups.List()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d Deps) createBackup(w http.ResponseWriter, r *http.Request) {
	info, err := d.Backups.Create(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (d Deps) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := d.Backups.Path(name)
	if errors.Is(err, backup.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, p)
}

// restoreBackup recebe o .zip no corpo do pedido (Content-Type application/zip).
func (d Deps) restoreBackup(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, backup.MaxUpload)
	safety, err := d.Backups.Restore(r.Context(), body)
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "arquivo grande demais")
	case errors.Is(err, backup.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, backup.ErrBusy):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"restored": "ok", "safetyBackup": safety})
	}
}
