// Package server monta as rotas HTTP da QuadrinhoTeca.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/edu-ricardo/selfconvertercomic/internal/backup"
	"github.com/edu-ricardo/selfconvertercomic/internal/catalog"
	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
	"github.com/edu-ricardo/selfconvertercomic/internal/notify"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

// Deps são as dependências das rotas.
type Deps struct {
	Version   string
	UI        fs.FS
	Settings  *settings.Store
	Libraries *library.Store
	Roots     library.Roots
	Scanner   *library.Manager
	Jobs      *jobs.Store
	Runner    *jobs.Runner
	Catalog   *catalog.Store
	Thumbs    *catalog.Thumbnailer
	Tasks     *tasks.Store
	Notifier  *notify.Notifier
	Backups   *backup.Manager
	Metadata  *metadata.Finder
}

// New devolve o handler raiz: API em /api/ e a interface web (SPA) no resto.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": d.Version})
	})
	mux.HandleFunc("GET /api/settings", d.getSettings)
	mux.HandleFunc("PUT /api/settings", d.putSettings)
	d.registerLibraries(mux)
	d.registerJobs(mux)
	d.registerCatalog(mux)
	d.registerTasks(mux)
	d.registerBackups(mux)
	d.registerMetadata(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "rota não encontrada")
	})
	mux.Handle("/", spa(d.UI))
	return mux
}

func (d Deps) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := d.Settings.Get(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// putSettings aplica o JSON recebido sobre as configurações atuais (campos omitidos não mudam).
func (d Deps) putSettings(w http.ResponseWriter, r *http.Request) {
	s, err := d.Settings.Get(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	if !decode(w, r, &s) {
		return
	}
	// A pasta de arquivo precisa estar num disco montado; senão o PDF iria para dentro do container.
	if s.Conversion.OriginalAction == "archive" && s.Conversion.ArchiveDir != "" && !d.Roots.Contains(s.Conversion.ArchiveDir) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "configurações inválidas",
			"fields": map[string]string{"conversion.archiveDir": "use uma pasta dentro de " + strings.Join(d.Roots, " ou ")}})
		return
	}
	var ve *settings.ValidationError
	switch err := d.Settings.Put(r.Context(), s); {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "configurações inválidas", "fields": ve.Fields})
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, s)
	}
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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("erro interno", "err", err)
	writeError(w, http.StatusInternalServerError, "erro interno")
}
