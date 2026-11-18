package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewStore(conn)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scan(t *testing.T, s *Store, lib Library) Result {
	t.Helper()
	r, err := s.Scan(context.Background(), lib)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestScanReflectsCreateChangeRemove(t *testing.T) {
	s := newStore(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "Marvel", "Guerras", "Capítulo 01.pdf"), "a")
	write(t, filepath.Join(root, "Marvel", "Guerra Civil.CBR"), "b")
	write(t, filepath.Join(root, "DC", "Batman.cbz"), "c")
	write(t, filepath.Join(root, "DC", "capa.jpg"), "ignorado")
	write(t, filepath.Join(root, ".lixo", "oculto.pdf"), "ignorado")

	lib, err := s.Create(context.Background(), "Quadrinhos", root, true)
	if err != nil {
		t.Fatal(err)
	}

	r := scan(t, s, lib)
	if r.Added != 3 || r.Total != 3 || r.Updated != 0 || r.Removed != 0 {
		t.Fatalf("primeira varredura: %+v", r)
	}
	got, _ := s.Get(context.Background(), lib.ID)
	if got.Counts["pdf"] != 1 || got.Counts["cbr"] != 1 || got.Counts["cbz"] != 1 || !got.Available || got.LastScanAt == nil {
		t.Fatalf("biblioteca após varredura: %+v", got)
	}

	// Sem mudanças: nada a fazer.
	if r := scan(t, s, lib); r.Added+r.Updated+r.Removed != 0 {
		t.Fatalf("revarredura sem mudanças: %+v", r)
	}

	// Altera um, remove outro, cria um novo.
	write(t, filepath.Join(root, "DC", "Batman.cbz"), "conteúdo maior")
	os.Remove(filepath.Join(root, "Marvel", "Guerra Civil.CBR"))
	write(t, filepath.Join(root, "Marvel", "Guerras", "Capítulo 02.pdf"), "d")

	r = scan(t, s, lib)
	if r.Added != 1 || r.Updated != 1 || r.Removed != 1 || r.Total != 3 {
		t.Fatalf("após mudanças: %+v", r)
	}
}

func TestScanMissingOrEmptyRootKeepsCatalog(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "HD")
	write(t, filepath.Join(root, "a.pdf"), "a")
	lib, _ := s.Create(ctx, "HD", root, true)
	scan(t, s, lib)

	// Disco "desmontado": a pasta de montagem fica vazia.
	os.Remove(filepath.Join(root, "a.pdf"))
	if _, err := s.Scan(ctx, lib); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("pasta vazia: esperava ErrUnavailable, veio %v", err)
	}
	// Pasta some de vez.
	os.RemoveAll(root)
	if _, err := s.Scan(ctx, lib); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("pasta ausente: esperava ErrUnavailable, veio %v", err)
	}
	got, _ := s.Get(ctx, lib.ID)
	if got.Available || got.LastError == "" || got.Counts["pdf"] != 1 {
		t.Fatalf("catálogo deveria ser mantido e biblioteca indisponível: %+v", got)
	}

	// Disco volta: disponível de novo.
	write(t, filepath.Join(root, "a.pdf"), "a")
	scan(t, s, lib)
	if got, _ := s.Get(ctx, lib.ID); !got.Available || got.LastError != "" {
		t.Fatalf("deveria voltar a ficar disponível: %+v", got)
	}
}

func TestCreateRejectsOverlap(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	root := t.TempDir()
	if _, err := s.Create(ctx, "", filepath.Join(root, "comics"), true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(root, "comics"), filepath.Join(root, "comics", "Marvel"), root} {
		if _, err := s.Create(ctx, "x", p, true); !errors.Is(err, ErrOverlap) {
			t.Errorf("%s: esperava ErrOverlap, veio %v", p, err)
		}
	}
	if _, err := s.Create(ctx, "", filepath.Join(root, "comics-2"), true); err != nil {
		t.Errorf("pasta irmã com prefixo parecido deveria ser aceita: %v", err)
	}
}

// Critério do M2: 10 mil arquivos em < 30 s e revarredura sem mudanças em < 5 s.
func TestScanPerformance10k(t *testing.T) {
	if testing.Short() {
		t.Skip("teste de desempenho")
	}
	s := newStore(t)
	root := t.TempDir()
	for i := 0; i < 10000; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("Editora%d", i%10), fmt.Sprintf("Serie%03d", i%500), fmt.Sprintf("Edicao %05d.pdf", i)), "x")
	}
	lib, _ := s.Create(context.Background(), "Grande", root, true)

	r := scan(t, s, lib)
	if r.Total != 10000 {
		t.Fatalf("total = %d", r.Total)
	}
	t.Logf("primeira varredura: %v", r.Duration)
	if r.Duration > 30*time.Second {
		t.Errorf("primeira varredura levou %v (limite 30 s)", r.Duration)
	}

	r = scan(t, s, lib)
	t.Logf("revarredura sem mudanças: %v", r.Duration)
	if r.Duration > 5*time.Second || r.Added+r.Updated+r.Removed != 0 {
		t.Errorf("revarredura: %+v (limite 5 s)", r)
	}
}

func TestDerive(t *testing.T) {
	cases := []struct{ rel, pub, series, title string }{
		{"Marvel/guerras_secretas/1984/Capítulo 01.pdf", "Marvel", "Marvel/guerras_secretas/1984", "Capítulo 01"},
		{"Marvel/Guerra Civil (2022).cbr", "Marvel", "Marvel", "Guerra Civil (2022)"},
		{"solto.cbz", "", "", "solto"},
	}
	for _, c := range cases {
		pub, series, title := Derive(c.rel)
		if pub != c.pub || series != c.series || title != c.title {
			t.Errorf("Derive(%q) = %q, %q, %q", c.rel, pub, series, title)
		}
	}
}
