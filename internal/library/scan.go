package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Result resume uma varredura.
type Result struct {
	Added, Updated, Removed, Total int
	Duration                       time.Duration
	Dirs                           []string // pastas visitadas (usadas pelo watcher)
}

// ErrUnavailable indica que a pasta da biblioteca não está acessível (disco desconectado?).
var ErrUnavailable = errors.New("biblioteca indisponível")

var formats = map[string]string{".pdf": "pdf", ".cbz": "cbz", ".cbr": "cbr"}

type fileState struct {
	id          int64
	size, mtime int64
	derived     bool // publisher/series/title preenchidos e ligado a uma edição
}

// Derive calcula editora, série e título a partir do caminho relativo (regra da entrega 1):
// editora = primeira pasta (se o arquivo não está na raiz), série = pasta do arquivo, título = nome
// sem extensão.
func Derive(relPath string) (publisher, series, title string) {
	dir, name := path.Split(relPath)
	series = strings.TrimSuffix(dir, "/")
	title = strings.TrimSuffix(name, path.Ext(name))
	if series != "" {
		publisher, _, _ = strings.Cut(series, "/")
	}
	return publisher, series, title
}

// Scan varre a biblioteca e sincroniza a tabela files: insere novos, atualiza alterados (tamanho ou
// mtime) e remove os que sumiram. Se a pasta não existe — ou está vazia enquanto o catálogo não está,
// sinal de disco desmontado — marca a biblioteca indisponível e não apaga nada.
func (s *Store) Scan(ctx context.Context, lib Library) (Result, error) {
	start := time.Now()
	var res Result

	known, err := s.knownFiles(ctx, lib.ID)
	if err != nil {
		return res, err
	}
	if reason := unavailableReason(lib.Path, len(known)); reason != "" {
		if err := s.setStatus(ctx, lib.ID, false, reason, false); err != nil {
			return res, err
		}
		return res, fmt.Errorf("%w: %s", ErrUnavailable, reason)
	}

	type found struct {
		rel, format string
		size, mtime int64
	}
	var changed []found
	added := map[string]bool{} // arquivos que não estavam no catálogo
	seen := make(map[string]bool, len(known))

	walkErr := filepath.WalkDir(lib.Path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == lib.Path {
				return err
			}
			return nil // subpasta ilegível: ignora e segue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := d.Name()
		if d.IsDir() {
			if p != lib.Path && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			res.Dirs = append(res.Dirs, p)
			return nil
		}
		format, ok := formats[strings.ToLower(filepath.Ext(name))]
		if !ok || strings.HasPrefix(name, ".") || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // sumiu durante a varredura
		}
		rel, _ := filepath.Rel(lib.Path, p)
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		size, mtime := info.Size(), info.ModTime().UnixNano()
		if k, ok := known[rel]; !ok || k.size != size || k.mtime != mtime || !k.derived {
			changed = append(changed, found{rel, format, size, mtime})
		}
		return nil
	})
	if walkErr != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		_ = s.setStatus(ctx, lib.ID, false, walkErr.Error(), false)
		return res, fmt.Errorf("%w: %v", ErrUnavailable, walkErr)
	}

	stale, err := editionsStale(ctx, s.db, lib.ID)
	if err != nil {
		return res, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	upsert, err := tx.PrepareContext(ctx, `
		INSERT INTO files (library_id, rel_path, format, size, mtime, publisher, series, title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, rel_path) DO UPDATE SET format = excluded.format, size = excluded.size,
			content_hash = CASE WHEN files.size = excluded.size AND files.mtime = excluded.mtime
				THEN files.content_hash ELSE '' END,
			mtime = excluded.mtime, publisher = excluded.publisher, series = excluded.series,
			title = excluded.title, changed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`)
	if err != nil {
		return res, err
	}
	defer upsert.Close()
	for _, f := range changed {
		pub, series, title := Derive(f.rel)
		if _, err := upsert.ExecContext(ctx, lib.ID, f.rel, f.format, f.size, f.mtime, pub, series, title); err != nil {
			return res, err
		}
		if k, ok := known[f.rel]; ok {
			if k.size == f.size && k.mtime == f.mtime {
				continue // só preencheu os campos derivados (banco migrado)
			}
			res.Updated++
		} else {
			res.Added++
			added[f.rel] = true
		}
	}

	del, err := tx.PrepareContext(ctx, "DELETE FROM files WHERE id = ?")
	if err != nil {
		return res, err
	}
	defer del.Close()
	for rel, k := range known {
		if !seen[rel] {
			if _, err := del.ExecContext(ctx, k.id); err != nil {
				return res, err
			}
			res.Removed++
		}
	}
	if len(changed) > 0 || res.Removed > 0 || stale {
		if err := syncEditions(ctx, tx, lib.ID, added); err != nil {
			return res, err
		}
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}

	res.Total = len(seen)
	res.Duration = time.Since(start)
	return res, s.setStatus(ctx, lib.ID, true, "", true)
}

func (s *Store) knownFiles(ctx context.Context, libID int64) (map[string]fileState, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, rel_path, size, mtime, title <> '' AND edition_id IS NOT NULL FROM files WHERE library_id = ?", libID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]fileState{}
	for rows.Next() {
		var rel string
		var f fileState
		if err := rows.Scan(&f.id, &rel, &f.size, &f.mtime, &f.derived); err != nil {
			return nil, err
		}
		m[rel] = f
	}
	return m, rows.Err()
}

// unavailableReason devolve "" quando a pasta parece acessível.
func unavailableReason(path string, knownFiles int) string {
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return "pasta não encontrada (disco desconectado?)"
	case !fi.IsDir():
		return "o caminho não é uma pasta"
	}
	if knownFiles > 0 {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "pasta ilegível: " + err.Error()
		}
		if len(entries) == 0 {
			return "pasta vazia, mas o catálogo tem arquivos (disco desmontado?)"
		}
	}
	return ""
}
