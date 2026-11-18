// Package jobs mantém a fila persistente de conversões e os workers que a executam.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Status de um job.
const (
	Queued    = "queued"
	Running   = "running"
	Done      = "done"
	Failed    = "failed"
	Cancelled = "cancelled"
)

// ErrNotFound indica job inexistente.
var ErrNotFound = errors.New("job não encontrado")

// Job é uma conversão na fila (ou já executada).
type Job struct {
	ID            int64   `json:"id"`
	LibraryID     int64   `json:"libraryId"`
	LibraryName   string  `json:"libraryName"`
	RelPath       string  `json:"relPath"`
	SrcSize       int64   `json:"srcSize"`
	SrcMtime      int64   `json:"-"`
	Auto          bool    `json:"auto"`
	Status        string  `json:"status"`
	Progress      int     `json:"progress"`
	Pages         int     `json:"pages"`
	Extracted     int     `json:"extracted"`
	Rendered      int     `json:"rendered"`
	OutputRelPath string  `json:"outputRelPath"`
	Error         string  `json:"error"`
	Attempts      int     `json:"attempts"`
	CreatedAt     string  `json:"createdAt"`
	StartedAt     *string `json:"startedAt"`
	FinishedAt    *string `json:"finishedAt"`
}

const jobColumns = `j.id, j.library_id, l.name, j.rel_path, j.src_size, j.src_mtime, j.auto, j.status, j.progress,
	j.pages, j.extracted, j.rendered, j.output_rel_path, j.error, j.attempts, j.created_at, j.started_at, j.finished_at`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.LibraryID, &j.LibraryName, &j.RelPath, &j.SrcSize, &j.SrcMtime, &j.Auto, &j.Status,
		&j.Progress, &j.Pages, &j.Extracted, &j.Rendered, &j.OutputRelPath, &j.Error, &j.Attempts,
		&j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	return j, err
}

// Store acessa a tabela jobs.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Get devolve um job.
func (s *Store) Get(ctx context.Context, id int64) (Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx,
		"SELECT "+jobColumns+" FROM jobs j JOIN libraries l ON l.id = j.library_id WHERE j.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// List devolve jobs nos status pedidos (vazio = todos), mais recentes primeiro na fila de
// concluídos e na ordem de execução para os ativos.
func (s *Store) List(ctx context.Context, statuses []string, limit int) ([]Job, error) {
	q := "SELECT " + jobColumns + " FROM jobs j JOIN libraries l ON l.id = j.library_id"
	args := []any{}
	if len(statuses) > 0 {
		q += " WHERE j.status IN (?" + strings.Repeat(",?", len(statuses)-1) + ")"
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += " ORDER BY CASE j.status WHEN 'running' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END," +
		" CASE WHEN j.status IN ('running','queued') THEN j.id ELSE -j.id END LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, rows.Err()
}

// Counts devolve a quantidade de jobs por status.
func (s *Store) Counts(ctx context.Context) (map[string]int, error) {
	c := map[string]int{Queued: 0, Running: 0, Done: 0, Failed: 0, Cancelled: 0}
	rows, err := s.db.QueryContext(ctx, "SELECT status, count(*) FROM jobs GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		c[st] = n
	}
	return c, rows.Err()
}

// Enqueue cria um job para o arquivo, a menos que já haja um ativo para ele.
// Devolve true se criou.
func (s *Store) Enqueue(ctx context.Context, libraryID int64, relPath string, size, mtime int64, auto bool) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs (library_id, rel_path, src_size, src_mtime, auto) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, libraryID, relPath, size, mtime, auto)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// claim marca o próximo job da fila como em execução.
func (s *Store) claim(ctx context.Context) (Job, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		UPDATE jobs SET status = 'running', progress = 0, error = '', attempts = attempts + 1,
			started_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), finished_at = NULL
		WHERE id = (SELECT id FROM jobs WHERE status = 'queued' ORDER BY id LIMIT 1)
		RETURNING id`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	j, err := s.Get(ctx, id)
	return j, err == nil, err
}

func (s *Store) setProgress(ctx context.Context, id int64, progress int) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET progress = ? WHERE id = ? AND status = 'running'", progress, id)
	return err
}

// finish grava o resultado final de um job.
func (s *Store) finish(ctx context.Context, j Job) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, progress = ?, pages = ?, extracted = ?, rendered = ?, output_rel_path = ?,
			error = ?, finished_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`, j.Status, j.Progress, j.Pages, j.Extracted, j.Rendered, j.OutputRelPath, j.Error, j.ID)
	return err
}

// requeue devolve à fila um job interrompido (desligamento da app).
func (s *Store) requeue(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'queued', progress = 0, started_at = NULL WHERE id = ?", id)
	return err
}

// resetRunning devolve à fila os jobs que estavam em execução quando a app caiu.
func (s *Store) resetRunning(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'queued', progress = 0, started_at = NULL WHERE status = 'running'")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// activeCount devolve quantos jobs estão na fila ou em execução.
func (s *Store) activeCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE status IN ('queued', 'running')").Scan(&n)
	return n, err
}

// Retry recoloca na fila um job que falhou ou foi cancelado.
func (s *Store) Retry(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'queued', progress = 0, error = '', started_at = NULL, finished_at = NULL
		WHERE id = ? AND status IN ('failed', 'cancelled')`, id)
	if err != nil {
		return err // inclui violar jobs_active_file: já existe outro job ativo para o arquivo
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// cancelQueued cancela um job que ainda não começou.
func (s *Store) cancelQueued(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'cancelled', finished_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND status = 'queued'`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Candidate é um PDF do catálogo que pode ser convertido.
type Candidate struct {
	LibraryID   int64
	LibraryPath string
	RelPath     string
	Size, Mtime int64
}

// candidates lista PDFs de bibliotecas disponíveis sem job para esta versão do arquivo.
// auto=true restringe a bibliotecas com auto-conversão e a arquivos estáveis (mtime <= stableBefore).
func (s *Store) candidates(ctx context.Context, libraryID int64, auto bool, stableBefore int64) ([]Candidate, error) {
	q := `SELECT f.library_id, l.path, f.rel_path, f.size, f.mtime
		FROM files f JOIN libraries l ON l.id = f.library_id
		WHERE f.format = 'pdf' AND l.available = 1
		  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.library_id = f.library_id AND j.rel_path = f.rel_path
		                  AND (j.status IN ('queued', 'running')
		                       OR (j.src_size = f.size AND j.src_mtime = f.mtime AND j.status <> 'cancelled')))`
	args := []any{}
	if libraryID != 0 {
		q += " AND f.library_id = ?"
		args = append(args, libraryID)
	}
	if auto {
		q += " AND l.auto_convert = 1 AND f.mtime <= ?"
		args = append(args, stableBefore)
	}
	q += " ORDER BY f.library_id, f.rel_path"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.LibraryID, &c.LibraryPath, &c.RelPath, &c.Size, &c.Mtime); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}
