package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/jobs"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

func TestTasksAPI(t *testing.T) {
	root := t.TempDir()
	conn, h := newTestServerConn(t, "dev", library.Roots{root})
	ctx := context.Background()
	lib, err := library.NewStore(conn).Create(ctx, "HD", root, true)
	if err != nil {
		t.Fatal(err)
	}
	// Um job que falhou e a tarefa correspondente (como o main faz no OnFinish).
	res, err := conn.Exec("INSERT INTO jobs (library_id, rel_path, src_size, src_mtime, status, error) VALUES (?, 'a.pdf', 1, 1, 'failed', 'corrompido')", lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	jobID, _ := res.LastInsertId()
	ts := tasks.NewStore(conn)
	ts.FromConversion(ctx, lib.ID, "a.pdf", jobID, true, "corrompido")
	ts.FromConversion(ctx, lib.ID, "b.pdf", 999, true, "outro")

	var body struct {
		Tasks []tasks.Task `json:"tasks"`
		Open  int          `json:"open"`
	}
	json.Unmarshal(get(t, h, "/api/tasks").Body.Bytes(), &body)
	if body.Open != 2 || len(body.Tasks) != 2 {
		t.Fatalf("tarefas: %+v", body)
	}
	byPath := map[string]tasks.Task{}
	for _, tk := range body.Tasks {
		byPath[tk.RelPath] = tk
	}

	// Tentar de novo: job volta para a fila e a tarefa fecha.
	if rec := do(t, h, http.MethodPost, "/api/tasks/"+jsonID(byPath["a.pdf"].ID)+"/retry", ""); rec.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
	}
	var status string
	conn.QueryRow("SELECT status FROM jobs WHERE id = ?", jobID).Scan(&status)
	if status != jobs.Queued {
		t.Fatalf("job deveria voltar para a fila, está %s", status)
	}

	// Job inexistente: 409 e a tarefa continua aberta; Ignorar fecha.
	bID := jsonID(byPath["b.pdf"].ID)
	if rec := do(t, h, http.MethodPost, "/api/tasks/"+bID+"/retry", ""); rec.Code != http.StatusConflict {
		t.Errorf("retry sem job: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/tasks/"+bID+"/dismiss", ""); rec.Code != http.StatusOK {
		t.Errorf("dismiss: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/tasks/"+bID+"/dismiss", ""); rec.Code != http.StatusNotFound {
		t.Errorf("dismiss de novo: %d", rec.Code)
	}
	json.Unmarshal(get(t, h, "/api/tasks").Body.Bytes(), &body)
	if body.Open != 0 {
		t.Fatalf("abertas: %d", body.Open)
	}
	json.Unmarshal(get(t, h, "/api/tasks?status=all").Body.Bytes(), &body)
	if len(body.Tasks) != 2 {
		t.Fatalf("todas: %d", len(body.Tasks))
	}
	if rec := get(t, h, "/api/tasks?status=xyz"); rec.Code != http.StatusBadRequest {
		t.Errorf("status inválido: %d", rec.Code)
	}

	// Teste de notificação sem ntfy configurado: erro explicando o que falta.
	rec := do(t, h, http.MethodPost, "/api/notifications/test", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "configure") {
		t.Errorf("teste de notificação: %d %s", rec.Code, rec.Body.String())
	}
}
