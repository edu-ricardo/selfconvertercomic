package tasks

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

func newStore(t *testing.T) (*Store, int64) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	lib, err := library.NewStore(conn).Create(context.Background(), "HD", t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(conn), lib.ID
}

func TestConversionLifecycle(t *testing.T) {
	s, lib := newStore(t)
	ctx := context.Background()
	rel := "Marvel/1984/Capítulo 03.pdf"

	// Falha → tarefa aberta.
	if err := s.FromConversion(ctx, lib, rel, 7, true, "PDF corrompido"); err != nil {
		t.Fatal(err)
	}
	open, _ := s.List(ctx, Open, 10)
	if len(open) != 1 || open[0].Title != "Erro ao converter Capítulo 03.pdf" || open[0].Detail != "PDF corrompido" || open[0].RefID != 7 || open[0].LibraryName != "HD" {
		t.Fatalf("tarefa: %+v", open)
	}

	// Falha de novo no mesmo arquivo → continua uma tarefa só (atualizada).
	s.FromConversion(ctx, lib, rel, 8, true, "outro erro")
	open, _ = s.List(ctx, Open, 10)
	if len(open) != 1 || open[0].RefID != 8 || open[0].Detail != "outro erro" {
		t.Fatalf("deveria haver uma tarefa atualizada: %+v", open)
	}

	// Sucesso depois → resolvida sozinha.
	s.FromConversion(ctx, lib, rel, 9, false, "")
	if n, _ := s.CountOpen(ctx); n != 0 {
		t.Fatalf("tarefa deveria ter sido resolvida; abertas: %d", n)
	}
	resolved, _ := s.List(ctx, Resolved, 10)
	if len(resolved) != 1 || resolved[0].ResolvedAt == nil {
		t.Fatalf("resolvidas: %+v", resolved)
	}

	// Sucesso com aviso → tarefa; ignorar fecha como dismissed.
	s.FromConversion(ctx, lib, "b.pdf", 10, false, "CBZ criado, mas ...")
	open, _ = s.List(ctx, Open, 10)
	if len(open) != 1 {
		t.Fatalf("aviso deveria virar tarefa: %+v", open)
	}
	if err := s.Close(ctx, open[0].ID, Dismissed); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(ctx, open[0].ID, Dismissed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fechar de novo: %v", err)
	}
	all, _ := s.List(ctx, "", 10)
	if len(all) != 2 {
		t.Fatalf("todas: %d", len(all))
	}
}
