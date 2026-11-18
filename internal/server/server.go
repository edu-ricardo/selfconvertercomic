// Package server monta as rotas HTTP da QuadrinhoTeca.
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// New devolve o handler raiz: API em /api/ e a interface web (SPA) no resto.
func New(version string, ui fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "rota não encontrada"})
	})
	mux.Handle("/", spa(ui))
	return mux
}

// spa serve arquivos estáticos e devolve index.html para rotas do frontend.
func spa(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(ui, name); err != nil {
			// Rota do frontend (ex.: /catalogo): entrega o index.html.
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
