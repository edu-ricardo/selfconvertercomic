package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/catalog"
	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

type world struct {
	conn *sql.DB
	path string
	mgr  *Manager
	now  time.Time
}

func openWorld(t *testing.T, dbPath, backupDir string) *world {
	t.Helper()
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{conn: conn, path: dbPath, now: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)}
	w.mgr = &Manager{DB: conn, Dir: backupDir, Settings: settings.NewStore(conn), Version: "test",
		Now: func() time.Time { return w.now }}
	return w
}

// populate cria uma biblioteca varrida, configurações alteradas, um job e uma tarefa.
func populate(t *testing.T, w *world, root string) {
	t.Helper()
	ctx := context.Background()
	for _, f := range []string{"Marvel/1984/Capítulo 01.pdf", "Marvel/1984/Capítulo 02.cbz", "DC/Batman.cbr"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	libs := library.NewStore(w.conn)
	lib, err := libs.Create(ctx, "Quadrinhos", root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := libs.Scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	s := settings.Defaults()
	s.Conversion.Workers = 3
	s.Readers.KavitaURL = "http://umbrel.local:8798"
	s.Backup.Keep = 10
	if err := w.mgr.Settings.Put(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := w.conn.Exec("INSERT INTO jobs (library_id, rel_path, src_size, src_mtime, status, error) VALUES (?, 'Marvel/1984/Capítulo 01.pdf', 1, 1, 'failed', 'x')", lib.ID); err != nil {
		t.Fatal(err)
	}
	tasks.NewStore(w.conn).FromConversion(ctx, lib.ID, "Marvel/1984/Capítulo 01.pdf", 1, true, "x")
}

// state resume o que o usuário vê: configurações, bibliotecas, arquivos, jobs, tarefas e busca.
func state(t *testing.T, conn *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	s, _ := settings.NewStore(conn).Get(ctx)
	libs, _ := library.NewStore(conn).List(ctx)
	var b strings.Builder
	sj, _ := json.Marshal(s)
	fmt.Fprintf(&b, "settings=%s\n", sj)
	for _, l := range libs {
		fmt.Fprintf(&b, "lib=%s %s auto=%v counts=%v\n", l.Name, l.Path, l.AutoConvert, l.Counts)
	}
	rows, _ := conn.Query("SELECT rel_path, format, series, publisher FROM files ORDER BY rel_path")
	for rows.Next() {
		var a, f, sr, p string
		rows.Scan(&a, &f, &sr, &p)
		fmt.Fprintf(&b, "file=%s %s %s %s\n", a, f, sr, p)
	}
	rows.Close()
	var jobs, open int
	conn.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs)
	conn.QueryRow("SELECT count(*) FROM tasks WHERE status = 'open'").Scan(&open)
	found, _ := catalog.NewStore(conn).ListFiles(ctx, catalog.Filter{Query: "capitulo", Limit: 10}, nil)
	fmt.Fprintf(&b, "jobs=%d tasks=%d busca=%d\n", jobs, open, found.Total)
	return b.String()
}

// Critério do M6: exportar → apagar banco → restaurar → catálogo e configurações idênticos.
func TestExportDeleteRestore(t *testing.T) {
	dir := t.TempDir()
	backups := filepath.Join(dir, "backups")
	dbPath := filepath.Join(dir, "quadrinhoteca.db")
	w := openWorld(t, dbPath, backups)
	populate(t, w, filepath.Join(dir, "comics"))
	before := state(t, w.conn)

	info, err := w.mgr.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "quadrinhoteca-20261118-100000-000.zip" || info.Size == 0 {
		t.Fatalf("backup: %+v", info)
	}

	// Apaga o banco (e os arquivos do WAL) e começa do zero.
	w.conn.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(dbPath + suffix)
	}
	w2 := openWorld(t, dbPath, backups)
	defer w2.conn.Close()
	if state(t, w2.conn) == before {
		t.Fatal("o banco novo deveria estar vazio")
	}

	zipPath, _ := w2.mgr.Path(info.Name)
	f, _ := os.Open(zipPath)
	defer f.Close()
	w2.now = w2.now.Add(time.Minute)
	safety, err := w2.mgr.Restore(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if after := state(t, w2.conn); after != before {
		t.Fatalf("estado restaurado difere:\nantes:\n%s\ndepois:\n%s", before, after)
	}
	if _, err := w2.mgr.Path(safety); err != nil {
		t.Fatalf("backup de segurança %q deveria existir", safety)
	}
}

func TestRestoreOverLiveDatabase(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, filepath.Join(dir, "qt.db"), filepath.Join(dir, "backups"))
	defer w.conn.Close()
	populate(t, w, filepath.Join(dir, "comics"))
	before := state(t, w.conn)
	info, err := w.mgr.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Mudanças depois do backup (que a restauração deve desfazer).
	w.conn.Exec("DELETE FROM libraries")
	s := settings.Defaults()
	w.mgr.Settings.Put(context.Background(), s)

	restoredCalled := false
	w.mgr.AfterRestore = func() { restoredCalled = true }
	p, _ := w.mgr.Path(info.Name)
	f, _ := os.Open(p)
	defer f.Close()
	w.now = w.now.Add(time.Minute)
	if _, err := w.mgr.Restore(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if after := state(t, w.conn); after != before {
		t.Fatalf("estado restaurado difere:\nantes:\n%s\ndepois:\n%s", before, after)
	}
	if !restoredCalled {
		t.Error("AfterRestore não foi chamado")
	}
}

func TestRestoreRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, filepath.Join(dir, "qt.db"), filepath.Join(dir, "backups"))
	defer w.conn.Close()
	ctx := context.Background()

	if _, err := w.mgr.Restore(ctx, strings.NewReader("não é zip")); !errors.Is(err, ErrInvalid) {
		t.Errorf("não-zip: %v", err)
	}

	mkzip := func(entries map[string]string) *os.File {
		p := filepath.Join(t.TempDir(), "x.zip")
		f, _ := os.Create(p)
		zw := zip.NewWriter(f)
		for n, c := range entries {
			e, _ := zw.Create(n)
			e.Write([]byte(c))
		}
		zw.Close()
		f.Seek(0, 0)
		return f
	}
	cases := map[string]map[string]string{
		"zip qualquer":        {"foto.jpg": "x"},
		"versão mais nova":    {manifestEntry: `{"app":"quadrinhoteca","schemaVersion":999}`, dbEntry: "x"},
		"banco corrompido":    {manifestEntry: `{"app":"quadrinhoteca","schemaVersion":1}`, dbEntry: "isto não é sqlite"},
		"de outro aplicativo": {manifestEntry: `{"app":"outro","schemaVersion":1}`, dbEntry: "x"},
	}
	for name, entries := range cases {
		f := mkzip(entries)
		if _, err := w.mgr.Restore(ctx, f); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: esperava ErrInvalid, veio %v", name, err)
		}
		f.Close()
	}
	if list, _ := w.mgr.List(); len(list) != 0 {
		t.Errorf("restauração recusada não deveria criar backup de segurança: %v", list)
	}

	w.mgr.CanRestore = func(context.Context) error { return errors.New("há conversões em andamento") }
	if _, err := w.mgr.Restore(ctx, strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "conversões") {
		t.Errorf("CanRestore deveria bloquear: %v", err)
	}
}

func TestRetentionAndDaily(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, filepath.Join(dir, "qt.db"), filepath.Join(dir, "backups"))
	defer w.conn.Close()
	ctx := context.Background()
	s := settings.Defaults()
	s.Backup.Keep = 3
	w.mgr.Settings.Put(ctx, s)

	// Diário: cria se não há nenhum; não repete antes de 24 h; repete depois.
	if err := w.mgr.dailyIfDue(ctx); err != nil {
		t.Fatal(err)
	}
	w.now = w.now.Add(23 * time.Hour)
	w.mgr.dailyIfDue(ctx)
	if list, _ := w.mgr.List(); len(list) != 1 {
		t.Fatalf("backups após 23 h: %d, quer 1", len(list))
	}
	for range 4 {
		w.now = w.now.Add(25 * time.Hour)
		if err := w.mgr.dailyIfDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := w.mgr.List()
	if len(list) != 3 || !list[0].CreatedAt.Equal(w.now) {
		t.Fatalf("retenção: %+v", list)
	}

	// Desligado: não cria.
	s.Backup.Daily = false
	w.mgr.Settings.Put(ctx, s)
	w.now = w.now.Add(48 * time.Hour)
	w.mgr.dailyIfDue(ctx)
	if l, _ := w.mgr.List(); len(l) != 3 {
		t.Fatalf("backup diário desligado criou backup: %d", len(l))
	}

	// Dois backups no mesmo instante não colidem.
	a, err1 := w.mgr.Create(ctx)
	b, err2 := w.mgr.Create(ctx)
	if err1 != nil || err2 != nil || a.Name == b.Name {
		t.Fatalf("backups simultâneos: %v %v %s %s", err1, err2, a.Name, b.Name)
	}
	if l, _ := w.mgr.List(); l[0].Name != b.Name {
		t.Fatalf("o mais recente deveria vir primeiro: %v", l)
	}

	if _, err := w.mgr.Path("../qt.db"); !errors.Is(err, ErrNotFound) {
		t.Errorf("nome fora do padrão deveria ser recusado: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "backups"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("arquivo temporário sobrou: %s", e.Name())
		}
	}
}
