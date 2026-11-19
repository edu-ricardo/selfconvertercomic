package server

import (
	"errors"
	"net/http"

	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/organize"
)

func (d Deps) registerOrganize(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/libraries/{id}/organize", d.organizePlan)
	mux.HandleFunc("POST /api/libraries/{id}/organize", d.organizeApply)
	mux.HandleFunc("GET /api/libraries/{id}/organize/history", d.organizeHistory)
	mux.HandleFunc("POST /api/organize/{id}/undo", d.organizeUndo)
}

// organizePlan devolve a prévia; ?template= (padrão: o das configurações) e ?series= (chave da série).
func (d Deps) organizePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tpl := r.URL.Query().Get("template")
	if tpl == "" {
		s, err := d.Settings.Get(r.Context())
		if err != nil {
			internalError(w, err)
			return
		}
		tpl = s.Organize.Template
	}
	p, err := d.Organizer.Plan(r.Context(), id, r.URL.Query().Get("series"), tpl)
	if !writeOrganizeError(w, err) {
		writeJSON(w, http.StatusOK, p)
	}
}

func (d Deps) organizeApply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Template string  `json:"template"`
		Editions []int64 `json:"editions"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.Editions) == 0 {
		writeError(w, http.StatusBadRequest, "nenhuma edição escolhida")
		return
	}
	op, err := d.Organizer.Apply(r.Context(), id, body.Template, body.Editions)
	if !writeOrganizeError(w, err) {
		writeJSON(w, http.StatusOK, op)
	}
}

func (d Deps) organizeHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := d.Organizer.History(r.Context(), id, 50)
	if !writeOrganizeError(w, err) {
		writeJSON(w, http.StatusOK, list)
	}
}

func (d Deps) organizeUndo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	op, err := d.Organizer.Undo(r.Context(), id)
	if !writeOrganizeError(w, err) {
		writeJSON(w, http.StatusOK, op)
	}
}

// writeOrganizeError escreve o erro com o código certo; devolve false se err == nil.
func writeOrganizeError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, library.ErrNotFound), errors.Is(err, organize.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, organize.ErrUnavailable), errors.Is(err, organize.ErrCannotUndo):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, organize.ErrTemplate):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		internalError(w, err)
	}
	return true
}
