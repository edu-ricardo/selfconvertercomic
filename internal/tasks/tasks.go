// Package tasks guarda o que precisa da ação do usuário (erros de conversão; na entrega 2, também
// confirmações de metadados e de organização).
package tasks

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Tipos de tarefa.
const (
	KindConversionError = "conversion_error"
	KindMetadataConfirm = "metadata_confirm" // rel_path = pasta/nome-base da edição; ref_id = id de um arquivo dela
)

// Status de uma tarefa.
const (
	Open      = "open"
	Resolved  = "resolved"
	Dismissed = "dismissed"
)

// ErrNotFound indica tarefa inexistente ou que não está aberta.
var ErrNotFound = errors.New("tarefa não encontrada ou já resolvida")

// Task é um item da tela de Tarefas.
type Task struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind"`
	LibraryID   int64   `json:"libraryId"`
	LibraryName string  `json:"libraryName"`
	RelPath     string  `json:"relPath"`
	RefID       int64   `json:"refId"`
	Title       string  `json:"title"`
	Detail      string  `json:"detail"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"createdAt"`
	ResolvedAt  *string `json:"resolvedAt"`
}

// Store acessa a tabela tasks.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Open cria (ou atualiza, se já houver uma aberta para o mesmo arquivo) uma tarefa.
func (s *Store) Open(ctx context.Context, kind string, libraryID int64, relPath string, refID int64, title, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (kind, library_id, rel_path, ref_id, title, detail) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, library_id, rel_path) WHERE status = 'open'
		DO UPDATE SET ref_id = excluded.ref_id, title = excluded.title, detail = excluded.detail,
			created_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		kind, libraryID, relPath, refID, title, detail)
	return err
}

// ResolveFile fecha a tarefa aberta do tipo para o arquivo (ex.: a conversão deu certo depois).
func (s *Store) ResolveFile(ctx context.Context, kind string, libraryID int64, relPath string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status = 'resolved', resolved_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE kind = ? AND library_id = ? AND rel_path = ? AND status = 'open'`, kind, libraryID, relPath)
	return err
}

// Close muda uma tarefa aberta para resolved ou dismissed.
func (s *Store) Close(ctx context.Context, id int64, status string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status = ?, resolved_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND status = 'open'`, status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FromConversion abre ou fecha a tarefa de um arquivo conforme o resultado da conversão:
// falha → tarefa "erro ao converter"; sucesso com aviso (PDF original não tratado) → tarefa;
// sucesso limpo → resolve a tarefa aberta, se houver.
func (s *Store) FromConversion(ctx context.Context, libraryID int64, relPath string, jobID int64, failed bool, errMsg string) error {
	name := relPath[strings.LastIndex(relPath, "/")+1:]
	switch {
	case failed:
		return s.Open(ctx, KindConversionError, libraryID, relPath, jobID, "Erro ao converter "+name, errMsg)
	case errMsg != "":
		return s.Open(ctx, KindConversionError, libraryID, relPath, jobID, "Convertido, mas o PDF original não foi tratado: "+name, errMsg)
	default:
		return s.ResolveFile(ctx, KindConversionError, libraryID, relPath)
	}
}

// Get devolve uma tarefa.
func (s *Store) Get(ctx context.Context, id int64) (Task, error) {
	t, err := scanTask(s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM tasks t JOIN libraries l ON l.id = t.library_id WHERE t.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

const columns = "t.id, t.kind, t.library_id, l.name, t.rel_path, t.ref_id, t.title, t.detail, t.status, t.created_at, t.resolved_at"

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.Kind, &t.LibraryID, &t.LibraryName, &t.RelPath, &t.RefID, &t.Title, &t.Detail,
		&t.Status, &t.CreatedAt, &t.ResolvedAt)
	return t, err
}

// List devolve tarefas no status pedido ("" = todas), mais recentes primeiro.
func (s *Store) List(ctx context.Context, status string, limit int) ([]Task, error) {
	q := "SELECT " + columns + " FROM tasks t JOIN libraries l ON l.id = t.library_id"
	var args []any
	if status != "" {
		q += " WHERE t.status = ?"
		args = append(args, status)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY t.id DESC LIMIT ?", append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// CountOpen devolve quantas tarefas estão abertas.
func (s *Store) CountOpen(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE status = 'open'").Scan(&n)
	return n, err
}
