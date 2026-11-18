package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitFor checa cond até o prazo.
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tempo esgotado esperando: %s", what)
}

func TestManagerWatchesAndRecoversDisk(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := filepath.Join(t.TempDir(), "HD")
	write(t, filepath.Join(root, "Marvel", "a.pdf"), "a")
	lib, _ := s.Create(ctx, "HD", root, true)

	m := NewManager(s, func() time.Duration { return time.Hour })
	m.debounce = 100 * time.Millisecond
	m.tick = 100 * time.Millisecond
	go m.Run(ctx)

	count := func() int {
		l, _ := s.Get(ctx, lib.ID)
		return l.Counts["pdf"]
	}
	waitFor(t, "varredura inicial", 5*time.Second, func() bool { return count() == 1 })

	// Arquivo novo numa pasta observada e numa pasta nova.
	write(t, filepath.Join(root, "Marvel", "b.pdf"), "b")
	write(t, filepath.Join(root, "DC", "c.pdf"), "c")
	waitFor(t, "arquivos novos detectados pelo watcher", 5*time.Second, func() bool { return count() == 3 })

	// Disco some → indisponível, catálogo mantido; volta → disponível (pela checagem periódica).
	// Forget imita o kernel derrubando as observações ao desmontar (e no Windows libera a pasta
	// para o Rename).
	m.Forget(lib.ID)
	tmp := root + "-fora"
	if err := os.Rename(root, tmp); err != nil {
		t.Fatal(err)
	}
	m.Enqueue(lib.ID)
	waitFor(t, "biblioteca indisponível", 5*time.Second, func() bool {
		l, _ := s.Get(ctx, lib.ID)
		return !l.Available && l.Counts["pdf"] == 3
	})
	if err := os.Rename(tmp, root); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "biblioteca disponível de novo", 5*time.Second, func() bool {
		l, _ := s.Get(ctx, lib.ID)
		return l.Available
	})

	// Depois de o disco voltar, as pastas voltam a ser observadas.
	write(t, filepath.Join(root, "Marvel", "d.pdf"), "d")
	waitFor(t, "watcher ativo após reconexão", 5*time.Second, func() bool { return count() == 4 })
}
