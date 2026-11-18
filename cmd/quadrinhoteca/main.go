// Command quadrinhoteca é o servidor da QuadrinhoTeca: API HTTP + interface web embutida.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/server"
	"github.com/edu-ricardo/selfconvertercomic/web"
)

// version é definido no build: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	addr := envOr("QT_ADDR", ":8080")

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(version, web.Files()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("QuadrinhoTeca iniciada", "addr", addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("servidor HTTP falhou", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("encerramento do servidor falhou", "err", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
