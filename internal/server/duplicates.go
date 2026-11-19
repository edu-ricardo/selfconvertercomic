package server

import (
	"net/http"

	"github.com/edu-ricardo/selfconvertercomic/internal/dupes"
)

func (d Deps) registerDuplicates(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/duplicates", d.listDuplicates)
}

// listDuplicates devolve os grupos de duplicados (só lista; nada é apagado).
func (d Deps) listDuplicates(w http.ResponseWriter, r *http.Request) {
	rep, err := dupes.Find(r.Context(), d.DB, 200)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
