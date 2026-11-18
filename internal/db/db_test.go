package db

import (
	"path/filepath"
	"testing"
)

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qt.db")

	conn, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Version(conn)
	if err != nil {
		t.Fatal(err)
	}
	if v < 1 {
		t.Fatalf("versão = %d, quer >= 1", v)
	}
	if _, err := conn.Exec("SELECT id, data, updated_at FROM settings"); err != nil {
		t.Fatalf("tabela settings ausente: %v", err)
	}
	conn.Close()

	// Reabrir não reaplica migrações nem muda a versão.
	conn, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	v2, _ := Version(conn)
	if v2 != v {
		t.Fatalf("versão mudou ao reabrir: %d → %d", v, v2)
	}

	var mode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q (%v), quer wal", mode, err)
	}
}
