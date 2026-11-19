// Package catalog responde às consultas da UI: séries, arquivos, filtros, busca e miniaturas.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// ErrNotFound indica arquivo inexistente.
var ErrNotFound = errors.New("arquivo não encontrado")

// Status de um arquivo no catálogo.
const (
	StatusPending    = "pending"    // PDF ainda não convertido
	StatusConverting = "converting" // PDF na fila ou convertendo
	StatusFailed     = "failed"     // última conversão do PDF falhou
	StatusConverted  = "converted"  // PDF que já tem CBZ ao lado
	StatusReady      = "ready"      // CBZ/CBR
)

var validStatus = map[string]bool{StatusPending: true, StatusConverting: true, StatusFailed: true, StatusConverted: true, StatusReady: true}

// statusExpr calcula o status de f (tabela files) com consultas por índice.
const statusExpr = `CASE
	WHEN f.format <> 'pdf' THEN 'ready'
	WHEN EXISTS (SELECT 1 FROM jobs j WHERE j.library_id = f.library_id AND j.rel_path = f.rel_path
	             AND j.status IN ('queued', 'running')) THEN 'converting'
	WHEN EXISTS (SELECT 1 FROM files c WHERE c.library_id = f.library_id AND c.format = 'cbz'
	             AND c.rel_path = substr(f.rel_path, 1, length(f.rel_path) - 4) || '.cbz') THEN 'converted'
	WHEN (SELECT j.status FROM jobs j WHERE j.library_id = f.library_id AND j.rel_path = f.rel_path
	      ORDER BY j.id DESC LIMIT 1) = 'failed' THEN 'failed'
	ELSE 'pending' END`

// seriesKey agrupa os arquivos pela série identificada da edição; sem edição (antes da primeira
// varredura depois da migração), pela pasta.
const seriesKey = "coalesce(e.series_key, 'pasta:' || f.series)"

// fromFiles junta arquivos, bibliotecas e edições.
const fromFiles = "FROM files f JOIN libraries l ON l.id = f.library_id LEFT JOIN editions e ON e.id = f.edition_id"

// editionOrder ordena pelo número da edição (desconhecidos por último) e depois pelo caminho.
const editionOrder = "coalesce(e.number, '') = '', CAST(e.number AS REAL), f.rel_path COLLATE NOCASE"

// Filter restringe séries e arquivos. Campos vazios não filtram.
type Filter struct {
	LibraryID int64
	Publisher string
	Format    string // pdf | cbz | cbr
	Status    string // ver constantes Status*
	Query     string // busca textual em título, série e editora
	Offset    int
	Limit     int
}

// Validate confere valores enumerados e ajusta a paginação.
func (f *Filter) Validate() error {
	if f.Format != "" && f.Format != "pdf" && f.Format != "cbz" && f.Format != "cbr" {
		return fmt.Errorf("formato inválido: %q", f.Format)
	}
	if f.Status != "" && !validStatus[f.Status] {
		return fmt.Errorf("status inválido: %q", f.Status)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 60
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return nil
}

// where devolve a condição SQL (sobre f, l e e) e os argumentos.
func (f Filter) where() (string, []any) {
	conds := []string{"1 = 1"}
	var args []any
	if f.LibraryID != 0 {
		conds = append(conds, "f.library_id = ?")
		args = append(args, f.LibraryID)
	}
	if f.Publisher != "" {
		conds = append(conds, "f.publisher = ?")
		args = append(args, f.Publisher)
	}
	if f.Format != "" {
		conds = append(conds, "f.format = ?")
		args = append(args, f.Format)
	}
	if f.Status != "" {
		conds = append(conds, "("+statusExpr+") = ?")
		args = append(args, f.Status)
	}
	if q := ftsQuery(f.Query); q != "" {
		conds = append(conds, `(f.id IN (SELECT rowid FROM files_fts WHERE files_fts MATCH ?)
			OR f.edition_id IN (SELECT rowid FROM editions_fts WHERE editions_fts MATCH ?))`)
		args = append(args, q, q)
	}
	return strings.Join(conds, " AND "), args
}

// ftsQuery transforma o texto digitado em termos FTS5 com prefixo ("guer sec" → "guer"* "sec"*).
func ftsQuery(s string) string {
	var terms []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		terms = append(terms, `"`+w+`"*`)
	}
	return strings.Join(terms, " ")
}

// Series é um grupo de arquivos (série identificada) no catálogo.
type Series struct {
	LibraryID   int64          `json:"libraryId"`
	LibraryName string         `json:"libraryName"`
	Key         string         `json:"key"`  // chave da série (ver seriesKey)
	Name        string         `json:"name"` // nome para exibição
	Publisher   string         `json:"publisher"`
	Count       int            `json:"count"`
	Formats     map[string]int `json:"formats"`
	CoverFileID int64          `json:"coverFileId"`
}

// Page é uma página de resultados.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Store consulta o catálogo.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ListSeries agrupa os arquivos filtrados por biblioteca + série identificada, em ordem alfabética.
// Quando a mesma série aparece com grafias diferentes ("V Force", "V-Force"), mostra a de maior
// ordem, que costuma ser a com pontuação, vinda da pasta.
func (s *Store) ListSeries(ctx context.Context, f Filter) (Page[Series], error) {
	page := Page[Series]{Items: []Series{}}
	where, args := f.where()
	base := fromFiles + " WHERE " + where
	group := " GROUP BY f.library_id, " + seriesKey

	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM (SELECT 1 "+base+group+")", args...).
		Scan(&page.Total); err != nil {
		return page, err
	}
	// A capa (1ª edição, preferindo CBZ/CBR) é buscada só para as séries da página, pelos índices de
	// editions(library_id, series_key) e files(library_id, series) — esta para arquivos ainda sem edição.
	q := `SELECT g.*, coalesce(
			(SELECT c.id FROM editions ce JOIN files c ON c.edition_id = ce.id
			 WHERE ce.library_id = g.lib AND ce.series_key = g.key
			 ORDER BY c.format = 'pdf', ce.number = '', CAST(ce.number AS REAL), c.rel_path LIMIT 1),
			(SELECT c.id FROM files c WHERE g.key LIKE 'pasta:%' AND c.library_id = g.lib AND c.series = substr(g.key, 7)
			 ORDER BY c.format = 'pdf', c.rel_path LIMIT 1), 0)
		FROM (SELECT f.library_id AS lib, l.name AS lname, ` + seriesKey + ` AS key, coalesce(max(e.series), '') AS name,
			min(f.series) AS folder, min(f.publisher), count(*), sum(f.format = 'pdf'), sum(f.format = 'cbz'),
			sum(f.format = 'cbr') ` + base + group + `
			ORDER BY name COLLATE NOCASE, folder COLLATE NOCASE, lname LIMIT ? OFFSET ?) g`
	rows, err := s.db.QueryContext(ctx, q, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var sr Series
		var folder string
		var pdf, cbz, cbr int
		if err := rows.Scan(&sr.LibraryID, &sr.LibraryName, &sr.Key, &sr.Name, &folder, &sr.Publisher, &sr.Count,
			&pdf, &cbz, &cbr, &sr.CoverFileID); err != nil {
			return page, err
		}
		sr.Formats = map[string]int{"pdf": pdf, "cbz": cbz, "cbr": cbr}
		if sr.Name == "" {
			sr.Name = seriesName(folder, sr.Publisher, sr.LibraryName)
		}
		page.Items = append(page.Items, sr)
	}
	return page, rows.Err()
}

// seriesName tira a editora do começo da pasta: "Marvel/guerras_secretas/1984" → "guerras_secretas / 1984".
// Usado só para arquivos ainda sem edição.
func seriesName(key, publisher, libraryName string) string {
	if key == "" {
		return libraryName
	}
	name := strings.TrimPrefix(strings.TrimPrefix(key, publisher), "/")
	if name == "" {
		name = key
	}
	return strings.ReplaceAll(name, "/", " / ")
}

// File é um arquivo do catálogo, com o que se sabe da edição dele.
type File struct {
	ID             int64  `json:"id"`
	LibraryID      int64  `json:"libraryId"`
	LibraryName    string `json:"libraryName"`
	RelPath        string `json:"relPath"`
	Format         string `json:"format"`
	Size           int64  `json:"size"`
	Mtime          int64  `json:"mtime"`
	Publisher      string `json:"publisher"`
	Series         string `json:"series"` // chave da série (a mesma de Series.Key)
	SeriesName     string `json:"seriesName"`
	Volume         string `json:"volume"`
	Number         string `json:"number"`
	NumberInferred bool   `json:"numberInferred"`
	Year           int    `json:"year"`
	Title          string `json:"title"`
	Status         string `json:"status"`
}

const fileColumns = `f.id, f.library_id, l.name, f.rel_path, f.format, f.size, f.mtime, f.publisher, f.series, f.title, ` +
	seriesKey + `, coalesce(e.series, ''), coalesce(e.volume, ''), coalesce(e.number, ''),
	coalesce(e.number_inferred, 0), coalesce(e.year, 0), ` + statusExpr

func scanFile(row interface{ Scan(...any) error }, extra ...any) (File, error) {
	var x File
	var folder string
	dest := []any{&x.ID, &x.LibraryID, &x.LibraryName, &x.RelPath, &x.Format, &x.Size, &x.Mtime, &x.Publisher, &folder,
		&x.Title, &x.Series, &x.SeriesName, &x.Volume, &x.Number, &x.NumberInferred, &x.Year, &x.Status}
	err := row.Scan(append(dest, extra...)...)
	if x.SeriesName == "" {
		x.SeriesName = seriesName(folder, x.Publisher, x.LibraryName)
	}
	return x, err
}

// ListFiles devolve os arquivos filtrados; series != nil restringe a uma série (exige LibraryID),
// em ordem de número da edição.
func (s *Store) ListFiles(ctx context.Context, f Filter, series *string) (Page[File], error) {
	page := Page[File]{Items: []File{}}
	where, args := f.where()
	order := "f.rel_path COLLATE NOCASE"
	if series != nil {
		where += " AND " + seriesKey + " = ?"
		args = append(args, *series)
		order = editionOrder
	}
	base := fromFiles + " WHERE " + where
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) "+base, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+fileColumns+" "+base+" ORDER BY "+order+" LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		x, err := scanFile(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, x)
	}
	return page, rows.Err()
}

// Get devolve um arquivo e o caminho absoluto dele no disco.
func (s *Store) Get(ctx context.Context, id int64) (File, string, error) {
	var root string
	x, err := scanFile(s.db.QueryRowContext(ctx, "SELECT "+fileColumns+", l.path "+fromFiles+" WHERE f.id = ?", id), &root)
	if errors.Is(err, sql.ErrNoRows) {
		return x, "", ErrNotFound
	}
	if err != nil {
		return x, "", err
	}
	return x, filepath.Join(root, filepath.FromSlash(x.RelPath)), nil
}

// Publishers lista as editoras (para o filtro), opcionalmente de uma biblioteca.
func (s *Store) Publishers(ctx context.Context, libraryID int64) ([]string, error) {
	q := "SELECT DISTINCT publisher FROM files WHERE publisher <> ''"
	var args []any
	if libraryID != 0 {
		q += " AND library_id = ?"
		args = append(args, libraryID)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY publisher COLLATE NOCASE", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
