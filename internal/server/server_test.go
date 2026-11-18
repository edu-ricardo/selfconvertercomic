package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/catalog"
	"github.com/edu-ricardo/selfconvertercomic/internal/convert"
	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

func testUI() fstest.MapFS {
	return fstest.MapFS{
		"index.html":   {Data: []byte("<html>app</html>")},
		"_app/main.js": {Data: []byte("console.log(1)")},
	}
}

func newTestServer(t *testing.T, version string) http.Handler {
	t.Helper()
	return newTestServerWithRoots(t, version, nil)
}

func newTestServerWithRoots(t *testing.T, version string, roots library.Roots) http.Handler {
	t.Helper()
	_, h := newTestServerConn(t, version, roots)
	return h
}

// newTestServerConn também devolve o banco, para os testes prepararem dados.
func newTestServerConn(t *testing.T, version string, roots library.Roots) (*sql.DB, http.Handler) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	libs := library.NewStore(conn)
	st := settings.NewStore(conn)
	js := jobs.NewStore(conn)
	return conn, New(Deps{
		Version:   version,
		UI:        testUI(),
		Settings:  st,
		Libraries: libs,
		Roots:     roots,
		Scanner:   library.NewManager(libs, func() time.Duration { return time.Hour }),
		Jobs:      js,
		Catalog:   catalog.NewStore(conn),
		Thumbs:    &catalog.Thumbnailer{Dir: t.TempDir()},
		Runner: jobs.NewRunner(js, st, convert.Converter{}, func(ctx context.Context, id int64) (string, string, error) {
			l, err := libs.Get(ctx, id)
			return l.Path, l.Name, err
		}),
	})
}

func do(t *testing.T, h http.Handler, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
	return rec
}

func get(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestHealth(t *testing.T) {
	rec := get(t, newTestServer(t, "1.2.3"), "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quer 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "1.2.3" {
		t.Fatalf("corpo inesperado: %v", body)
	}
}

func TestUnknownAPIRouteIs404JSON(t *testing.T) {
	rec := get(t, newTestServer(t, "dev"), "/api/nao-existe")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("status = %d, content-type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestSPA(t *testing.T) {
	h := newTestServer(t, "dev")
	cases := map[string]string{
		"/":             "<html>app</html>",
		"/catalogo/123": "<html>app</html>", // rota do frontend cai no index
		"/_app/main.js": "console.log(1)",
	}
	for url, want := range cases {
		rec := get(t, h, url)
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("%s: status %d, corpo %q; quer %q", url, rec.Code, rec.Body.String(), want)
		}
	}
}

func TestSettingsGetReturnsDefaults(t *testing.T) {
	rec := get(t, newTestServer(t, "dev"), "/api/settings")
	var got settings.Settings
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got != settings.Defaults() {
		t.Fatalf("status %d, corpo %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsPutPartialUpdate(t *testing.T) {
	h := newTestServer(t, "dev")
	rec := do(t, h, http.MethodPut, "/api/settings", `{"conversion":{"workers":3},"readers":{"kavitaUrl":"http://umbrel.local:8798"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status %d: %s", rec.Code, rec.Body.String())
	}
	var got settings.Settings
	json.Unmarshal(get(t, h, "/api/settings").Body.Bytes(), &got)
	want := settings.Defaults()
	want.Conversion.Workers = 3
	want.Readers.KavitaURL = "http://umbrel.local:8798"
	if got != want {
		t.Fatalf("got %+v\nquer %+v", got, want)
	}
}

func TestSettingsPutValidation(t *testing.T) {
	h := newTestServer(t, "dev")
	rec := do(t, h, http.MethodPut, "/api/settings", `{"conversion":{"workers":0}}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "conversion.workers") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPut, "/api/settings", `{"naoExiste":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("campo desconhecido: status %d", rec.Code)
	}
}
