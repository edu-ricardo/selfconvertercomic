package organize

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/edu-ricardo/selfconvertercomic/internal/fsutil"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

// Erros para a API.
var (
	ErrNotFound    = errors.New("operação não encontrada")
	ErrUnavailable = errors.New("biblioteca indisponível (disco desconectado?)")
	ErrCannotUndo  = errors.New("não dá para desfazer")
)

// Move é um arquivo do plano.
type Move struct {
	FileID int64  `json:"fileId"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Item é uma edição do plano: os arquivos dela (PDF e CBZ) vão juntos.
type Item struct {
	EditionID int64  `json:"editionId"`
	From      string `json:"from"` // pasta/nome-base atual
	To        string `json:"to"`
	Source    string `json:"source"`  // "fontes" (metadados aplicados) ou "nome" (identificado pelo nome)
	Problem   string `json:"problem"` // "" = pode aplicar
	Files     []Move `json:"files"`
}

// Plan é a prévia: só as edições que mudariam de lugar.
type Plan struct {
	Template  string `json:"template"`
	Items     []Item `json:"items"`
	Unchanged int    `json:"unchanged"` // já no lugar
}

// Op é uma aplicação do plano no histórico.
type Op struct {
	ID        int64   `json:"id"`
	LibraryID int64   `json:"libraryId"`
	Template  string  `json:"template"`
	Status    string  `json:"status"` // done | partial | undone
	Error     string  `json:"error"`
	Moves     int     `json:"moves"` // arquivos movidos
	Editions  int     `json:"editions"`
	CreatedAt string  `json:"createdAt"`
	UndoneAt  *string `json:"undoneAt"`
}

// Organizer calcula e aplica planos.
type Organizer struct {
	DB        *sql.DB
	Libraries *library.Store
	// Exclusive roda fn sem varredura da biblioteca ao mesmo tempo (library.Manager.Exclusive).
	Exclusive func(libraryID int64, fn func() error) error
}

type fileRow struct {
	id   int64
	rel  string
	ext  string
	edID int64
}

type editionRow struct {
	id                     int64
	dir, stem              string
	series, volume, number string
	year                   int
	applied                *metadata.Details
	folderPublisher        string
	files                  []fileRow
}

func (o *Organizer) library(ctx context.Context, id int64) (library.Library, error) {
	l, err := o.Libraries.Get(ctx, id)
	if err != nil {
		return l, err
	}
	if !l.Available {
		return l, ErrUnavailable
	}
	return l, nil
}

// load lê as edições da biblioteca (de uma série, se seriesKey != ""; só as de ids, se não nulo).
func (o *Organizer) load(ctx context.Context, libID int64, seriesKey string, ids map[int64]bool) ([]*editionRow, error) {
	q := `SELECT e.id, e.dir, e.stem, e.series, e.volume, e.number, e.year, e.meta_json,
			f.id, f.rel_path, f.publisher
		FROM editions e JOIN files f ON f.edition_id = e.id WHERE e.library_id = ?`
	args := []any{libID}
	if seriesKey != "" {
		q += " AND e.series_key = ?"
		args = append(args, seriesKey)
	}
	rows, err := o.DB.QueryContext(ctx, q+" ORDER BY e.id, f.rel_path", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*editionRow
	byID := map[int64]*editionRow{}
	for rows.Next() {
		var e editionRow
		var meta string
		var f fileRow
		var pub string
		if err := rows.Scan(&e.id, &e.dir, &e.stem, &e.series, &e.volume, &e.number, &e.year, &meta, &f.id, &f.rel, &pub); err != nil {
			return nil, err
		}
		if ids != nil && !ids[e.id] {
			continue
		}
		cur := byID[e.id]
		if cur == nil {
			cur = &e
			cur.folderPublisher = pub
			if meta != "" {
				cur.applied = &metadata.Details{}
				if err := json.Unmarshal([]byte(meta), cur.applied); err != nil {
					return nil, err
				}
			}
			byID[e.id] = cur
			list = append(list, cur)
		}
		f.edID, f.ext = e.id, path.Ext(f.rel)
		cur.files = append(cur.files, f)
	}
	return list, rows.Err()
}

// values monta as variáveis do template: o que veio das fontes tem preferência sobre o nome.
func (e *editionRow) values() (map[string]string, string) {
	v := map[string]string{"serie": e.series, "numero": e.number, "volume": e.volume, "original": e.stem,
		"editora": e.folderPublisher}
	if e.year != 0 {
		v["ano"] = strconv.Itoa(e.year)
	}
	source := "nome"
	if d := e.applied; d != nil {
		source = "fontes"
		set := func(k, val string) {
			if strings.TrimSpace(val) != "" {
				v[k] = val
			}
		}
		set("serie", d.Series)
		set("numero", d.Number)
		set("editora", d.Publisher)
		set("titulo", d.Title)
		if d.Year != 0 {
			v["ano"] = strconv.Itoa(d.Year)
		}
	}
	v["numero3"] = pad3(v["numero"])
	return v, source
}

// pad3: "1" → "001"; números com fração ou letras ficam como estão.
func pad3(n string) string {
	if i, err := strconv.Atoi(n); err == nil && i >= 0 && i < 1000 {
		return fmt.Sprintf("%03d", i)
	}
	return n
}

// Plan calcula a prévia para a biblioteca (ou uma série dela).
func (o *Organizer) Plan(ctx context.Context, libID int64, seriesKey, tpl string) (Plan, error) {
	if err := CheckTemplate(tpl); err != nil {
		return Plan{}, err
	}
	lib, err := o.library(ctx, libID)
	if err != nil {
		return Plan{}, err
	}
	eds, err := o.load(ctx, libID, seriesKey, nil)
	if err != nil {
		return Plan{}, err
	}
	return o.plan(ctx, lib, eds, tpl)
}

func (o *Organizer) plan(ctx context.Context, lib library.Library, eds []*editionRow, tpl string) (Plan, error) {
	p := Plan{Template: tpl, Items: []Item{}}
	// Todos os arquivos atuais da biblioteca: um destino que já é de outro arquivo é conflito.
	current := map[string]int64{}
	rows, err := o.DB.QueryContext(ctx, "SELECT id, rel_path FROM files WHERE library_id = ?", lib.ID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var id int64
		var rel string
		if err := rows.Scan(&id, &rel); err != nil {
			rows.Close()
			return p, err
		}
		current[strings.ToLower(rel)] = id
	}
	rows.Close()

	targets := map[string]int{} // destino (sem caixa) → quantas edições querem ir para lá
	for _, e := range eds {
		v, source := e.values()
		to, missing := Render(tpl, v)
		from := path.Join(e.dir, e.stem)
		it := Item{EditionID: e.id, From: from, To: to, Source: source}
		if len(missing) > 0 {
			it.Problem = "falta " + strings.Join(missing, " e ")
		}
		if it.Problem == "" && to == from {
			p.Unchanged++
			continue
		}
		for _, f := range e.files {
			m := Move{FileID: f.id, From: f.rel, To: to + f.ext}
			it.Files = append(it.Files, m)
			if it.Problem != "" {
				continue
			}
			key := strings.ToLower(m.To)
			targets[key]++
			if owner, ok := current[key]; ok && owner != f.id {
				it.Problem = "já existe um arquivo em " + m.To
			} else if !ok {
				if _, err := os.Lstat(filepath.Join(lib.Path, filepath.FromSlash(m.To))); err == nil && !strings.EqualFold(m.To, m.From) {
					it.Problem = "já existe um arquivo em " + m.To
				}
			}
		}
		p.Items = append(p.Items, it)
	}
	for i := range p.Items {
		it := &p.Items[i]
		for _, f := range it.Files {
			if it.Problem == "" && targets[strings.ToLower(f.To)] > 1 {
				it.Problem = "outra edição iria para o mesmo nome (duplicada?)"
			}
		}
	}
	sort.SliceStable(p.Items, func(i, j int) bool { return naturalLess(p.Items[i].To, p.Items[j].To) })
	return p, nil
}

// Apply refaz o plano só para as edições aprovadas e aplica as que não têm problema, uma por vez:
// move os arquivos (PDF e CBZ juntos) e atualiza o banco; uma falha desfaz a edição em andamento e
// para (status partial). Tudo sem varredura ao mesmo tempo.
func (o *Organizer) Apply(ctx context.Context, libID int64, tpl string, editionIDs []int64) (Op, error) {
	if err := CheckTemplate(tpl); err != nil {
		return Op{}, err
	}
	lib, err := o.library(ctx, libID)
	if err != nil {
		return Op{}, err
	}
	ids := make(map[int64]bool, len(editionIDs))
	for _, id := range editionIDs {
		ids[id] = true
	}
	var opID int64
	err = o.Exclusive(libID, func() error {
		eds, err := o.load(ctx, libID, "", ids)
		if err != nil {
			return err
		}
		p, err := o.plan(ctx, lib, eds, tpl)
		if err != nil {
			return err
		}
		res, err := o.DB.ExecContext(ctx, "INSERT INTO organize_ops (library_id, template, status) VALUES (?, ?, 'running')", libID, tpl)
		if err != nil {
			return err
		}
		opID, _ = res.LastInsertId()
		status, msg := "done", ""
		var dirs []string
		for _, it := range p.Items {
			if it.Problem != "" {
				continue
			}
			if err := o.moveEdition(ctx, lib, opID, it, false); err != nil {
				status, msg = "partial", fmt.Sprintf("%s: %v", it.From, err)
				break
			}
			for _, f := range it.Files {
				dirs = append(dirs, filepath.Dir(filepath.Join(lib.Path, filepath.FromSlash(f.From))))
			}
		}
		for _, d := range dirs {
			fsutil.RemoveEmptyDirs(lib.Path, d)
		}
		_, err = o.DB.ExecContext(ctx, "UPDATE organize_ops SET status = ?, error = ? WHERE id = ?", status, msg, opID)
		return err
	})
	if err != nil {
		return Op{}, err
	}
	return o.Get(ctx, opID)
}

// moveEdition move os arquivos de uma edição e atualiza files, editions, tarefas e o histórico numa
// transação. reverse = desfazendo (não registra movimentos). Se algo falha, devolve os arquivos já
// movidos ao lugar de antes.
func (o *Organizer) moveEdition(ctx context.Context, lib library.Library, opID int64, it Item, reverse bool) error {
	abs := func(rel string) string { return filepath.Join(lib.Path, filepath.FromSlash(rel)) }
	var done []Move
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			if err := fsutil.Move(abs(done[i].To), abs(done[i].From)); err != nil {
				slog.Error("organização: não consegui devolver o arquivo", "de", done[i].To, "para", done[i].From, "err", err)
			}
		}
	}
	for _, m := range it.Files {
		if err := fsutil.Move(abs(m.From), abs(m.To)); err != nil {
			rollback()
			return err
		}
		done = append(done, m)
	}
	tx, err := o.DB.BeginTx(ctx, nil)
	if err != nil {
		rollback()
		return err
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	}
	dir, stem := path.Split(it.To)
	fromDir, fromStem := path.Split(it.From)
	steps := []func() error{
		func() error {
			return exec("UPDATE editions SET dir = ?, stem = ? WHERE id = ?", strings.TrimSuffix(dir, "/"), stem, it.EditionID)
		},
		func() error {
			return exec("UPDATE tasks SET rel_path = ? WHERE kind = ? AND library_id = ? AND rel_path = ?",
				it.To, tasks.KindMetadataConfirm, lib.ID, path.Join(strings.TrimSuffix(fromDir, "/"), fromStem))
		},
	}
	for _, m := range it.Files {
		m := m
		steps = append(steps, func() error { return exec("UPDATE files SET rel_path = ? WHERE id = ?", m.To, m.FileID) })
		if !reverse {
			steps = append(steps, func() error {
				return exec("INSERT INTO organize_moves (op_id, edition_id, file_id, from_rel, to_rel) VALUES (?, ?, ?, ?, ?)",
					opID, it.EditionID, m.FileID, m.From, m.To)
			})
		}
	}
	for _, s := range steps {
		if err := s(); err != nil {
			rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		rollback()
		return err
	}
	return nil
}

// Undo devolve os arquivos de uma operação ao lugar de antes, se nada mudou desde então.
func (o *Organizer) Undo(ctx context.Context, opID int64) (Op, error) {
	op, err := o.Get(ctx, opID)
	if err != nil {
		return op, err
	}
	if op.Status == "undone" {
		return op, fmt.Errorf("%w: a operação já foi desfeita", ErrCannotUndo)
	}
	lib, err := o.library(ctx, op.LibraryID)
	if err != nil {
		return op, err
	}
	rows, err := o.DB.QueryContext(ctx, `SELECT edition_id, file_id, from_rel, to_rel FROM organize_moves WHERE op_id = ? ORDER BY id`, opID)
	if err != nil {
		return op, err
	}
	var items []Item
	byEdition := map[int64]int{}
	for rows.Next() {
		var edID int64
		var m Move
		if err := rows.Scan(&edID, &m.FileID, &m.From, &m.To); err != nil {
			rows.Close()
			return op, err
		}
		// Desfazer = mover de To para From.
		back := Move{FileID: m.FileID, From: m.To, To: m.From}
		i, ok := byEdition[edID]
		if !ok {
			i = len(items)
			byEdition[edID] = i
			items = append(items, Item{EditionID: edID, From: stemOf(m.To), To: stemOf(m.From)})
		}
		items[i].Files = append(items[i].Files, back)
	}
	rows.Close()

	err = o.Exclusive(op.LibraryID, func() error {
		// Confere tudo antes de mexer: cada arquivo continua onde a operação o deixou e o lugar antigo
		// está livre.
		for _, it := range items {
			for _, m := range it.Files {
				if _, err := os.Lstat(filepath.Join(lib.Path, filepath.FromSlash(m.From))); err != nil {
					return fmt.Errorf("%w: %s não está mais lá", ErrCannotUndo, m.From)
				}
				if _, err := os.Lstat(filepath.Join(lib.Path, filepath.FromSlash(m.To))); err == nil && !strings.EqualFold(m.To, m.From) {
					return fmt.Errorf("%w: já existe outro arquivo em %s", ErrCannotUndo, m.To)
				}
			}
		}
		for i := len(items) - 1; i >= 0; i-- {
			if err := o.moveEdition(ctx, lib, opID, items[i], true); err != nil {
				return fmt.Errorf("desfazendo %s: %w", items[i].From, err)
			}
			for _, m := range items[i].Files {
				fsutil.RemoveEmptyDirs(lib.Path, filepath.Dir(filepath.Join(lib.Path, filepath.FromSlash(m.From))))
			}
		}
		_, err := o.DB.ExecContext(ctx, `UPDATE organize_ops SET status = 'undone',
			undone_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, opID)
		return err
	})
	if err != nil {
		return op, err
	}
	return o.Get(ctx, opID)
}

// naturalLess compara textos com os números pelo valor ("#2" antes de "#10").
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digitPrefix(a), digitPrefix(b)
		switch {
		case da != "" && db != "":
			na, _ := strconv.Atoi(da)
			nb, _ := strconv.Atoi(db)
			if na != nb {
				return na < nb
			}
			a, b = a[len(da):], b[len(db):]
		case a[0] != b[0]:
			return a[0] < b[0]
		default:
			a, b = a[1:], b[1:]
		}
	}
	return len(a) < len(b)
}

func digitPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func stemOf(rel string) string { return strings.TrimSuffix(rel, path.Ext(rel)) }

const opColumns = `o.id, o.library_id, o.template, o.status, o.error, o.created_at, o.undone_at,
	(SELECT count(*) FROM organize_moves m WHERE m.op_id = o.id),
	(SELECT count(DISTINCT edition_id) FROM organize_moves m WHERE m.op_id = o.id)`

func scanOp(row interface{ Scan(...any) error }) (Op, error) {
	var op Op
	err := row.Scan(&op.ID, &op.LibraryID, &op.Template, &op.Status, &op.Error, &op.CreatedAt, &op.UndoneAt, &op.Moves, &op.Editions)
	return op, err
}

// Get devolve uma operação.
func (o *Organizer) Get(ctx context.Context, id int64) (Op, error) {
	op, err := scanOp(o.DB.QueryRowContext(ctx, "SELECT "+opColumns+" FROM organize_ops o WHERE o.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return op, ErrNotFound
	}
	return op, err
}

// History lista as operações de uma biblioteca, mais recentes primeiro.
func (o *Organizer) History(ctx context.Context, libID int64, limit int) ([]Op, error) {
	rows, err := o.DB.QueryContext(ctx, "SELECT "+opColumns+" FROM organize_ops o WHERE o.library_id = ? ORDER BY o.id DESC LIMIT ?", libID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Op{}
	for rows.Next() {
		op, err := scanOp(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, op)
	}
	return list, rows.Err()
}
