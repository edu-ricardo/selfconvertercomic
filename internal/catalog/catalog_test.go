package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

type fixture struct {
	conn *sql.DB
	cat  *Store
	lib  library.Library
	root string
}

func newFixture(t *testing.T, files ...string) fixture {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	libs := library.NewStore(conn)
	lib, err := libs.Create(context.Background(), "HD", root, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := libs.Scan(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	return fixture{conn: conn, cat: NewStore(conn), lib: lib, root: root}
}

func (fx fixture) job(t *testing.T, rel, status string) {
	t.Helper()
	if _, err := fx.conn.Exec("INSERT INTO jobs (library_id, rel_path, src_size, src_mtime, status) VALUES (?, ?, 1, 1, ?)",
		fx.lib.ID, rel, status); err != nil {
		t.Fatal(err)
	}
}

func TestSeriesFiltersAndStatus(t *testing.T) {
	fx := newFixture(t,
		"Marvel/guerras_secretas/1984/Capítulo 01.pdf",
		"Marvel/guerras_secretas/1984/Capítulo 01.cbz",
		"Marvel/guerras_secretas/1984/Capítulo 02.pdf",
		"Marvel/guerras_secretas/1984/Capítulo 03.pdf",
		"Marvel/guerras_secretas/1984/Capítulo 04.pdf",
		"Marvel/Guerra Civil (2022).cbr",
		"DC Comics/Batman Vol. 1.cbr",
		"solto.cbz",
	)
	fx.job(t, "Marvel/guerras_secretas/1984/Capítulo 02.pdf", "running")
	fx.job(t, "Marvel/guerras_secretas/1984/Capítulo 03.pdf", "failed")
	ctx := context.Background()

	all := mustSeries(t, fx, Filter{})
	if all.Total != 4 {
		t.Fatalf("séries: %d, quer 4: %+v", all.Total, all.Items)
	}
	byKey := map[string]Series{}
	for _, s := range all.Items {
		byKey[s.Key] = s
	}
	gs := byKey["Marvel/guerras_secretas/1984"]
	if gs.Name != "guerras_secretas / 1984" || gs.Publisher != "Marvel" || gs.Count != 5 || gs.Formats["pdf"] != 4 {
		t.Errorf("série Guerras Secretas: %+v", gs)
	}
	cover, _, _ := fx.cat.Get(ctx, gs.CoverFileID)
	if cover.Format != "cbz" {
		t.Errorf("capa deveria preferir o CBZ, veio %s", cover.RelPath)
	}
	if byKey[""].Name != "HD" || byKey["Marvel"].Name != "Marvel" {
		t.Errorf("nomes: raiz=%q marvel=%q", byKey[""].Name, byKey["Marvel"].Name)
	}

	status := map[string]string{}
	files, _ := fx.cat.ListFiles(ctx, Filter{Limit: 50}, nil)
	for _, f := range files.Items {
		status[f.Title+"."+f.Format] = f.Status
	}
	want := map[string]string{
		"Capítulo 01.pdf": StatusConverted, "Capítulo 01.cbz": StatusReady, "Capítulo 02.pdf": StatusConverting,
		"Capítulo 03.pdf": StatusFailed, "Capítulo 04.pdf": StatusPending,
	}
	for k, v := range want {
		if status[k] != v {
			t.Errorf("%s: status %q, quer %q", k, status[k], v)
		}
	}

	checks := []struct {
		name string
		f    Filter
		want int // nº de arquivos
	}{
		{"editora", Filter{Publisher: "DC Comics"}, 1},
		{"formato", Filter{Format: "cbr"}, 2},
		{"status", Filter{Status: StatusPending}, 1},
		{"busca sem acento", Filter{Query: "capitulo"}, 5},
		{"busca por prefixo e série", Filter{Query: "guerras 1984 cap"}, 5},
		{"busca + formato", Filter{Query: "batman", Format: "cbr"}, 1},
		{"combinado sem resultado", Filter{Publisher: "DC Comics", Format: "pdf"}, 0},
	}
	for _, c := range checks {
		c.f.Limit = 50
		p, err := fx.cat.ListFiles(ctx, c.f, nil)
		if err != nil || p.Total != c.want || len(p.Items) != c.want {
			t.Errorf("%s: total %d (itens %d), quer %d (%v)", c.name, p.Total, len(p.Items), c.want, err)
		}
	}

	key := "Marvel/guerras_secretas/1984"
	p, _ := fx.cat.ListFiles(ctx, Filter{LibraryID: fx.lib.ID, Limit: 2}, &key)
	if p.Total != 5 || len(p.Items) != 2 || p.Items[0].Title != "Capítulo 01" {
		t.Errorf("arquivos da série paginados: %+v", p)
	}
	pubs, _ := fx.cat.Publishers(ctx, 0)
	if fmt.Sprint(pubs) != "[DC Comics Marvel]" {
		t.Errorf("editoras: %v", pubs)
	}
}

func mustSeries(t *testing.T, fx fixture, f Filter) Page[Series] {
	t.Helper()
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	p, err := fx.cat.ListSeries(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Critério do M4: com 10 mil itens, a listagem paginada responde em < 200 ms.
func TestListPerformance10k(t *testing.T) {
	if testing.Short() {
		t.Skip("teste de desempenho")
	}
	var files []string
	for i := range 10000 {
		files = append(files, fmt.Sprintf("Editora%d/Serie%03d/Edicao %05d.pdf", i%10, i%500, i))
	}
	fx := newFixture(t, files...)
	for i := range 2000 { // uma parte já com histórico de conversão
		fx.job(t, files[i], []string{"done", "failed"}[i%2])
	}
	ctx := context.Background()

	measure := func(name string, fn func() error) {
		start := time.Now()
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		d := time.Since(start)
		t.Logf("%s: %v", name, d)
		if d > 200*time.Millisecond {
			t.Errorf("%s levou %v (limite 200 ms)", name, d)
		}
	}
	measure("séries, 1ª página", func() error { _, err := fx.cat.ListSeries(ctx, Filter{Limit: 60}); return err })
	measure("séries, última página", func() error { _, err := fx.cat.ListSeries(ctx, Filter{Limit: 60, Offset: 480}); return err })
	measure("séries com status=failed", func() error {
		p, err := fx.cat.ListSeries(ctx, Filter{Limit: 60, Status: StatusFailed})
		if err == nil && p.Total == 0 {
			err = fmt.Errorf("esperava séries com falha")
		}
		return err
	})
	measure("arquivos com busca", func() error {
		_, err := fx.cat.ListFiles(ctx, Filter{Limit: 60, Query: "edicao 0999"}, nil)
		return err
	})
	measure("arquivos de uma editora", func() error {
		_, err := fx.cat.ListFiles(ctx, Filter{Limit: 60, Publisher: "Editora3"}, nil)
		return err
	})
}
