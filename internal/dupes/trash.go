package dupes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/fsutil"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

// TrashDir é a pasta da lixeira dentro de cada biblioteca (oculta: a varredura ignora).
const TrashDir = ".quadrinhoteca-lixeira"

// Erros da lixeira.
var (
	ErrLastCopy   = errors.New("excluir deixaria um quadrinho sem nenhuma cópia")
	ErrOccupied   = errors.New("já existe outro arquivo no caminho original")
	ErrNotInTrash = errors.New("item não está na lixeira")
)

// Item é um arquivo na lixeira.
type Item struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	RelPath     string `json:"relPath"`
	Size        int64  `json:"size"`
	Format      string `json:"format"`
	Reason      string `json:"reason"`
	DeletedAt   string `json:"deletedAt"`
}

// Trash move arquivos para a lixeira e de volta, sempre sem varredura ao mesmo tempo.
type Trash struct {
	DB        *sql.DB
	Libraries *library.Store
	Exclusive func(libraryID int64, fn func() error) error
	Now       func() time.Time
}

type candidate struct {
	id, libID, editionID int64
	rel, format, hash    string
	size                 int64
	ident                string // mesma identidade de edição usada em Find
}

// identSQL é a identidade de edição de Find (mesma edição em lugares diferentes).
const identSQL = `CASE WHEN e.meta_status IN ('applied', 'rewrite') AND json_extract(e.meta_json, '$.issueId') <> ''
		THEN json_extract(e.meta_json, '$.provider') || ':' || json_extract(e.meta_json, '$.issueId')
		WHEN e.number <> '' AND e.series_key <> '' THEN e.series_key || '#' || e.number || '/' || e.year
		ELSE '' END`

// MoveToTrash manda os arquivos para a lixeira. Cada um precisa continuar representado por algo que
// fica: outra cópia idêntica, o CBZ da mesma edição (para um PDF) ou outra versão da mesma edição.
// Se algum não puder, nada é movido.
func (t *Trash) MoveToTrash(ctx context.Context, fileIDs []int64, reason string) (int, error) {
	all, err := t.load(ctx)
	if err != nil {
		return 0, err
	}
	selected := map[int64]bool{}
	for _, id := range fileIDs {
		selected[id] = true
	}
	var chosen []candidate
	for _, c := range all {
		if !selected[c.id] {
			continue
		}
		if !representedBy(c, all, selected) {
			return 0, fmt.Errorf("%w: %s", ErrLastCopy, c.rel)
		}
		chosen = append(chosen, c)
	}
	if len(chosen) != len(selected) {
		return 0, fmt.Errorf("arquivo não encontrado no catálogo")
	}
	byLib := map[int64][]candidate{}
	for _, c := range chosen {
		byLib[c.libID] = append(byLib[c.libID], c)
	}
	moved := 0
	stamp := t.now().Format("20060102-150405")
	for libID, list := range byLib {
		lib, err := t.Libraries.Get(ctx, libID)
		if err != nil {
			return moved, err
		}
		err = t.Exclusive(libID, func() error {
			for _, c := range list {
				trashRel := path.Join(TrashDir, stamp, c.rel)
				if err := fsutil.Move(abs(lib.Path, c.rel), abs(lib.Path, trashRel)); err != nil {
					return fmt.Errorf("%s: %w", c.rel, err)
				}
				if _, err := t.DB.ExecContext(ctx, `INSERT INTO trash (library_id, rel_path, trash_rel, size, format, reason)
					VALUES (?, ?, ?, ?, ?, ?)`, libID, c.rel, trashRel, c.size, c.format, reason); err != nil {
					return err
				}
				fsutil.RemoveEmptyDirs(lib.Path, filepath.Dir(abs(lib.Path, c.rel)))
				moved++
			}
			return nil
		})
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// representedBy diz se c continua representado por algum arquivo que não vai para a lixeira.
func representedBy(c candidate, all []candidate, selected map[int64]bool) bool {
	for _, o := range all {
		if o.id == c.id || selected[o.id] {
			continue
		}
		switch {
		case c.hash != "" && c.hash != "erro" && o.hash == c.hash: // cópia idêntica
			return true
		case c.format == "pdf" && o.format == "cbz" && o.editionID != 0 && o.editionID == c.editionID: // PDF já convertido
			return true
		case c.ident != "" && o.ident == c.ident && o.editionID != c.editionID: // outra versão da mesma edição
			return true
		}
	}
	return false
}

func (t *Trash) load(ctx context.Context) ([]candidate, error) {
	rows, err := t.DB.QueryContext(ctx, `SELECT f.id, f.library_id, coalesce(f.edition_id, 0), f.rel_path, f.format,
			f.content_hash, f.size, coalesce(`+identSQL+`, '')
		FROM files f LEFT JOIN editions e ON e.id = f.edition_id
		-- O que já foi para a lixeira some do catálogo só na varredura seguinte: não conta como cópia.
		WHERE NOT EXISTS (SELECT 1 FROM trash t WHERE t.library_id = f.library_id AND t.rel_path = f.rel_path)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.libID, &c.editionID, &c.rel, &c.format, &c.hash, &c.size, &c.ident); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// ConvertedPDFs lista os PDFs que têm o CBZ da mesma edição ao lado (opção "PDFs que têm CBZ").
func (t *Trash) ConvertedPDFs(ctx context.Context) ([]File, error) {
	rows, err := t.DB.QueryContext(ctx, `SELECT `+fileCols+` FROM files f JOIN libraries l ON l.id = f.library_id
		WHERE f.format = 'pdf' AND f.edition_id IS NOT NULL AND EXISTS (SELECT 1 FROM files c
			WHERE c.edition_id = f.edition_id AND c.format = 'cbz')
		ORDER BY f.rel_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []File{}
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.LibraryID, &f.LibraryName, &f.RelPath, &f.Format, &f.Size, &f.EditionID); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// List devolve o que está na lixeira, mais recente primeiro.
func (t *Trash) List(ctx context.Context) ([]Item, error) {
	rows, err := t.DB.QueryContext(ctx, `SELECT t.id, t.library_id, l.name, t.rel_path, t.size, t.format, t.reason, t.deleted_at
		FROM trash t JOIN libraries l ON l.id = t.library_id ORDER BY t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.LibraryID, &it.LibraryName, &it.RelPath, &it.Size, &it.Format, &it.Reason, &it.DeletedAt); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

type trashRow struct {
	id, libID     int64
	rel, trashRel string
}

func (t *Trash) rows(ctx context.Context, ids []int64) ([]trashRow, error) {
	q := "SELECT id, library_id, rel_path, trash_rel FROM trash"
	var args []any
	if ids != nil {
		if len(ids) == 0 {
			return nil, nil
		}
		q += " WHERE id IN (?" + strings.Repeat(",?", len(ids)-1) + ")"
		for _, id := range ids {
			args = append(args, id)
		}
	}
	rows, err := t.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []trashRow
	for rows.Next() {
		var r trashRow
		if err := rows.Scan(&r.id, &r.libID, &r.rel, &r.trashRel); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	if ids != nil && len(list) != len(ids) {
		return nil, ErrNotInTrash
	}
	return list, rows.Err()
}

// Restore devolve itens da lixeira ao caminho original (que precisa estar livre).
func (t *Trash) Restore(ctx context.Context, ids []int64) (int, error) {
	list, err := t.rows(ctx, ids)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range list {
		lib, err := t.Libraries.Get(ctx, r.libID)
		if err != nil {
			return n, err
		}
		err = t.Exclusive(r.libID, func() error {
			if _, err := os.Lstat(abs(lib.Path, r.rel)); err == nil {
				return fmt.Errorf("%w: %s", ErrOccupied, r.rel)
			}
			if err := fsutil.Move(abs(lib.Path, r.trashRel), abs(lib.Path, r.rel)); err != nil {
				return err
			}
			fsutil.RemoveEmptyDirs(lib.Path, filepath.Dir(abs(lib.Path, r.trashRel)))
			_, err := t.DB.ExecContext(ctx, "DELETE FROM trash WHERE id = ?", r.id)
			return err
		})
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Empty apaga de vez os itens (ids == nil: a lixeira inteira).
func (t *Trash) Empty(ctx context.Context, ids []int64) (int, error) {
	list, err := t.rows(ctx, ids)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range list {
		lib, err := t.Libraries.Get(ctx, r.libID)
		if err != nil {
			return n, err
		}
		p := abs(lib.Path, r.trashRel)
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return n, err
		}
		fsutil.RemoveEmptyDirs(lib.Path, filepath.Dir(p)) // inclusive a própria lixeira, se esvaziou
		if _, err := t.DB.ExecContext(ctx, "DELETE FROM trash WHERE id = ?", r.id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func abs(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

func (t *Trash) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}
