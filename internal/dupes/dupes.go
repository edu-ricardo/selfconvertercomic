// Package dupes acha duplicados (RF-44): arquivos com o conteúdo igual (SHA-256, calculado em segundo
// plano) e a mesma edição em pastas diferentes. Só lista — nunca apaga nada.
package dupes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Hasher calcula o hash dos arquivos que ainda não têm, um por vez.
type Hasher struct {
	DB   *sql.DB
	Idle time.Duration // espera quando não há nada (padrão 1 min)

	once sync.Once
	wake chan struct{}
}

func (h *Hasher) wakeCh() chan struct{} {
	h.once.Do(func() { h.wake = make(chan struct{}, 1) })
	return h.wake
}

// Notify acorda o cálculo (uma varredura achou arquivos novos ou alterados).
func (h *Hasher) Notify() {
	select {
	case h.wakeCh() <- struct{}{}:
	default:
	}
}

// Run calcula os hashes pendentes até ctx acabar.
func (h *Hasher) Run(ctx context.Context) {
	idle := h.Idle
	if idle == 0 {
		idle = time.Minute
	}
	for ctx.Err() == nil {
		did, err := h.Step(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("hash dos arquivos", "err", err)
		}
		if !did || err != nil {
			select {
			case <-ctx.Done():
			case <-h.wakeCh():
			case <-time.After(idle):
			}
		}
	}
}

// Step calcula o hash de um arquivo pendente (de biblioteca disponível). Devolve false se não havia
// nenhum. Se o arquivo mudou durante o cálculo, o resultado é descartado (a varredura o marcará de novo).
func (h *Hasher) Step(ctx context.Context) (bool, error) {
	var id, size, mtime int64
	var root, rel string
	err := h.DB.QueryRowContext(ctx, `SELECT f.id, f.size, f.mtime, l.path, f.rel_path FROM files f
		JOIN libraries l ON l.id = f.library_id
		WHERE f.content_hash = '' AND l.available ORDER BY f.id LIMIT 1`).Scan(&id, &size, &mtime, &root, &rel)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	sum, err := hashFile(ctx, filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// Ilegível ou sumiu: marca para não travar a fila; a próxima mudança do arquivo zera a marca.
		slog.Warn("hash: arquivo ilegível", "arquivo", rel, "err", err)
		sum = "erro"
	}
	_, err = h.DB.ExecContext(ctx, "UPDATE files SET content_hash = ? WHERE id = ? AND size = ? AND mtime = ?",
		sum, id, size, mtime)
	return true, err
}

func hashFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 256<<10)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := f.Read(buf)
		h.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// File é um arquivo de um grupo de duplicados.
type File struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	RelPath     string `json:"relPath"`
	Format      string `json:"format"`
	Size        int64  `json:"size"`
	EditionID   int64  `json:"editionId"`
}

// Group é um conjunto de duplicados.
type Group struct {
	Kind  string `json:"kind"`  // content (conteúdo idêntico) | edition (mesma edição em pastas diferentes)
	Label string `json:"label"` // o que os une (ex.: "Guerras Secretas #1 (1984)")
	Files []File `json:"files"`
}

// Report é a tela de duplicados.
type Report struct {
	Pending int     `json:"pending"` // arquivos ainda sem hash
	Total   int     `json:"total"`
	Groups  []Group `json:"groups"`
	More    bool    `json:"more"` // havia mais grupos do que o limite
}

// Find monta o relatório: primeiro os de conteúdo idêntico, depois as edições repetidas.
func Find(ctx context.Context, db *sql.DB, limit int) (Report, error) {
	r := Report{Groups: []Group{}}
	if err := db.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(content_hash = ''), 0) FROM files`).
		Scan(&r.Total, &r.Pending); err != nil {
		return r, err
	}

	// Conteúdo idêntico (em qualquer biblioteca).
	content, err := groups(ctx, db, "content", `
		SELECT f.content_hash, '', `+fileCols+` FROM files f JOIN libraries l ON l.id = f.library_id
		WHERE f.content_hash IN (SELECT content_hash FROM files WHERE content_hash NOT IN ('', 'erro')
			GROUP BY content_hash HAVING count(*) > 1)
		ORDER BY f.size DESC, f.content_hash, f.rel_path`, limit+1)
	if err != nil {
		return r, err
	}
	for i := range content {
		content[i].Label = "Conteúdo idêntico · " + humanSize(content[i].Files[0].Size)
	}

	// Mesma edição em pastas diferentes: a mesma fonte/edição aplicada, ou a mesma série + número + ano
	// identificados pelo nome. PDF e CBZ da mesma pasta são a mesma edição e não contam.
	editions, err := groups(ctx, db, "edition", `
		WITH ident AS (
			SELECT e.id, e.library_id,
				CASE WHEN e.meta_status IN ('applied', 'rewrite') AND json_extract(e.meta_json, '$.issueId') <> ''
					THEN json_extract(e.meta_json, '$.provider') || ':' || json_extract(e.meta_json, '$.issueId')
					ELSE e.series_key || '#' || e.number || '/' || e.year END AS k,
				e.series || ' #' || e.number || CASE WHEN e.year > 0 THEN ' (' || e.year || ')' ELSE '' END AS label
			FROM editions e WHERE e.number <> '' AND e.series_key <> ''
		)
		SELECT i.k, i.label, `+fileCols+` FROM ident i JOIN files f ON f.edition_id = i.id JOIN libraries l ON l.id = f.library_id
		WHERE i.k IN (SELECT k FROM ident GROUP BY k HAVING count(*) > 1)
		ORDER BY i.k, f.rel_path`, limit+1)
	if err != nil {
		return r, err
	}
	r.Groups = append(content, notCovered(editions, content)...)
	if len(r.Groups) > limit {
		r.Groups, r.More = r.Groups[:limit], true
	}
	return r, nil
}

// notCovered tira os grupos de "mesma edição" em que cada arquivo já tem uma cópia exata dentro do
// próprio grupo (árvore duplicada inteira): eles já aparecem como conteúdo idêntico. Fica o que traz
// informação nova — outra digitalização, outro formato, versões diferentes.
func notCovered(editions, content []Group) []Group {
	groupOf := map[int64]int{}
	for i, g := range content {
		for _, f := range g.Files {
			groupOf[f.ID] = i
		}
	}
	var out []Group
	for _, g := range editions {
		covered := true
		for _, f := range g.Files {
			covered = covered && hasCopy(f, g.Files, groupOf)
		}
		if !covered {
			out = append(out, g)
		}
	}
	return out
}

// hasCopy diz se outro arquivo de files tem o mesmo conteúdo que f.
func hasCopy(f File, files []File, groupOf map[int64]int) bool {
	ci, ok := groupOf[f.ID]
	if !ok {
		return false
	}
	for _, o := range files {
		if oi, has := groupOf[o.ID]; has && o.ID != f.ID && oi == ci {
			return true
		}
	}
	return false
}

const fileCols = "f.id, f.library_id, l.name, f.rel_path, f.format, f.size, coalesce(f.edition_id, 0)"

// groups junta as linhas consecutivas com a mesma chave (1ª coluna) em grupos; o rótulo (2ª coluna) é o
// da primeira linha do grupo.
func groups(ctx context.Context, db *sql.DB, kind, q string, max int) ([]Group, error) {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	lastKey := ""
	for rows.Next() {
		var key, label string
		var f File
		if err := rows.Scan(&key, &label, &f.ID, &f.LibraryID, &f.LibraryName, &f.RelPath, &f.Format, &f.Size, &f.EditionID); err != nil {
			return nil, err
		}
		if len(out) == 0 || key != lastKey {
			if len(out) >= max {
				break
			}
			out = append(out, Group{Kind: kind, Label: label})
			lastKey = key
		}
		g := &out[len(out)-1]
		g.Files = append(g.Files, f)
	}
	return out, rows.Err()
}

func humanSize(n int64) string {
	const mb = 1 << 20
	if n >= mb {
		return strconv.FormatFloat(float64(n)/mb, 'f', 1, 64) + " MB"
	}
	return strconv.FormatInt(n/1024, 10) + " KB"
}
