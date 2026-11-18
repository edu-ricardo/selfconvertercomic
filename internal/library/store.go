// Package library cadastra bibliotecas, varre as pastas e mantém o catálogo de arquivos.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrNotFound indica biblioteca inexistente.
var ErrNotFound = errors.New("biblioteca não encontrada")

// ErrNameRequired indica nome vazio.
var ErrNameRequired = errors.New("nome obrigatório")

// ErrOverlap indica pasta igual, dentro ou acima de outra biblioteca.
var ErrOverlap = errors.New("a pasta se sobrepõe a outra biblioteca")

// Library é uma pasta raiz cadastrada.
type Library struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	AutoConvert bool           `json:"autoConvert"`
	Available   bool           `json:"available"`
	LastScanAt  *string        `json:"lastScanAt"`
	LastError   string         `json:"lastError"`
	Counts      map[string]int `json:"counts"` // por formato: pdf, cbz, cbr
}

// Store acessa bibliotecas e arquivos no banco.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const libColumns = "id, name, path, auto_convert, available, last_scan_at, last_error"

func scanLibrary(row interface{ Scan(...any) error }) (Library, error) {
	var l Library
	err := row.Scan(&l.ID, &l.Name, &l.Path, &l.AutoConvert, &l.Available, &l.LastScanAt, &l.LastError)
	return l, err
}

// List devolve todas as bibliotecas com a contagem de arquivos por formato.
func (s *Store) List(ctx context.Context) ([]Library, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+libColumns+" FROM libraries ORDER BY name COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	libs := []Library{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		libs = append(libs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range libs {
		if libs[i].Counts, err = s.counts(ctx, libs[i].ID); err != nil {
			return nil, err
		}
	}
	return libs, nil
}

// Get devolve uma biblioteca.
func (s *Store) Get(ctx context.Context, id int64) (Library, error) {
	l, err := scanLibrary(s.db.QueryRowContext(ctx, "SELECT "+libColumns+" FROM libraries WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	l.Counts, err = s.counts(ctx, id)
	return l, err
}

func (s *Store) counts(ctx context.Context, id int64) (map[string]int, error) {
	c := map[string]int{"pdf": 0, "cbz": 0, "cbr": 0}
	rows, err := s.db.QueryContext(ctx, "SELECT format, count(*) FROM files WHERE library_id = ? GROUP BY format", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		var n int
		if err := rows.Scan(&f, &n); err != nil {
			return nil, err
		}
		c[f] = n
	}
	return c, rows.Err()
}

// Create cadastra uma biblioteca; path já deve ter passado por Roots.Resolve.
func (s *Store) Create(ctx context.Context, name, path string, autoConvert bool) (Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(path)
	}
	libs, err := s.List(ctx)
	if err != nil {
		return Library{}, err
	}
	for _, l := range libs {
		if overlaps(l.Path, path) {
			return Library{}, fmt.Errorf("%w (%s)", ErrOverlap, l.Name)
		}
	}
	res, err := s.db.ExecContext(ctx, "INSERT INTO libraries (name, path, auto_convert) VALUES (?, ?, ?)", name, path, autoConvert)
	if err != nil {
		return Library{}, err
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, id)
}

func overlaps(a, b string) bool {
	sep := string(filepath.Separator)
	a, b = filepath.Clean(a), filepath.Clean(b)
	return a == b || strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}

// Update altera nome e auto-conversão (a pasta não muda; para isso, remova e cadastre de novo).
func (s *Store) Update(ctx context.Context, id int64, name string, autoConvert bool) (Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Library{}, ErrNameRequired
	}
	res, err := s.db.ExecContext(ctx, "UPDATE libraries SET name = ?, auto_convert = ? WHERE id = ?", name, autoConvert, id)
	if err != nil {
		return Library{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Library{}, ErrNotFound
	}
	return s.Get(ctx, id)
}

// Delete remove a biblioteca e seu catálogo (os arquivos no disco não são tocados).
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM libraries WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) setStatus(ctx context.Context, id int64, available bool, lastErr string, scanned bool) error {
	q := "UPDATE libraries SET available = ?, last_error = ? WHERE id = ?"
	if scanned {
		q = "UPDATE libraries SET available = ?, last_error = ?, last_scan_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?"
	}
	_, err := s.db.ExecContext(ctx, q, available, lastErr, id)
	return err
}
