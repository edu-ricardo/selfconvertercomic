package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/organize"
)

func TestOrganizeAPI(t *testing.T) {
	root := t.TempDir()
	comics := filepath.Join(root, "comics")
	p := filepath.Join(comics, "Marvel", "guerras_secretas", "1984", "Capítulo 01.pdf")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
	conn, h := newTestServerConn(t, "dev", library.Roots{root})
	libs := library.NewStore(conn)
	lib, _ := libs.Create(context.Background(), "", comics, false)
	libs.Scan(context.Background(), lib)
	base := "/api/libraries/" + jsonID(lib.ID) + "/organize"

	// Sem template: usa o das configurações.
	rec := get(t, h, base)
	var plan organize.Plan
	json.Unmarshal(rec.Body.Bytes(), &plan)
	if rec.Code != http.StatusOK || len(plan.Items) != 1 || plan.Items[0].To != "Marvel/Guerras Secretas (1984)/Guerras Secretas (1984) #1" {
		t.Fatalf("plano: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, base+"?template="+url.QueryEscape("{autor}/{serie}")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("template inválido: %d", rec.Code)
	}
	if rec := get(t, h, "/api/libraries/999/organize"); rec.Code != http.StatusNotFound {
		t.Errorf("biblioteca inexistente: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, base, `{"template":"{serie}","editions":[]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("aplicar sem edições: %d", rec.Code)
	}
	body := `{"template":"{serie} #{numero}","editions":[` + jsonID(plan.Items[0].EditionID) + `]}`
	rec = do(t, h, http.MethodPost, base, body)
	var op organize.Op
	json.Unmarshal(rec.Body.Bytes(), &op)
	if rec.Code != http.StatusOK || op.Status != "done" || op.Moves != 1 {
		t.Fatalf("aplicar: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(comics, "Guerras Secretas #1.pdf")); err != nil {
		t.Errorf("arquivo não foi movido: %v", err)
	}
	if rec := get(t, h, base+"/history"); !strings.Contains(rec.Body.String(), `"status":"done"`) {
		t.Errorf("histórico: %s", rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/organize/"+jsonID(op.ID)+"/undo", ""); rec.Code != http.StatusOK {
		t.Errorf("desfazer: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/organize/"+jsonID(op.ID)+"/undo", ""); rec.Code != http.StatusConflict {
		t.Errorf("desfazer de novo: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/organize/999/undo", ""); rec.Code != http.StatusNotFound {
		t.Errorf("operação inexistente: %d", rec.Code)
	}
}
