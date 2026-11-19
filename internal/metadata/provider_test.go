package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

// Respostas no formato real das APIs (campos que usamos), conferidas em 2026-11-19.
func fakeGCD(t *testing.T, hits *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		base := "http://" + r.Host + "/api"
		switch r.URL.Path {
		case "/api/series/name/Guerras Secretas/":
			fmt.Fprintf(w, `{"count":2,"next":null,"results":[
			 {"api_url":"%[1]s/series/171534/","name":"Guerras Secretas","country":"br","language":"pt","year_began":2016,
			  "active_issues":["%[1]s/issue/2214216/"],"issue_descriptors":["1"]},
			 {"api_url":"%[1]s/series/41527/","name":"Secret Wars (Guerras Secretas)","country":"br","language":"pt","year_began":"1986",
			  "active_issues":["%[1]s/issue/726769/","%[1]s/issue/726770/"],"issue_descriptors":["1","2"]}]}`, base)
		case "/api/issue/726769/":
			fmt.Fprintf(w, `{"api_url":"%s/issue/726769/","series_name":"Secret Wars (Guerras Secretas) (1986 series)","number":"1",
			 "title":"","key_date":"1986-08-12","indicia_publisher":"Editora Abril S.A.",
			 "cover":"https://files1.comics.org/img/gcd/covers_by_id/989/w400/989158.jpg",
			 "story_set":[{"type":"cover","title":""},{"type":"comic story","title":"A Guerra Se Inicia"}]}`, base)
		case "/api/issue/2214216/":
			fmt.Fprintf(w, `{"api_url":"%s/issue/2214216/","number":"1","key_date":"","indicia_publisher":"Panini Brasil Ltda.","cover":"","story_set":[]}`, base)
		default:
			http.NotFound(w, r)
		}
	}))
}

func fakeComicVine(t *testing.T, hits *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.UserAgent() == "" || strings.HasPrefix(r.UserAgent(), "Go-http-client") {
			t.Errorf("User-Agent próprio é obrigatório no ComicVine: %q", r.UserAgent())
		}
		if r.URL.Query().Get("api_key") != "boa" {
			fmt.Fprint(w, `{"error":"Invalid API Key","status_code":100,"results":[]}`)
			return
		}
		switch r.URL.Path {
		case "/api/types/":
			fmt.Fprint(w, `{"error":"OK","status_code":1,"results":[]}`)
		case "/api/search/":
			if r.URL.Query().Get("query") != "Guerras Secretas" {
				t.Errorf("busca: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"error":"OK","status_code":1,"results":[{"id":2895,"name":"Secret Wars","start_year":"1984",
			 "count_of_issues":12,"publisher":{"name":"Marvel"},"site_detail_url":"https://comicvine.gamespot.com/secret-wars/4050-2895/"}]}`)
		case "/api/issues/":
			if f := r.URL.Query().Get("filter"); f != "volume:2895,issue_number:1" {
				t.Errorf("filtro: %s", f)
			}
			fmt.Fprint(w, `{"error":"OK","status_code":1,"results":[{"id":31744,"name":"The War Begins","issue_number":"1",
			 "cover_date":"1984-05-01","image":{"medium_url":"https://comicvine.gamespot.com/a/cover.jpg"},
			 "site_detail_url":"https://comicvine.gamespot.com/secret-wars-1/4000-31744/"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

type env struct {
	conn         *sql.DB
	finder       *Finder
	cfg          settings.Settings
	gcdHits, cvH *atomic.Int32
}

func newEnv(t *testing.T) *env {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	e := &env{conn: conn, gcdHits: &atomic.Int32{}, cvH: &atomic.Int32{}, cfg: settings.Defaults()}
	gcd, cv := fakeGCD(t, e.gcdHits), fakeComicVine(t, e.cvH)
	t.Cleanup(gcd.Close)
	t.Cleanup(cv.Close)
	e.cfg.Metadata.ComicVineAPIKey = "boa"
	e.cfg.Metadata.RateSeconds = 1
	fetcher := func(p string) *Fetcher {
		return &Fetcher{Provider: p, DB: conn, Client: http.DefaultClient, UserAgent: "QuadrinhoTeca/test"}
	}
	e.finder = &Finder{
		ComicVine: &ComicVine{F: fetcher("ComicVine"), BaseURL: cv.URL + "/api"},
		GCD:       &GCD{F: fetcher("GCD"), BaseURL: gcd.URL + "/api"},
		Settings:  func(context.Context) (settings.Settings, error) { return e.cfg, nil },
	}
	return e
}

func TestFinderSearch(t *testing.T) {
	e := newEnv(t)
	e.cfg.Metadata.RateSeconds = 0 // ritmo testado à parte
	ctx := context.Background()
	g := Guess{Series: "Guerras Secretas", Number: "1", Year: 1984}

	res, err := e.finder.Search(ctx, g)
	if err != nil || len(res.Errors) > 0 {
		t.Fatalf("busca: %v %v", err, res.Errors)
	}
	var got []string
	for _, c := range res.Candidates {
		got = append(got, fmt.Sprintf("%s %s #%s %d %d", c.Provider, c.Series, c.Number, c.Year, c.Score))
	}
	want := []string{
		"gcd Secret Wars (Guerras Secretas) #1 1986 85", // edição BR do ano certo: a preferida
		"gcd Guerras Secretas #1 2016 65",               // mesmo nome, mas de 2016
		"comicvine Secret Wars #1 1984 59",              // original em inglês: nome diferente
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("candidatos:\n%s\nquer:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	top := res.Candidates[0]
	if top.Title != "A Guerra Se Inicia" || top.Publisher != "Editora Abril S.A." || !strings.HasSuffix(top.URL, "/issue/726769/") ||
		strings.Contains(top.URL, "/api/") || top.CoverURL == "" {
		t.Errorf("detalhes do GCD: %+v", top)
	}
	cv := res.Candidates[2]
	if cv.Title != "The War Begins" || cv.Publisher != "Marvel" || cv.IssueID != "31744" || cv.CoverURL == "" {
		t.Errorf("detalhes do ComicVine: %+v", cv)
	}

	// Segunda busca: tudo do cache, nenhum pedido.
	g0, c0 := e.gcdHits.Load(), e.cvH.Load()
	if _, err := e.finder.Search(ctx, g); err != nil {
		t.Fatal(err)
	}
	if e.gcdHits.Load() != g0 || e.cvH.Load() != c0 {
		t.Errorf("segunda busca fez pedidos: GCD %d, ComicVine %d", e.gcdHits.Load()-g0, e.cvH.Load()-c0)
	}
	var keys string
	e.conn.QueryRow("SELECT group_concat(key, ' | ') FROM metadata_cache").Scan(&keys)
	if strings.Contains(keys, "boa") {
		t.Errorf("a chave de API não pode ir para o cache: %s", keys)
	}

	// Cache vencido: busca de novo.
	e.cfg.Metadata.CacheDays = 1
	e.finder.GCD.F.Now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	e.finder.Search(ctx, g)
	if e.gcdHits.Load() == g0 {
		t.Error("cache vencido deveria refazer o pedido")
	}

	// Sem ano: nunca chega à confiança alta, mesmo com nome e número iguais.
	res, _ = e.finder.Search(ctx, Guess{Series: "Guerras Secretas", Number: "1"})
	for _, c := range res.Candidates {
		if c.Score > maxWithoutYear {
			t.Errorf("sem ano: %s %d", c.Series, c.Score)
		}
	}

	// Sem número: só as séries, sem abrir edições.
	g0 = e.gcdHits.Load()
	res, _ = e.finder.Search(ctx, Guess{Series: "Guerras Secretas", Year: 1986})
	if len(res.Candidates) == 0 || res.Candidates[0].IssueID != "" || e.gcdHits.Load() != g0 {
		t.Errorf("sem número: %+v (pedidos %d)", res.Candidates, e.gcdHits.Load()-g0)
	}
}

func TestFinderErrors(t *testing.T) {
	e := newEnv(t)
	e.cfg.Metadata.RateSeconds = 0
	ctx := context.Background()

	// Chave ruim: o ComicVine falha com mensagem clara e o GCD continua respondendo.
	e.cfg.Metadata.ComicVineAPIKey = "ruim"
	res, err := e.finder.Search(ctx, Guess{Series: "Guerras Secretas", Number: "1"})
	if err != nil || len(res.Candidates) != 2 || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "chave de API inválida") {
		t.Errorf("chave ruim: %v %+v", err, res)
	}
	if err := e.finder.TestComicVine(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("testar chave ruim: %v", err)
	}
	e.cfg.Metadata.ComicVineAPIKey = "boa"
	if err := e.finder.TestComicVine(ctx); err != nil {
		t.Errorf("testar chave boa: %v", err)
	}

	e.cfg.Metadata.ComicVineAPIKey, e.cfg.Metadata.GCDEnabled = "", false
	if _, err := e.finder.Search(ctx, Guess{Series: "x"}); !errors.Is(err, ErrNoProvider) {
		t.Errorf("sem provedores: %v", err)
	}

	// Códigos HTTP → tipo de erro; nada vai para o cache.
	for code, want := range map[int]error{420: ErrRateLimited, 429: ErrRateLimited, 503: ErrUnavailable, 401: ErrUnauthorized} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		f := &Fetcher{Provider: "X", DB: e.conn, Client: http.DefaultClient, UserAgent: "t"}
		_, err := f.Get(ctx, "k", srv.URL, Limits{TTL: time.Hour}, nil)
		var pe *ProviderError
		if !errors.Is(err, want) || !errors.As(err, &pe) || pe.Provider != "X" {
			t.Errorf("HTTP %d: %v, quer %v", code, err, want)
		}
		srv.Close()
	}
	var n int
	e.conn.QueryRow("SELECT count(*) FROM metadata_cache WHERE key LIKE 'X %'").Scan(&n)
	if n != 0 {
		t.Errorf("respostas de erro foram para o cache: %d", n)
	}
}

// Um pedido por vez por provedor, com o intervalo configurado entre eles.
func TestFetcherRate(t *testing.T) {
	e := newEnv(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, "{}") }))
	defer srv.Close()
	f := &Fetcher{Provider: "X", DB: e.conn, Client: http.DefaultClient, UserAgent: "t"}
	lim := Limits{Interval: 150 * time.Millisecond, TTL: time.Hour}
	start := time.Now()
	for i := range 3 {
		if _, err := f.Get(context.Background(), fmt.Sprint(i), srv.URL, lim, nil); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 300*time.Millisecond {
		t.Errorf("3 pedidos em %v; com intervalo de 150 ms deveriam levar ≥ 300 ms", d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Get(ctx, "outro", srv.URL, lim, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelado durante a espera: %v", err)
	}
}

// Casos reais da busca de 2026-11-19 (GCD e ComicVine).
func TestScoreRealCases(t *testing.T) {
	cases := []struct {
		g        Guess
		c        Candidate
		min, max int
	}{
		// Acertos.
		{Guess{Series: "Guerras Secretas", Number: "1", Year: 1984}, Candidate{Series: "Secret Wars (Guerras Secretas)", Number: "1", Year: 1986, Language: "pt"}, 85, 100},
		{Guess{Series: "Magneto", Number: "18", Year: 2015}, Candidate{Series: "Magneto", Number: "18", Year: 2015}, 95, 100},
		// Falsos positivos que não podem chegar à confiança alta (85).
		{Guess{Series: "Os Novos Vingadores", Number: "25", Year: 2015}, Candidate{Series: "Os Vingadores", Number: "25", Year: 2015}, 0, 84},
		{Guess{Series: "Desafio Infinito", Number: "1", Year: 2015}, Candidate{Series: "Infinito", Number: "1", Year: 2013}, 0, 84},
		{Guess{Series: "Desafio Infinito", Number: "1", Year: 2015}, Candidate{Series: "Desafio Infinito", Number: "1", Year: 1995, Language: "pt"}, 0, 84},
		{Guess{Series: "Guerras Secretas", Number: "1", Year: 1984}, Candidate{Series: "Guerras Secretas: X-Men", Number: "1", Year: 2016}, 0, 84},
	}
	for _, c := range cases {
		if s := Score(c.g, c.c); s < c.min || s > c.max {
			t.Errorf("%q × %q (%d): %d, quer entre %d e %d", c.g.Series, c.c.Series, c.c.Year, s, c.min, c.max)
		}
	}
}
