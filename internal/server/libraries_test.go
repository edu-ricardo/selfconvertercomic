package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

func TestLibrariesCRUD(t *testing.T) {
	root := t.TempDir()
	comics := filepath.Join(root, "comics")
	os.MkdirAll(filepath.Join(comics, "Marvel"), 0o755)
	h := newTestServerWithRoots(t, "dev", library.Roots{root})

	body, _ := json.Marshal(map[string]any{"path": comics})
	rec := do(t, h, http.MethodPost, "/api/libraries", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	var created libraryView
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Name != "comics" || !created.AutoConvert || created.ID == 0 {
		t.Fatalf("criada: %+v", created)
	}

	// Sobreposição e caminho fora das raízes.
	body, _ = json.Marshal(map[string]any{"path": filepath.Join(comics, "Marvel")})
	if rec := do(t, h, http.MethodPost, "/api/libraries", string(body)); rec.Code != http.StatusConflict {
		t.Errorf("sobreposição: %d", rec.Code)
	}
	body, _ = json.Marshal(map[string]any{"path": filepath.Dir(root)})
	if rec := do(t, h, http.MethodPost, "/api/libraries", string(body)); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("fora das raízes: %d", rec.Code)
	}

	id := jsonID(created.ID)
	if rec := do(t, h, http.MethodPut, "/api/libraries/"+id, `{"name":"Quadrinhos","autoConvert":false}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"autoConvert":false`) {
		t.Errorf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPut, "/api/libraries/"+id, `{"name":" "}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("PUT sem nome: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/libraries/"+id+"/scan", ""); rec.Code != http.StatusAccepted {
		t.Errorf("scan: %d", rec.Code)
	}

	rec = get(t, h, "/api/libraries")
	if !strings.Contains(rec.Body.String(), `"name":"Quadrinhos"`) {
		t.Errorf("GET: %s", rec.Body.String())
	}

	if rec := do(t, h, http.MethodDelete, "/api/libraries/"+id, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/libraries/"+id, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE de novo: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(comics, "Marvel")); err != nil {
		t.Error("remover a biblioteca não pode apagar pastas do disco")
	}
}

func TestBrowse(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "HD", "comics"), 0o755)
	h := newTestServerWithRoots(t, "dev", library.Roots{root})

	var l library.Listing
	json.Unmarshal(get(t, h, "/api/fs?path="+filepath.Join(root, "HD")).Body.Bytes(), &l)
	if len(l.Dirs) != 1 || l.Dirs[0].Name != "comics" || l.Parent != root {
		t.Fatalf("listagem: %+v", l)
	}
	if rec := get(t, h, "/api/fs?path="+filepath.Dir(root)); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("fora das raízes: %d", rec.Code)
	}
}

func jsonID(id int64) string { b, _ := json.Marshal(id); return string(b) }

func TestJobsAPI(t *testing.T) {
	root := t.TempDir()
	h := newTestServerWithRoots(t, "dev", library.Roots{root})

	rec := get(t, h, "/api/jobs")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"turbo":{"enabled":false`) {
		t.Fatalf("GET /api/jobs: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/api/jobs?status=xyz"); rec.Code != http.StatusBadRequest {
		t.Errorf("filtro inválido: %d", rec.Code)
	}
	rec = do(t, h, http.MethodPut, "/api/turbo", `{"enabled":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("turbo: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/jobs/42/retry", ""); rec.Code != http.StatusNotFound {
		t.Errorf("retry inexistente: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/jobs/42/cancel", ""); rec.Code != http.StatusNotFound {
		t.Errorf("cancel inexistente: %d", rec.Code)
	}
}

func TestArchiveDirMustBeInsideRoots(t *testing.T) {
	root := t.TempDir()
	h := newTestServerWithRoots(t, "dev", library.Roots{root})
	outside, _ := json.Marshal(map[string]any{"conversion": map[string]any{"originalAction": "archive", "archiveDir": filepath.Dir(root)}})
	if rec := do(t, h, http.MethodPut, "/api/settings", string(outside)); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("pasta fora das raízes: %d %s", rec.Code, rec.Body.String())
	}
	inside, _ := json.Marshal(map[string]any{"conversion": map[string]any{"originalAction": "archive", "archiveDir": filepath.Join(root, "pdfs")}})
	if rec := do(t, h, http.MethodPut, "/api/settings", string(inside)); rec.Code != http.StatusOK {
		t.Errorf("pasta dentro das raízes: %d %s", rec.Code, rec.Body.String())
	}
}
