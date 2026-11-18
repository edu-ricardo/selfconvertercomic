package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testUI() fstest.MapFS {
	return fstest.MapFS{
		"index.html":   {Data: []byte("<html>app</html>")},
		"_app/main.js": {Data: []byte("console.log(1)")},
	}
}

func get(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestHealth(t *testing.T) {
	rec := get(t, New("1.2.3", testUI()), "/api/health")
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
	rec := get(t, New("dev", testUI()), "/api/nao-existe")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("status = %d, content-type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestSPA(t *testing.T) {
	h := New("dev", testUI())
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
