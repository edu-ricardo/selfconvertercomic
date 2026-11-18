// Command quadrinhoteca é o servidor da QuadrinhoTeca: API HTTP + interface web embutida.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/catalog"
	"github.com/edu-ricardo/selfconvertercomic/internal/convert"
	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/server"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/edu-ricardo/selfconvertercomic/web"
)

// version é definido no build: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	addr := envOr("QT_ADDR", ":8080")
	dataDir := envOr("QT_DATA_DIR", "data")
	roots := library.ParseRoots(envOr("QT_LIBRARY_ROOTS", "/external,/storage"))

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatal("criando pasta de dados", err)
	}
	conn, err := db.Open(filepath.Join(dataDir, "quadrinhoteca.db"))
	if err != nil {
		fatal("abrindo banco", err)
	}
	defer conn.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	settingsStore := settings.NewStore(conn)
	libraries := library.NewStore(conn)
	scanner := library.NewManager(libraries, func() time.Duration {
		s, err := settingsStore.Get(context.Background())
		if err != nil {
			return 6 * time.Hour
		}
		return time.Duration(s.Scan.RescanHours) * time.Hour
	})
	jobStore := jobs.NewStore(conn)
	runner := jobs.NewRunner(jobStore, settingsStore, convert.Converter{}, func(ctx context.Context, id int64) (string, string, error) {
		l, err := libraries.Get(ctx, id)
		if err == nil && !l.Available {
			err = errors.New(l.LastError)
		}
		return l.Path, l.Name, err
	})
	// Varredura nova → procura PDFs para converter; conversão pronta → revarre para o CBZ aparecer.
	scanner.OnScan = func(library.Library, library.Result) { runner.EnqueueEligible(ctx) }
	runner.OnDone = scanner.Enqueue
	go scanner.Run(ctx)
	go runner.Run(ctx)

	srv := &http.Server{
		Addr: addr,
		Handler: server.New(server.Deps{
			Version:   version,
			UI:        web.Files(),
			Settings:  settingsStore,
			Libraries: libraries,
			Roots:     roots,
			Scanner:   scanner,
			Jobs:      jobStore,
			Runner:    runner,
			Catalog:   catalog.NewStore(conn),
			Thumbs:    &catalog.Thumbnailer{Dir: filepath.Join(dataDir, "cache", "thumbs")},
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("QuadrinhoTeca iniciada", "addr", addr, "data", dataDir, "roots", roots, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("servidor HTTP falhou", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("encerramento do servidor falhou", "err", err)
	}
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
