package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/catalog"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

// Os provedores do servidor de teste apontam para um endereço sem ninguém: simula serviço fora do ar.
func TestMetadataAPI(t *testing.T) {
	root := t.TempDir()
	comics := filepath.Join(root, "comics")
	p := filepath.Join(comics, "Marvel", "guerras_secretas", "1984", "Capítulo 01.pdf")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
	conn, h := newTestServerConn(t, "dev", library.Roots{root})
	libs := library.NewStore(conn)
	lib, _ := libs.Create(context.Background(), "", comics, false)
	libs.Scan(context.Background(), lib)
	var files catalog.Page[catalog.File]
	json.Unmarshal(get(t, h, "/api/catalog/files").Body.Bytes(), &files)
	id := jsonID(files.Items[0].ID)

	// Padrão: GCD ligado, ComicVine sem chave → busca só no GCD, que está fora: 200 com o erro explicado.
	rec := get(t, h, "/api/files/"+id+"/candidates")
	var res struct {
		Query struct {
			Series, Number string
			Year           int
		}
		Candidates []any
		Errors     []string
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || res.Query.Series != "Guerras Secretas" || res.Query.Number != "1" || res.Query.Year != 1984 ||
		len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "GCD: serviço fora do ar") {
		t.Errorf("candidatos com GCD fora: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/api/files/999/candidates"); rec.Code != http.StatusNotFound {
		t.Errorf("arquivo inexistente: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/metadata/comicvine/test", ""); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "nenhuma chave") {
		t.Errorf("testar sem chave: %d %s", rec.Code, rec.Body.String())
	}

	// Identificar: a edição entra na fila e o detalhe do arquivo mostra o estado.
	if rec := do(t, h, http.MethodPost, "/api/files/"+id+"/identify", ""); rec.Code != http.StatusAccepted {
		t.Errorf("identificar arquivo: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/api/files/"+id); !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Errorf("detalhe sem o estado da edição: %s", rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/libraries/"+jsonID(lib.ID)+"/identify", ""); rec.Code != http.StatusAccepted ||
		!strings.Contains(rec.Body.String(), `"queued":0`) { // já está na fila
		t.Errorf("identificar biblioteca: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/libraries/999/identify", ""); rec.Code != http.StatusNotFound {
		t.Errorf("biblioteca inexistente: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/catalog/series/identify?library="+jsonID(lib.ID)+"&series=guerrassecretas", ""); rec.Code != http.StatusAccepted {
		t.Errorf("identificar série: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/catalog/series/identify", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("série sem parâmetros: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/files/"+id+"/metadata", `{"provider":"outro","issueId":"1"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("aplicar com fonte inválida: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/files/"+id+"/metadata", `{"provider":"gcd","issueId":"1"}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("aplicar com o GCD fora: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/api/files/"+id+"/candidates?series="); rec.Code != http.StatusBadRequest {
		t.Errorf("busca manual sem série: %d", rec.Code)
	}

	// Link: não reconhecido → 422; reconhecido, com a fonte fora → 503.
	if rec := do(t, h, http.MethodPost, "/api/files/"+id+"/metadata", `{"url":"https://example.com/x"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("link desconhecido: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/files/"+id+"/metadata", `{"url":"https://www.comics.org/series/41527/","number":"1"}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("link de série com a fonte fora: %d %s", rec.Code, rec.Body.String())
	}
	// Manual.
	if rec := do(t, h, http.MethodPut, "/api/files/"+id+"/metadata", `{"number":"1"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("manual sem série: %d", rec.Code)
	}
	rec = do(t, h, http.MethodPut, "/api/files/"+id+"/metadata", `{"series":"Guerras Secretas","number":"1","year":1984,"writers":["Jim Shooter"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"provider":"manual"`) || !strings.Contains(rec.Body.String(), `"status":"applied"`) {
		t.Errorf("manual: %d %s", rec.Code, rec.Body.String())
	}

	s := settings.Defaults()
	s.Metadata.GCDEnabled = false
	settings.NewStore(conn).Put(context.Background(), s)
	if rec := get(t, h, "/api/files/"+id+"/candidates"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("sem provedores: %d %s", rec.Code, rec.Body.String())
	}
	s.Metadata.ComicVineAPIKey = "qualquer"
	settings.NewStore(conn).Put(context.Background(), s)
	if rec := do(t, h, http.MethodPost, "/api/metadata/comicvine/test", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("testar com ComicVine fora: %d %s", rec.Code, rec.Body.String())
	}
}
