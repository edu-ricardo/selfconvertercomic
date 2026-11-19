package server

import (
	"errors"
	"net/http"

	"github.com/edu-ricardo/selfconvertercomic/internal/dupes"
	"github.com/edu-ricardo/selfconvertercomic/internal/fsutil"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

func (d Deps) registerDuplicates(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/duplicates", d.listDuplicates)
	mux.HandleFunc("GET /api/duplicates/converted-pdfs", d.convertedPDFs)
	mux.HandleFunc("GET /api/trash", d.listTrash)
	mux.HandleFunc("POST /api/trash", d.moveToTrash)
	mux.HandleFunc("POST /api/trash/restore", d.restoreTrash)
	mux.HandleFunc("POST /api/trash/empty", d.emptyTrash)
}

func (d Deps) convertedPDFs(w http.ResponseWriter, r *http.Request) {
	list, err := d.Trash.ConvertedPDFs(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d Deps) listTrash(w http.ResponseWriter, r *http.Request) {
	list, err := d.Trash.List(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// moveToTrash manda arquivos para a lixeira da app (a pedido do usuário; nunca a última cópia).
func (d Deps) moveToTrash(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FileIDs []int64 `json:"fileIds"`
		Reason  string  `json:"reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "nenhum arquivo escolhido")
		return
	}
	n, err := d.Trash.MoveToTrash(r.Context(), body.FileIDs, body.Reason)
	if !writeTrashError(w, err) {
		writeJSON(w, http.StatusOK, map[string]int{"moved": n})
	}
}

func (d Deps) restoreTrash(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	n, err := d.Trash.Restore(r.Context(), body.IDs)
	if !writeTrashError(w, err) {
		writeJSON(w, http.StatusOK, map[string]int{"restored": n})
	}
}

// emptyTrash apaga de vez: os itens pedidos, ou a lixeira inteira se "all" for true.
func (d Deps) emptyTrash(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if !decode(w, r, &body) {
		return
	}
	ids := body.IDs
	if body.All {
		ids = nil
	} else if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "informe ids ou all")
		return
	}
	n, err := d.Trash.Empty(r.Context(), ids)
	if !writeTrashError(w, err) {
		writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
	}
}

func writeTrashError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, dupes.ErrLastCopy), errors.Is(err, dupes.ErrOccupied), errors.Is(err, fsutil.ErrExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, dupes.ErrNotInTrash), errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		internalError(w, err)
	}
	return true
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
