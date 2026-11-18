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

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/server"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/edu-ricardo/selfconvertercomic/web"
)

// version é definido no build: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	addr := envOr("QT_ADDR", ":8080")
	dataDir := envOr("QT_DATA_DIR", "data")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatal("criando pasta de dados", err)
	}
	conn, err := db.Open(filepath.Join(dataDir, "quadrinhoteca.db"))
	if err != nil {
		fatal("abrindo banco", err)
	}
	defer conn.Close()

	srv := &http.Server{
		Addr: addr,
		Handler: server.New(server.Deps{
			Version:  version,
			UI:       web.Files(),
			Settings: settings.NewStore(conn),
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("QuadrinhoTeca iniciada", "addr", addr, "data", dataDir, "version", version)
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
