package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/backup"
)

func TestBackupsAPI(t *testing.T) {
	h := newTestServer(t, "dev")
	do(t, h, http.MethodPut, "/api/settings", `{"conversion":{"workers":4}}`)

	rec := do(t, h, http.MethodPost, "/api/backups", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar: %d %s", rec.Code, rec.Body.String())
	}
	var info backup.Info
	json.Unmarshal(rec.Body.Bytes(), &info)

	var list []backup.Info
	json.Unmarshal(get(t, h, "/api/backups").Body.Bytes(), &list)
	if len(list) != 1 || list[0].Name != info.Name {
		t.Fatalf("lista: %+v", list)
	}

	dl := get(t, h, "/api/backups/"+info.Name)
	if dl.Code != http.StatusOK || dl.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(dl.Header().Get("Content-Disposition"), info.Name) {
		t.Fatalf("download: %d %v", dl.Code, dl.Header())
	}
	zipBytes, _ := io.ReadAll(dl.Body)
	if rec := get(t, h, "/api/backups/..%2Fqt.db"); rec.Code != http.StatusNotFound {
		t.Errorf("nome inválido: %d", rec.Code)
	}

	// Muda a configuração e restaura o backup baixado.
	do(t, h, http.MethodPut, "/api/settings", `{"conversion":{"workers":1}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/backups/restore", bytes.NewReader(zipBytes))
	req.Header.Set("Content-Type", "application/zip")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "safetyBackup") {
		t.Fatalf("restaurar: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(get(t, h, "/api/settings").Body.String(), `"workers":4`) {
		t.Fatal("a configuração do backup deveria ter voltado")
	}

	if rec := do(t, h, http.MethodPost, "/api/backups/restore", "lixo"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("arquivo inválido: %d", rec.Code)
	}
}
