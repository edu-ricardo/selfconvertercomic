package identify

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

// GCD falso no formato real: "Guerras Secretas" acha a edição da Abril (com créditos); "Os Novos
// Vingadores" só acha "Os Vingadores" (nome parecido, outra série → tarefa).
func fakeGCD(hits *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b := "http://" + r.Host + "/api"
		switch r.URL.Path {
		case "/api/series/name/Guerras Secretas/":
			fmt.Fprintf(w, `{"results":[{"api_url":"%[1]s/series/41527/","name":"Secret Wars (Guerras Secretas)","language":"pt",
				"year_began":"1986","active_issues":["%[1]s/issue/726769/","%[1]s/issue/726770/","%[1]s/issue/726771/"],
				"issue_descriptors":["1","2","3"]}]}`, b)
		case "/api/series/name/Os Novos Vingadores/":
			fmt.Fprintf(w, `{"results":[{"api_url":"%[1]s/series/900/","name":"Os Vingadores","language":"pt","year_began":2015,
				"active_issues":["%[1]s/issue/901/"],"issue_descriptors":["25"]}]}`, b)
		case "/api/series/41527/", "/api/series/900/":
			fmt.Fprintf(w, `{"name":"x","language":"pt","year_began":"1986","publisher":"%s/publisher/4738/"}`, b)
		case "/api/publisher/4738/":
			fmt.Fprint(w, `{"name":"Editora Abril"}`)
		case "/api/issue/726771/":
			// Cadastro incompleto (como as #3–#12 reais da Abril): sem data, editora nem histórias.
			fmt.Fprintf(w, `{"api_url":"%s/issue/726771/","number":"3","title":"","key_date":"","indicia_publisher":"",
				"series":"%s/series/41527/","cover":"","story_set":[]}`, b, b)
		case "/api/issue/726769/", "/api/issue/726770/", "/api/issue/901/":
			id := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[2]
			num := map[string]string{"726769": "1", "726770": "2", "901": "25"}[id]
			fmt.Fprintf(w, `{"api_url":"%s/issue/%s/","series_name":"Secret Wars (Guerras Secretas) (1986 series)","number":"%s",
				"title":"","key_date":"1986-08-12","indicia_publisher":"Editora Abril S.A.","series":"%s/series/41527/",
				"cover":"https://files1.comics.org/x.jpg","story_set":[
				 {"type":"cover","pencils":"Mike Zeck","script":"None"},
				 {"type":"comic story","title":"A Guerra Se Inicia","script":"Jim Shooter (credited)","pencils":"Mike Zeck (credited as Michael Zeck)",
				  "inks":"John Beatty","colors":"?","letters":"Joe Rosen","synopsis":"Heróis e vilões vão para o Mundo de Batalha."}]}`, b, id, num, b)
		default:
			http.NotFound(w, r)
		}
	}))
}

type env struct {
	conn  *sql.DB
	libs  *library.Store
	lib   library.Library
	root  string
	cfg   settings.Settings
	id    *Identifier
	tasks *tasks.Store
	hits  *atomic.Int32
}

func newEnv(t *testing.T) *env {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	e := &env{conn: conn, libs: library.NewStore(conn), root: t.TempDir(), cfg: settings.Defaults(), hits: &atomic.Int32{},
		tasks: tasks.NewStore(conn)}
	e.cfg.Metadata.RateSeconds = 0
	srv := fakeGCD(e.hits)
	t.Cleanup(srv.Close)
	finder := &metadata.Finder{
		GCD:       &metadata.GCD{F: &metadata.Fetcher{Provider: "GCD", DB: conn, Client: http.DefaultClient, UserAgent: "t"}, BaseURL: srv.URL + "/api"},
		ComicVine: &metadata.ComicVine{F: &metadata.Fetcher{Provider: "ComicVine", DB: conn, Client: http.DefaultClient, UserAgent: "t"}},
		Settings:  func(context.Context) (settings.Settings, error) { return e.cfg, nil },
	}
	e.id = &Identifier{DB: conn, Finder: finder, Tasks: e.tasks,
		Settings: func(context.Context) (settings.Settings, error) { return e.cfg, nil }}
	e.lib, err = e.libs.Create(context.Background(), "HD", e.root, false)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) cbz(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for i := range 2 {
		w, _ := zw.Create(fmt.Sprintf("%03d.jpg", i))
		w.Write([]byte{0xFF, 0xD8, 0xFF, byte(i)})
	}
	zw.Close()
	f.Close()
	return p
}

func (e *env) file(t *testing.T, rel string) {
	t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("%PDF"), 0o644)
}

func (e *env) scan(t *testing.T) {
	t.Helper()
	if _, err := e.libs.Scan(context.Background(), e.lib); err != nil {
		t.Fatal(err)
	}
}

// drain processa a fila até esvaziar (o que Run faz em segundo plano).
func (e *env) drain(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for range 50 {
		id, status, err := e.id.next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if id == 0 {
			return
		}
		if err := e.id.process(ctx, id, status); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("a fila não esvaziou")
}

func (e *env) edition(t *testing.T, key string) Edition {
	t.Helper()
	var id int64
	if err := e.conn.QueryRow("SELECT id FROM editions WHERE dir || '/' || stem = ?", key).Scan(&id); err != nil {
		t.Fatalf("edição %s: %v", key, err)
	}
	ed, err := e.id.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ed
}

func comicInfoOf(t *testing.T, path string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "ComicInfo.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	return ""
}

func TestIdentifyAppliesOrOpensTask(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const gs = "Marvel/guerras_secretas/1984/"
	cbz1 := e.cbz(t, gs+"Capítulo 01.cbz")
	e.file(t, gs+"Capítulo 01.pdf")
	e.file(t, gs+"Capítulo 02.pdf") // só PDF por enquanto
	e.cbz(t, "Marvel/Tempo/Os Novos Vingadores v3 #25 (MK-SQ).cbz")
	e.scan(t)
	e.drain(t)

	// Confiança alta → aplicado, ComicInfo.xml gravado no CBZ, sem tarefa.
	ed := e.edition(t, gs+"Capítulo 01")
	if ed.Status != StatusApplied || ed.Score < metadata.AutoApplyScore || ed.Applied == nil || ed.Applied.Title != "A Guerra Se Inicia" {
		t.Fatalf("Capítulo 01: %+v", ed)
	}
	ci := comicInfoOf(t, cbz1)
	for _, want := range []string{"<Series>Secret Wars (Guerras Secretas)</Series>", "<Number>1</Number>", "<Title>A Guerra Se Inicia</Title>",
		"<Writer>Jim Shooter</Writer>", "<Penciller>Mike Zeck</Penciller>", "<Inker>John Beatty</Inker>", "<Letterer>Joe Rosen</Letterer>",
		"<CoverArtist>Mike Zeck</CoverArtist>", "<Publisher>Editora Abril</Publisher>", "<Year>1986</Year>", "<Month>8</Month>",
		"<LanguageISO>pt</LanguageISO>", "<Web>", "/issue/726769/", "<PageCount>2</PageCount>", "CC BY-SA"} {
		if !strings.Contains(ci, want) {
			t.Errorf("ComicInfo sem %q:\n%s", want, ci)
		}
	}
	if strings.Contains(ci, "<Colorist>") {
		t.Errorf("cores desconhecidas (\"?\") não deveriam aparecer:\n%s", ci)
	}

	// Edição só com PDF: aplicada no banco; quando o CBZ aparece, o ComicInfo é gravado nele.
	if ed := e.edition(t, gs+"Capítulo 02"); ed.Status != StatusApplied {
		t.Fatalf("Capítulo 02 (só PDF): %+v", ed)
	}
	cbz2 := e.cbz(t, gs+"Capítulo 02.cbz")
	e.scan(t)
	if ed := e.edition(t, gs+"Capítulo 02"); ed.Status != StatusRewrite {
		t.Fatalf("CBZ novo numa edição aplicada deveria pedir regravação: %+v", ed)
	}
	hits := e.hits.Load()
	e.drain(t)
	if ci := comicInfoOf(t, cbz2); !strings.Contains(ci, "<Number>2</Number>") || e.hits.Load() != hits {
		t.Errorf("regravação: pedidos %d, ComicInfo:\n%s", e.hits.Load()-hits, ci)
	}

	// Edição com cadastro incompleto no GCD: ano e editora vêm da série (todas as edições ficam iguais).
	cbz3 := e.cbz(t, gs+"Capítulo 03.cbz")
	e.scan(t)
	e.drain(t)
	if ed := e.edition(t, gs+"Capítulo 03"); ed.Status != StatusApplied || ed.Applied.Year != 1986 || ed.Applied.Publisher != "Editora Abril" {
		t.Errorf("Capítulo 03 (cadastro incompleto): %+v", ed.Applied)
	}
	if ci := comicInfoOf(t, cbz3); !strings.Contains(ci, "<Year>1986</Year>") || !strings.Contains(ci, "<Publisher>Editora Abril</Publisher>") {
		t.Errorf("ComicInfo do Capítulo 03:\n%s", ci)
	}

	// Confiança baixa → tarefa com os candidatos, ligada a um arquivo da edição.
	const nv = "Marvel/Tempo/Os Novos Vingadores v3 #25 (MK-SQ)"
	ed = e.edition(t, nv)
	if ed.Status != StatusTask || len(ed.Candidates) != 1 || ed.Candidates[0].Series != "Os Vingadores" || ed.Applied != nil {
		t.Fatalf("Os Novos Vingadores: %+v", ed)
	}
	open, _ := e.tasks.List(ctx, tasks.Open, 10)
	if len(open) != 1 || open[0].Kind != tasks.KindMetadataConfirm || open[0].RelPath != nv || open[0].RefID == 0 ||
		open[0].Title != "Confirmar metadados: Os Novos Vingadores #25" {
		t.Fatalf("tarefa: %+v", open)
	}

	// O usuário confirma o candidato → aplicado e a tarefa fecha.
	c := ed.Candidates[0]
	if err := e.id.Apply(ctx, ed.ID, c.Provider, c.IssueID, c.Score, nil); err != nil {
		t.Fatal(err)
	}
	if ed := e.edition(t, nv); ed.Status != StatusApplied || len(ed.Candidates) != 1 {
		t.Errorf("depois de confirmar: %+v", ed)
	}
	if n, _ := e.tasks.CountOpen(ctx); n != 0 {
		t.Errorf("tarefa deveria fechar: %d abertas", n)
	}
}

func TestIdentifyQueueAndSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.cbz(t, "Marvel/guerras_secretas/1984/Capítulo 01.cbz")
	e.scan(t)
	key := "Marvel/guerras_secretas/1984/Capítulo 01"

	// Identificação automática desligada: a edição nova espera.
	e.cfg.Metadata.AutoIdentify = false
	e.drain(t)
	if ed := e.edition(t, key); ed.Status != StatusNew || e.hits.Load() != 0 {
		t.Fatalf("com a automática desligada: %+v (pedidos %d)", ed, e.hits.Load())
	}
	// "Identificar biblioteca" põe na fila mesmo assim.
	if n, err := e.id.QueueLibrary(ctx, e.lib.ID); err != nil || n != 1 {
		t.Fatalf("fila da biblioteca: %d %v", n, err)
	}
	e.drain(t)
	if ed := e.edition(t, key); ed.Status != StatusApplied {
		t.Fatalf("depois de pedir: %+v", ed)
	}
	// Já aplicada: a biblioteca não refaz; a série refaz só as sem metadados; a edição refaz sempre.
	if n, _ := e.id.QueueLibrary(ctx, e.lib.ID); n != 0 {
		t.Errorf("biblioteca refez edição já aplicada: %d", n)
	}
	if n, _ := e.id.QueueSeries(ctx, e.lib.ID, "guerrassecretas"); n != 0 {
		t.Errorf("série refez edição já aplicada: %d", n)
	}
	if err := e.id.QueueEdition(ctx, e.edition(t, key).ID); err != nil || e.edition(t, key).Status != StatusQueued {
		t.Errorf("refazer a edição: %v", err)
	}

	// Sem fontes ligadas: erro explicado, a edição sai da fila.
	e.cfg.Metadata.GCDEnabled = false
	e.drain(t)
	if ed := e.edition(t, key); ed.Status != StatusError || !strings.Contains(ed.Error, "nenhuma fonte") {
		t.Errorf("sem fontes: %+v", ed)
	}

	// Biblioteca indisponível: nada é processado.
	e.conn.Exec("UPDATE libraries SET available = 0")
	e.id.QueueLibrary(ctx, e.lib.ID)
	if id, _, _ := e.id.next(ctx); id != 0 {
		t.Error("não deveria processar edições de biblioteca indisponível")
	}
}

func TestConfident(t *testing.T) {
	c := func(series string, year, score int) metadata.Candidate {
		return metadata.Candidate{Provider: "gcd", IssueID: "1", Series: series, Number: "1", Year: year, Score: score}
	}
	cases := []struct {
		name  string
		cands []metadata.Candidate
		want  bool
	}{
		{"único acima do limiar", []metadata.Candidate{c("A", 2015, 90), c("B", 1990, 60)}, true},
		{"abaixo do limiar", []metadata.Candidate{c("A", 2015, 84)}, false},
		{"duas fontes concordam", []metadata.Candidate{c("Magneto", 2015, 95), {Provider: "comicvine", IssueID: "9", Series: "Magneto", Number: "1", Year: 2015, Score: 95}}, true},
		{"duas séries diferentes acima do limiar", []metadata.Candidate{c("A", 2015, 90), c("B", 2015, 88)}, false},
		{"só a série, sem edição", []metadata.Candidate{{Series: "A", Score: 90}}, false},
		{"nada", nil, false},
	}
	for _, tc := range cases {
		if _, ok := confident(tc.cands); ok != tc.want {
			t.Errorf("%s: %v, quer %v", tc.name, ok, tc.want)
		}
	}
}

// Fonte lenta/fora do ar ou no limite: a edição continua na fila (Run pausa e tenta depois), sem erro.
func TestIdentifyTransientKeepsQueued(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for code, want := range map[int]error{503: metadata.ErrUnavailable, 429: metadata.ErrRateLimited} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		e.id.Finder.GCD = &metadata.GCD{F: &metadata.Fetcher{Provider: "GCD", DB: e.conn, Client: http.DefaultClient, UserAgent: "t"},
			BaseURL: srv.URL + "/api"}
		rel := fmt.Sprintf("Marvel/Teste %d/Capítulo 01.cbz", code)
		e.cbz(t, rel)
		e.scan(t)
		key := strings.TrimSuffix(rel, ".cbz")
		id, status, _ := e.id.next(ctx)
		if err := e.id.process(ctx, id, status); !errors.Is(err, want) {
			t.Errorf("HTTP %d: %v, quer %v", code, err, want)
		}
		if ed := e.edition(t, key); ed.Status != StatusNew || ed.Error != "" {
			t.Errorf("HTTP %d: a edição deveria continuar esperando: %+v", code, ed)
		}
		srv.Close()
		e.conn.Exec("UPDATE editions SET meta_status = 'applied'") // tira da frente para o próximo caso
	}
}

func TestApplyManual(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.cfg.Metadata.AutoIdentify = false
	cbz := e.cbz(t, "Marvel/Grandes Eventos/Mark Millar - Guerra Civil (2022).cbz")
	e.scan(t)
	ed := e.edition(t, "Marvel/Grandes Eventos/Mark Millar - Guerra Civil (2022)")
	if err := e.id.ApplyManual(ctx, ed.ID, metadata.Details{Series: "  ", Number: "1"}); !errors.Is(err, ErrManualSeries) {
		t.Errorf("sem série: %v", err)
	}
	err := e.id.ApplyManual(ctx, ed.ID, metadata.Details{Provider: "gcd", IssueID: "9", Series: " Guerra Civil ", Number: "#1",
		Year: 2022, Month: 3, Publisher: "Panini", Writers: []string{" Mark Millar ", ""}, Pencillers: []string{"Steve McNiven"}})
	if err != nil {
		t.Fatal(err)
	}
	ed = e.edition(t, "Marvel/Grandes Eventos/Mark Millar - Guerra Civil (2022)")
	if ed.Status != StatusApplied || ed.Score != ManualScore || ed.Applied.Provider != "manual" || ed.Applied.IssueID != "" ||
		ed.Applied.Number != "1" || len(ed.Applied.Writers) != 1 {
		t.Fatalf("manual: %+v %+v", ed, ed.Applied)
	}
	ci := comicInfoOf(t, cbz)
	for _, want := range []string{"<Series>Guerra Civil</Series>", "<Number>1</Number>", "<Writer>Mark Millar</Writer>",
		"<Penciller>Steve McNiven</Penciller>", "<Publisher>Panini</Publisher>", "preenchidos à mão"} {
		if !strings.Contains(ci, want) {
			t.Errorf("ComicInfo sem %q:\n%s", want, ci)
		}
	}
	if e.hits.Load() != 0 {
		t.Errorf("metadados manuais não deveriam consultar fontes: %d pedidos", e.hits.Load())
	}
}
