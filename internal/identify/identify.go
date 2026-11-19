// Package identify procura e aplica metadados às edições em segundo plano: busca nas fontes, aplica
// sozinho quando a confiança é alta e abre uma tarefa de confirmação quando não é (RF-53); grava o
// ComicInfo.xml nos CBZ da edição (RF-54).
package identify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/convert"
	"github.com/edu-ricardo/selfconvertercomic/internal/metadata"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/edu-ricardo/selfconvertercomic/internal/tasks"
)

// Status de metadados de uma edição (ver a migração 0008).
const (
	StatusNone    = ""
	StatusNew     = "new"
	StatusQueued  = "queued"
	StatusApplied = "applied"
	StatusRewrite = "rewrite"
	StatusTask    = "task"
	StatusError   = "error"
)

// ErrNotFound indica edição inexistente.
var ErrNotFound = errors.New("edição não encontrada")

// Edition é o estado de metadados de uma edição, para a UI.
type Edition struct {
	ID         int64                `json:"id"`
	LibraryID  int64                `json:"libraryId"`
	Key        string               `json:"key"` // pasta/nome-base
	Guess      metadata.Guess       `json:"guess"`
	Status     string               `json:"status"`
	Error      string               `json:"error"`
	Score      int                  `json:"score"`
	Applied    *metadata.Details    `json:"applied"`
	Candidates []metadata.Candidate `json:"candidates"`
}

// Identifier processa as edições pedidas, uma por vez (as fontes já limitam o ritmo).
type Identifier struct {
	DB       *sql.DB
	Finder   *metadata.Finder
	Tasks    *tasks.Store
	Settings func(context.Context) (settings.Settings, error)
	Idle     time.Duration // espera quando não há nada a fazer (padrão 1 min)
	Pause    time.Duration // espera mínima depois de bater no limite de uma fonte (padrão 15 min)
	Now      func() time.Time

	once sync.Once
	wake chan struct{}
}

func (i *Identifier) wakeCh() chan struct{} {
	i.once.Do(func() { i.wake = make(chan struct{}, 1) })
	return i.wake
}

// Notify acorda o processamento (algo foi pedido).
func (i *Identifier) Notify() {
	select {
	case i.wakeCh() <- struct{}{}:
	default:
	}
}

// Get devolve o estado de metadados da edição.
func (i *Identifier) Get(ctx context.Context, id int64) (Edition, error) {
	var e Edition
	var dir, stem, applied, cands string
	g := &e.Guess
	err := i.DB.QueryRowContext(ctx, `SELECT id, library_id, dir, stem, series, volume, number, number_inferred, count, year,
		meta_status, meta_error, meta_score, meta_json, meta_candidates FROM editions WHERE id = ?`, id).
		Scan(&e.ID, &e.LibraryID, &dir, &stem, &g.Series, &g.Volume, &g.Number, &g.NumberInferred, &g.Count, &g.Year,
			&e.Status, &e.Error, &e.Score, &applied, &cands)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	e.Key = path.Join(dir, stem)
	if applied != "" {
		e.Applied = &metadata.Details{}
		if err := json.Unmarshal([]byte(applied), e.Applied); err != nil {
			return e, err
		}
	}
	e.Candidates = []metadata.Candidate{}
	if cands != "" {
		if err := json.Unmarshal([]byte(cands), &e.Candidates); err != nil {
			return e, err
		}
	}
	return e, nil
}

// QueueEdition pede a identificação de uma edição (de novo, se já tiver metadados).
func (i *Identifier) QueueEdition(ctx context.Context, id int64) error {
	res, err := i.DB.ExecContext(ctx, "UPDATE editions SET meta_status = 'queued', meta_error = '' WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	i.Notify()
	return nil
}

// QueueLibrary pede a identificação das edições da biblioteca que ainda não têm metadados nem
// tarefa aberta (as com tarefa esperam a confirmação). Devolve quantas entraram na fila.
func (i *Identifier) QueueLibrary(ctx context.Context, libraryID int64) (int, error) {
	return i.queue(ctx, "library_id = ? AND meta_status IN ('', 'new', 'error')", libraryID)
}

// QueueSeries pede a identificação das edições de uma série ainda sem metadados (inclui as com tarefa:
// é um "tentar de novo" da série inteira).
func (i *Identifier) QueueSeries(ctx context.Context, libraryID int64, seriesKey string) (int, error) {
	return i.queue(ctx, "library_id = ? AND series_key = ? AND meta_status IN ('', 'new', 'error', 'task')", libraryID, seriesKey)
}

func (i *Identifier) queue(ctx context.Context, where string, args ...any) (int, error) {
	res, err := i.DB.ExecContext(ctx, "UPDATE editions SET meta_status = 'queued', meta_error = '' WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		i.Notify()
	}
	return int(n), nil
}

// Run processa a fila até ctx acabar.
func (i *Identifier) Run(ctx context.Context) {
	idle, pause := i.Idle, i.Pause
	if idle == 0 {
		idle = time.Minute
	}
	if pause == 0 {
		pause = 15 * time.Minute
	}
	unavailable := min(pause, 5*time.Minute)
	for ctx.Err() == nil {
		id, status, err := i.next(ctx)
		wait, hold := time.Duration(0), false
		switch {
		case err != nil:
			slog.Error("identificação: próxima edição", "err", err)
			wait = idle
		case id == 0:
			wait = idle
		default:
			err := i.process(ctx, id, status)
			if errors.Is(err, metadata.ErrRateLimited) {
				// Pausa firme: uma varredura nova (Notify) não a encurta, senão a fonte é martelada.
				wait, hold = max(pause, metadata.RetryAfter(err)), true
				slog.Warn("identificação pausada: limite de pedidos de uma fonte", "pausa", wait.Round(time.Second))
			} else if errors.Is(err, metadata.ErrUnavailable) {
				wait, hold = unavailable, true
				slog.Warn("identificação pausada: fonte fora do ar ou lenta", "pausa", wait, "err", err)
			} else if err != nil && ctx.Err() == nil {
				// Sai da fila com o erro à vista (senão a mesma edição voltaria na hora, sem fim).
				slog.Error("identificação", "edição", id, "err", err)
				if err := i.setError(ctx, id, err.Error()); err != nil {
					slog.Error("identificação: gravando erro", "err", err)
					wait = idle
				}
			}
		}
		if wait > 0 {
			wake := i.wakeCh()
			if hold {
				wake = nil // nunca recebe
			}
			select {
			case <-ctx.Done():
			case <-wake:
			case <-time.After(wait):
			}
		}
	}
}

// next escolhe a próxima edição: primeiro as que só precisam regravar o ComicInfo, depois as pedidas
// e, com a identificação automática ligada, as novas. Só de bibliotecas disponíveis.
func (i *Identifier) next(ctx context.Context) (int64, string, error) {
	s, err := i.Settings(ctx)
	if err != nil {
		return 0, "", err
	}
	var id int64
	var status string
	err = i.DB.QueryRowContext(ctx, `SELECT e.id, e.meta_status FROM editions e JOIN libraries l ON l.id = e.library_id
		WHERE l.available AND (e.meta_status IN ('rewrite', 'queued') OR (? AND e.meta_status = 'new'))
		ORDER BY e.meta_status = 'rewrite' DESC, e.meta_status = 'queued' DESC, e.id LIMIT 1`, s.Metadata.AutoIdentify).
		Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return id, status, err
}

func (i *Identifier) process(ctx context.Context, id int64, status string) error {
	e, err := i.Get(ctx, id)
	if err != nil {
		return err
	}
	if status == StatusRewrite && e.Applied != nil {
		return i.write(ctx, e, *e.Applied, e.Score)
	}
	return i.identify(ctx, e)
}

// identify busca a edição nas fontes e aplica ou abre a tarefa.
func (i *Identifier) identify(ctx context.Context, e Edition) error {
	res, err := i.Finder.Search(ctx, e.Guess)
	if errors.Is(err, metadata.ErrNoProvider) {
		return i.setError(ctx, e.ID, err.Error())
	}
	if err != nil {
		return err
	}
	if len(res.Candidates) == 0 && len(res.Errs) > 0 {
		for _, err := range res.Errs {
			if transient(err) {
				return err // continua na fila; tenta depois da pausa
			}
		}
		return i.setError(ctx, e.ID, strings.Join(res.Errors, "; "))
	}
	cands := res.Candidates[:min(len(res.Candidates), 5)]
	if c, ok := confident(cands); ok {
		return i.Apply(ctx, e.ID, c.Provider, c.IssueID, c.Score, cands)
	}
	return i.openTask(ctx, e, cands)
}

// transient: falha que passa sozinha (limite de pedidos, fonte lenta ou fora do ar). A edição fica na
// fila e o processamento pausa, em vez de marcá-la com erro.
func transient(err error) bool {
	return errors.Is(err, metadata.ErrRateLimited) || errors.Is(err, metadata.ErrUnavailable)
}

// confident devolve o melhor candidato quando ele passa do limiar e nenhum outro diferente também
// passa (ex.: duas séries homônimas do mesmo ano). O mesmo número na outra fonte não conta como
// diferente (GCD e ComicVine concordando).
func confident(cands []metadata.Candidate) (metadata.Candidate, bool) {
	if len(cands) == 0 || cands[0].Score < metadata.AutoApplyScore || cands[0].IssueID == "" {
		return metadata.Candidate{}, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		same := metadata.Fold(c.Series) == metadata.Fold(best.Series) && c.Number == best.Number && c.Year == best.Year
		if c.Score >= metadata.AutoApplyScore && !same {
			return metadata.Candidate{}, false
		}
	}
	return best, true
}

func (i *Identifier) openTask(ctx context.Context, e Edition, cands []metadata.Candidate) error {
	data, _ := json.Marshal(cands)
	if _, err := i.DB.ExecContext(ctx, `UPDATE editions SET meta_status = 'task', meta_error = '', meta_candidates = ?,
		meta_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, string(data), e.ID); err != nil {
		return err
	}
	fileID, err := i.fileID(ctx, e.ID)
	if err != nil {
		return err
	}
	title := "Confirmar metadados: " + describe(e.Guess)
	detail := "Nenhuma edição encontrada nas fontes. Busque com outro nome na página do quadrinho."
	if len(cands) > 0 {
		c := cands[0]
		detail = fmt.Sprintf("Melhor candidato: %s #%s (%d), %s, nota %d de %d necessários.",
			c.Series, c.Number, c.Year, providerName(c.Provider), c.Score, metadata.AutoApplyScore)
	}
	return i.Tasks.Open(ctx, tasks.KindMetadataConfirm, e.LibraryID, e.Key, fileID, title, detail)
}

// Apply busca os detalhes da edição escolhida e os aplica (ComicInfo.xml nos CBZ e banco), fechando a
// tarefa de confirmação. cands, se não nulo, substitui os candidatos guardados.
func (i *Identifier) Apply(ctx context.Context, id int64, provider, issueID string, score int, cands []metadata.Candidate) error {
	e, err := i.Get(ctx, id)
	if err != nil {
		return err
	}
	d, err := i.Finder.Details(ctx, provider, issueID)
	if err != nil {
		return err
	}
	if cands != nil {
		data, _ := json.Marshal(cands)
		if _, err := i.DB.ExecContext(ctx, "UPDATE editions SET meta_candidates = ? WHERE id = ?", string(data), id); err != nil {
			return err
		}
	}
	return i.write(ctx, e, d, score)
}

// ManualScore é a nota de metadados escolhidos ou preenchidos pelo usuário.
const ManualScore = 100

// ApplyManual grava metadados preenchidos à mão (fonte "manual"): limpa espaços e nomes vazios nas
// listas e exige a série.
func (i *Identifier) ApplyManual(ctx context.Context, id int64, d metadata.Details) error {
	e, err := i.Get(ctx, id)
	if err != nil {
		return err
	}
	d.Provider, d.IssueID, d.URL, d.CoverURL = "manual", "", "", ""
	d.Series, d.Number, d.Title = strings.TrimSpace(d.Series), strings.TrimLeft(strings.TrimSpace(d.Number), "#"), strings.TrimSpace(d.Title)
	d.Publisher, d.Language, d.Summary = strings.TrimSpace(d.Publisher), strings.TrimSpace(d.Language), strings.TrimSpace(d.Summary)
	for _, l := range []*[]string{&d.Writers, &d.Pencillers, &d.Inkers, &d.Colorists, &d.Letterers, &d.CoverArtists} {
		var clean []string
		for _, n := range *l {
			if n = strings.TrimSpace(n); n != "" {
				clean = append(clean, n)
			}
		}
		*l = clean
	}
	if d.Series == "" {
		return ErrManualSeries
	}
	if d.Month < 0 || d.Month > 12 || d.Year < 0 || d.Year > 9999 {
		return ErrManualDate
	}
	return i.write(ctx, e, d, ManualScore)
}

// Erros dos metadados manuais.
var (
	ErrManualSeries = errors.New("informe a série")
	ErrManualDate   = errors.New("data inválida (mês de 1 a 12)")
)

// write grava o ComicInfo.xml em cada CBZ da edição e marca os metadados como aplicados.
func (i *Identifier) write(ctx context.Context, e Edition, d metadata.Details, score int) error {
	rows, err := i.DB.QueryContext(ctx, `SELECT l.path, f.rel_path FROM files f JOIN libraries l ON l.id = f.library_id
		WHERE f.edition_id = ? AND f.format = 'cbz'`, e.ID)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var root, rel string
		if err := rows.Scan(&root, &rel); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
	}
	rows.Close()
	ci := comicInfo(e.Guess, d)
	for _, p := range paths {
		if err := convert.WriteComicInfo(p, ci); err != nil {
			return i.setError(ctx, e.ID, "não foi possível gravar o ComicInfo.xml: "+err.Error())
		}
	}
	data, _ := json.Marshal(d)
	if _, err := i.DB.ExecContext(ctx, `UPDATE editions SET meta_status = 'applied', meta_error = '', meta_score = ?,
		meta_json = ?, meta_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, score, string(data), e.ID); err != nil {
		return err
	}
	slog.Info("metadados aplicados", "edição", e.Key, "fonte", d.Provider, "série", d.Series, "número", d.Number, "cbz", len(paths))
	return i.Tasks.ResolveFile(ctx, tasks.KindMetadataConfirm, e.LibraryID, e.Key)
}

func (i *Identifier) setError(ctx context.Context, id int64, msg string) error {
	slog.Warn("identificação com erro", "edição", id, "erro", msg)
	_, err := i.DB.ExecContext(ctx, `UPDATE editions SET meta_status = 'error', meta_error = ?,
		meta_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, msg, id)
	return err
}

// fileID devolve um arquivo da edição (de preferência o CBZ) para o link da tarefa.
func (i *Identifier) fileID(ctx context.Context, editionID int64) (int64, error) {
	var id int64
	err := i.DB.QueryRowContext(ctx, `SELECT id FROM files WHERE edition_id = ? ORDER BY format = 'cbz' DESC, id LIMIT 1`,
		editionID).Scan(&id)
	return id, err
}

// comicInfo monta o ComicInfo.xml a partir dos detalhes da fonte.
func comicInfo(g metadata.Guess, d metadata.Details) convert.ComicInfo {
	join := func(names []string) string { return strings.Join(names, ", ") }
	notes := "Metadados de " + providerName(d.Provider)
	switch d.Provider {
	case "gcd":
		notes += " (comics.org, CC BY-SA 4.0), aplicados pela QuadrinhoTeca."
	case "manual":
		notes = "Metadados preenchidos à mão na QuadrinhoTeca."
	default:
		notes += ", aplicados pela QuadrinhoTeca."
	}
	return convert.ComicInfo{
		Title: d.Title, Series: d.Series, Number: d.Number, Count: g.Count, Summary: d.Summary,
		Year: d.Year, Month: d.Month, Writer: join(d.Writers), Penciller: join(d.Pencillers), Inker: join(d.Inkers),
		Colorist: join(d.Colorists), Letterer: join(d.Letterers), CoverArtist: join(d.CoverArtists),
		Publisher: d.Publisher, Web: d.URL, LanguageISO: d.Language,
		Notes: notes,
	}
}

func describe(g metadata.Guess) string {
	s := g.Series
	if g.Number != "" {
		s += " #" + g.Number
	}
	if g.Year != 0 {
		s += " (" + strconv.Itoa(g.Year) + ")"
	}
	return s
}

func providerName(p string) string {
	switch p {
	case "gcd":
		return "GCD"
	case "comicvine":
		return "ComicVine"
	}
	return p
}
