package library

import (
	"context"
	"database/sql"
	"path"
	"strings"

	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
)

type editionKey struct{ dir, stem string }

func splitEdition(rel string) editionKey {
	dir, name := path.Split(rel)
	return editionKey{strings.TrimSuffix(dir, "/"), strings.TrimSuffix(name, path.Ext(name))}
}

type editionRow struct {
	id     int64
	key    editionKey
	byName bool // source = 'name': campos vêm do analisador e podem ser recalculados
	guess  metadata.Guess
}

// syncEditions liga cada arquivo da biblioteca à sua edição (criando as que faltam), reanalisa as
// edições identificadas pelo nome com versão antiga do analisador, apaga as edições sem arquivos e
// refaz a inferência de números pela ordem de leitura.
func syncEditions(ctx context.Context, tx *sql.Tx, libID int64) error {
	eds, err := loadEditions(ctx, tx, libID)
	if err != nil {
		return err
	}
	byKey := make(map[editionKey]*editionRow, len(eds))
	for _, e := range eds {
		byKey[e.key] = e
	}

	rows, err := tx.QueryContext(ctx, "SELECT id, rel_path, coalesce(edition_id, 0) FROM files WHERE library_id = ?", libID)
	if err != nil {
		return err
	}
	type link struct {
		fileID, editionID int64
		key               editionKey
		rel               string
	}
	var links []link
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.fileID, &l.rel, &l.editionID); err != nil {
			rows.Close()
			return err
		}
		l.key = splitEdition(l.rel)
		links = append(links, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	insert, err := tx.PrepareContext(ctx, `INSERT INTO editions (library_id, dir, stem, parser_version, meta_status)
		VALUES (?, ?, ?, 0, 'new')`)
	if err != nil {
		return err
	}
	defer insert.Close()
	setFile, err := tx.PrepareContext(ctx, "UPDATE files SET edition_id = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer setFile.Close()
	// CBZ novo (ex.: acabou de ser convertido) numa edição com metadados aplicados: falta gravar o
	// ComicInfo.xml nele.
	rewrite, err := tx.PrepareContext(ctx, `UPDATE editions SET meta_status = 'rewrite' WHERE id = ? AND meta_status = 'applied'`)
	if err != nil {
		return err
	}
	defer rewrite.Close()
	for _, l := range links {
		e := byKey[l.key]
		if e == nil {
			res, err := insert.ExecContext(ctx, libID, l.key.dir, l.key.stem)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			e = &editionRow{id: id, key: l.key, byName: true}
			byKey[l.key] = e
			eds = append(eds, e)
		}
		if e.id != l.editionID {
			if _, err := setFile.ExecContext(ctx, e.id, l.fileID); err != nil {
				return err
			}
			if strings.EqualFold(path.Ext(l.rel), ".cbz") {
				if _, err := rewrite.ExecContext(ctx, e.id); err != nil {
					return err
				}
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM editions WHERE library_id = ?
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.edition_id = editions.id)`, libID); err != nil {
		return err
	}

	// Análise pelo nome e inferência de números sobre todas as edições identificadas pelo nome que ainda
	// têm arquivos (a inferência olha a série inteira, então é refeita a cada sincronização).
	used := map[editionKey]bool{}
	for _, l := range links {
		used[l.key] = true
	}
	var named []*editionRow
	var guesses []metadata.Guess
	for _, e := range eds {
		if e.byName && used[e.key] {
			named = append(named, e)
			guesses = append(guesses, metadata.ParseName(path.Join(e.key.dir, e.key.stem+".x")))
		}
	}
	metadata.InferNumbers(guesses)
	update, err := tx.PrepareContext(ctx, `UPDATE editions SET series = ?, series_key = ?, volume = ?, number = ?,
		number_inferred = ?, count = ?, year = ?, grp = ?, read_order = ?, parser_version = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer update.Close()
	for i, e := range named {
		g := guesses[i]
		if g == e.guess && e.guess != (metadata.Guess{}) {
			continue
		}
		if _, err := update.ExecContext(ctx, g.Series, g.Key(), g.Volume, g.Number, g.NumberInferred, g.Count,
			g.Year, g.Group, g.Order, metadata.ParserVersion, e.id); err != nil {
			return err
		}
	}
	return nil
}

func loadEditions(ctx context.Context, tx *sql.Tx, libID int64) ([]*editionRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, dir, stem, source = 'name', parser_version = ?, series, volume, number,
		number_inferred, count, year, grp, read_order FROM editions WHERE library_id = ?`, metadata.ParserVersion, libID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*editionRow
	for rows.Next() {
		e := &editionRow{}
		var current bool
		g := &e.guess
		if err := rows.Scan(&e.id, &e.key.dir, &e.key.stem, &e.byName, &current, &g.Series, &g.Volume, &g.Number,
			&g.NumberInferred, &g.Count, &g.Year, &g.Group, &g.Order); err != nil {
			return nil, err
		}
		if !current {
			e.guess = metadata.Guess{} // versão antiga do analisador: força a regravação
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// editionsStale diz se há edições identificadas pelo nome com versão antiga do analisador.
func editionsStale(ctx context.Context, db *sql.DB, libID int64) (bool, error) {
	var stale bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM editions WHERE library_id = ? AND source = 'name'
		AND parser_version <> ?)`, libID, metadata.ParserVersion).Scan(&stale)
	return stale, err
}
