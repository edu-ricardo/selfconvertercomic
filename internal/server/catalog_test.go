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
)

func TestCatalogAPI(t *testing.T) {
	root := t.TempDir()
	comics := filepath.Join(root, "comics")
	for _, f := range []string{"Marvel/1984/Cap 01.pdf", "Marvel/1984/Cap 02.cbz", "DC/Batman.cbr"} {
		p := filepath.Join(comics, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	conn, h := newTestServerConn(t, "dev", library.Roots{root})
	libs := library.NewStore(conn)
	lib, err := libs.Create(context.Background(), "", comics, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := libs.Scan(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

	var series catalog.Page[catalog.Series]
	json.Unmarshal(get(t, h, "/api/catalog/series").Body.Bytes(), &series)
	if series.Total != 2 {
		t.Fatalf("séries: %+v", series)
	}
	if rec := get(t, h, "/api/catalog/series?format=doc"); rec.Code != http.StatusBadRequest {
		t.Errorf("formato inválido: %d", rec.Code)
	}
	if rec := get(t, h, "/api/catalog/files?series=Marvel/1984"); rec.Code != http.StatusBadRequest {
		t.Errorf("series sem library: %d", rec.Code)
	}

	var files catalog.Page[catalog.File]
	json.Unmarshal(get(t, h, "/api/catalog/files?library="+jsonID(lib.ID)+"&series=Marvel/1984").Body.Bytes(), &files)
	if files.Total != 2 || files.Items[0].Status != catalog.StatusPending {
		t.Fatalf("arquivos da série: %+v", files)
	}
	pdfID := jsonID(files.Items[0].ID)
	cbzID := jsonID(files.Items[1].ID)

	if rec := get(t, h, "/api/files/"+pdfID); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"readers":{}`) {
		t.Errorf("detalhe: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/api/files/999"); rec.Code != http.StatusNotFound {
		t.Errorf("arquivo inexistente: %d", rec.Code)
	}
	if rec := get(t, h, "/api/files/"+pdfID+"/thumb"); rec.Code != http.StatusNotFound {
		t.Errorf("capa de PDF inválido deveria dar 404: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/files/"+pdfID+"/convert", ""); rec.Code != http.StatusAccepted {
		t.Errorf("converter: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/files/"+pdfID+"/convert", ""); rec.Code != http.StatusConflict {
		t.Errorf("converter de novo: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/files/"+cbzID+"/convert", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("converter CBZ: %d", rec.Code)
	}

	var pubs []string
	json.Unmarshal(get(t, h, "/api/catalog/publishers").Body.Bytes(), &pubs)
	if len(pubs) != 2 {
		t.Errorf("editoras: %v", pubs)
	}
}
