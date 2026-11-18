package settings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
)

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewStore(conn)
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGetWithoutSavedReturnsDefaults(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "qt.db"))
	got, err := st.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != Defaults() {
		t.Fatalf("got %+v, quer padrões", got)
	}
}

func TestPutPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qt.db")
	ctx := context.Background()

	s := Defaults()
	s.Conversion.Workers = 2
	s.Readers.KavitaURL = "http://umbrel.local:8798"
	if err := openStore(t, path).Put(ctx, s); err != nil {
		t.Fatal(err)
	}

	got, err := openStore(t, path).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != s {
		t.Fatalf("got %+v, quer %+v", got, s)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	s := Defaults()
	s.Conversion.Workers = 0
	s.Conversion.OriginalAction = "archive" // sem ArchiveDir
	s.Readers.BookOrbitURL = "umbrel.local:8746"
	s.Notifications.NtfyURL = "http://ntfy.local"

	var ve *ValidationError
	if err := s.Validate(); !errors.As(err, &ve) {
		t.Fatalf("esperava ValidationError, veio %v", err)
	}
	for _, field := range []string{
		"conversion.workers", "conversion.archiveDir", "readers.bookOrbitUrl", "notifications.ntfyTopic",
	} {
		if _, ok := ve.Fields[field]; !ok {
			t.Errorf("campo %s deveria ser inválido; erros: %v", field, ve.Fields)
		}
	}
}

func TestPutRejectsInvalid(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "qt.db"))
	s := Defaults()
	s.Backup.Keep = 0
	if err := st.Put(context.Background(), s); err == nil {
		t.Fatal("Put aceitou configuração inválida")
	}
	got, _ := st.Get(context.Background())
	if got != Defaults() {
		t.Fatal("configuração inválida foi gravada")
	}
}
