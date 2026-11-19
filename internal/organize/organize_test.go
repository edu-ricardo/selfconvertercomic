package organize

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

const tpl = "{serie} ({ano})/{serie} ({ano}) #{numero}"

type env struct {
	conn *sql.DB
	libs *library.Store
	lib  library.Library
	root string
	org  *Organizer
}

func newEnv(t *testing.T, files ...string) *env {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	e := &env{conn: conn, libs: library.NewStore(conn), root: t.TempDir()}
	for _, f := range files {
		e.write(t, f, f)
	}
	e.lib, err = e.libs.Create(context.Background(), "HD", e.root, false)
	if err != nil {
		t.Fatal(err)
	}
	e.scan(t)
	e.org = &Organizer{DB: conn, Libraries: e.libs, Exclusive: func(_ int64, fn func() error) error { return fn() }}
	return e
}

func (e *env) write(t *testing.T, rel, content string) {
	t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *env) scan(t *testing.T) library.Result {
	t.Helper()
	r, err := e.libs.Scan(context.Background(), e.lib)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *env) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(rel)))
	return err == nil
}

func (e *env) content(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func editionID(t *testing.T, conn *sql.DB, key string) int64 {
	t.Helper()
	var id int64
	if err := conn.QueryRow("SELECT id FROM editions WHERE dir || '/' || stem = ?", key).Scan(&id); err != nil {
		t.Fatalf("edição %s: %v", key, err)
	}
	return id
}

const gs = "Marvel/guerras_secretas/1984/"

func TestPlanApplyUndo(t *testing.T) {
	e := newEnv(t,
		gs+"Capítulo 01.pdf", gs+"Capítulo 01.cbz", // com metadados das fontes
		gs+"Capítulo 02.pdf",                                           // só pelo nome
		"arquivospdf/"+gs+"Capítulo 02.pdf",                            // cópia na árvore duplicada → mesmo destino
		gs+"Capítulo 03.cbz",                                           // destino já ocupado
		"Guerras Secretas (1984)/Guerras Secretas (1984) #3.cbz",       // já organizado
		"Marvel/Grandes Eventos/Mark Millar - Guerra Civil (2022).cbr", // sem número
	)
	ctx := context.Background()
	ed1 := editionID(t, e.conn, gs+"Capítulo 01")
	e.conn.Exec(`UPDATE editions SET meta_status = 'applied', meta_json = ? WHERE id = ?`,
		`{"provider":"gcd","series":"Secret Wars (Guerras Secretas)","number":"1","year":1986,"publisher":"Editora Abril S.A."}`, ed1)

	p, err := e.org.Plan(ctx, e.lib.ID, "", tpl)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Item{}
	for _, it := range p.Items {
		got[it.From] = it
	}
	want := map[string]struct{ to, problem, source string }{
		gs + "Capítulo 01":                  {"Secret Wars (Guerras Secretas) (1986)/Secret Wars (Guerras Secretas) (1986) #1", "", "fontes"},
		gs + "Capítulo 02":                  {"Guerras Secretas (1984)/Guerras Secretas (1984) #2", "outra edição", "nome"},
		"arquivospdf/" + gs + "Capítulo 02": {"Guerras Secretas (1984)/Guerras Secretas (1984) #2", "outra edição", "nome"},
		gs + "Capítulo 03":                  {"Guerras Secretas (1984)/Guerras Secretas (1984) #3", "já existe", "nome"},
		"Marvel/Grandes Eventos/Mark Millar - Guerra Civil (2022)": {"Mark Millar - Guerra Civil (2022)/Mark Millar - Guerra Civil (2022)", "falta número", "nome"},
	}
	if len(got) != len(want) || p.Unchanged != 1 {
		t.Fatalf("plano: %d itens, %d sem mudança: %+v", len(got), p.Unchanged, p.Items)
	}
	for from, w := range want {
		it := got[from]
		if it.To != w.to || it.Source != w.source || (w.problem == "") != (it.Problem == "") || !strings.Contains(it.Problem, w.problem) {
			t.Errorf("%s:\n  %+v\n  quer %+v", from, it, w)
		}
	}
	if len(got[gs+"Capítulo 01"].Files) != 2 {
		t.Errorf("PDF e CBZ deveriam ir juntos: %+v", got[gs+"Capítulo 01"].Files)
	}

	// Aplicar tudo: só o que não tem problema anda.
	var all []int64
	for _, it := range p.Items {
		all = append(all, it.EditionID)
	}
	op, err := e.org.Apply(ctx, e.lib.ID, tpl, all)
	if err != nil || op.Status != "done" || op.Moves != 2 || op.Editions != 1 {
		t.Fatalf("aplicar: %+v %v", op, err)
	}
	const to = "Secret Wars (Guerras Secretas) (1986)/Secret Wars (Guerras Secretas) (1986) #1"
	if !e.exists(to+".pdf") || !e.exists(to+".cbz") || e.exists(gs+"Capítulo 01.pdf") || e.content(t, to+".cbz") != gs+"Capítulo 01.cbz" {
		t.Fatal("arquivos não foram movidos como esperado")
	}
	if !e.exists(gs+"Capítulo 03.cbz") || e.content(t, "Guerras Secretas (1984)/Guerras Secretas (1984) #3.cbz") != "Guerras Secretas (1984)/Guerras Secretas (1984) #3.cbz" {
		t.Fatal("arquivo com conflito foi tocado")
	}
	// O banco acompanha: a varredura seguinte não vê nada novo nem sumido, e a edição mantém os metadados.
	if r := e.scan(t); r.Added != 0 || r.Removed != 0 {
		t.Errorf("varredura depois de organizar: %+v", r)
	}
	var status string
	e.conn.QueryRow("SELECT meta_status FROM editions WHERE id = ?", ed1).Scan(&status)
	if editionID(t, e.conn, to) != ed1 || status != "applied" {
		t.Errorf("a edição deveria ser a mesma, com os metadados: status %q", status)
	}

	// Desfazer volta tudo.
	op, err = e.org.Undo(ctx, op.ID)
	if err != nil || op.Status != "undone" || op.UndoneAt == nil {
		t.Fatalf("desfazer: %+v %v", op, err)
	}
	if !e.exists(gs+"Capítulo 01.pdf") || !e.exists(gs+"Capítulo 01.cbz") || e.exists(to+".pdf") {
		t.Fatal("desfazer não devolveu os arquivos")
	}
	if e.exists("Secret Wars (Guerras Secretas) (1986)") {
		t.Error("a pasta criada e esvaziada deveria sumir")
	}
	if r := e.scan(t); r.Added != 0 || r.Removed != 0 {
		t.Errorf("varredura depois de desfazer: %+v", r)
	}
	if editionID(t, e.conn, gs+"Capítulo 01") != ed1 {
		t.Error("desfazer deveria voltar a mesma edição")
	}
	if _, err := e.org.Undo(ctx, op.ID); !errors.Is(err, ErrCannotUndo) {
		t.Errorf("desfazer duas vezes: %v", err)
	}

	// Algo mudou depois de aplicar (um arquivo novo no lugar antigo): não desfaz nada.
	op, _ = e.org.Apply(ctx, e.lib.ID, tpl, []int64{ed1})
	e.write(t, gs+"Capítulo 01.pdf", "outro")
	if _, err := e.org.Undo(ctx, op.ID); !errors.Is(err, ErrCannotUndo) {
		t.Errorf("desfazer com o lugar ocupado: %v", err)
	}
	if !e.exists(to+".pdf") || !e.exists(to+".cbz") || e.content(t, gs+"Capítulo 01.pdf") != "outro" {
		t.Error("desfazer recusado não deveria mexer em nada")
	}

	hist, _ := e.org.History(ctx, e.lib.ID, 10)
	if len(hist) != 2 || hist[0].ID != op.ID {
		t.Errorf("histórico: %+v", hist)
	}
}

func TestPlanSeriesAndErrors(t *testing.T) {
	e := newEnv(t, gs+"Capítulo 01.pdf", "Marvel/Kitty Pryde/2001/Capítulo 01.pdf")
	ctx := context.Background()
	p, err := e.org.Plan(ctx, e.lib.ID, "kittypryde", tpl)
	if err != nil || len(p.Items) != 1 || p.Items[0].To != "Kitty Pryde (2001)/Kitty Pryde (2001) #1" {
		t.Fatalf("plano de uma série: %+v %v", p, err)
	}
	if _, err := e.org.Plan(ctx, e.lib.ID, "", "{autor}/{serie}"); err == nil {
		t.Error("template com variável desconhecida deveria ser recusado")
	}
	e.conn.Exec("UPDATE libraries SET available = 0")
	if _, err := e.org.Plan(ctx, e.lib.ID, "", tpl); !errors.Is(err, ErrUnavailable) {
		t.Errorf("biblioteca indisponível: %v", err)
	}
}
