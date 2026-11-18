package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/convert"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

// Converter é a parte do conversor usada pelo runner (substituível nos testes).
type Converter interface {
	Convert(ctx context.Context, src, dst string, progress func(done, total int)) (convert.Result, error)
}

// LibraryPath resolve a pasta raiz de uma biblioteca.
type LibraryPath func(ctx context.Context, libraryID int64) (path string, name string, err error)

// Runner executa a fila com N workers (normal) ou M workers (modo turbo, até um horário).
type Runner struct {
	store    *Store
	settings *settings.Store
	conv     Converter
	libPath  LibraryPath
	// OnFinish é chamado quando um job termina com sucesso ou falha (não em cancelamento nem quando
	// volta para a fila no desligamento). Usado para revarrer a biblioteca, abrir/fechar tarefas e
	// contar as conversões para a notificação.
	OnFinish func(Job)
	// OnIdle é chamado quando a fila esvazia (nada em execução nem aguardando).
	OnIdle func()

	tick time.Duration
	now  func() time.Time

	wake chan struct{}

	mu         sync.Mutex
	running    map[int64]context.CancelFunc
	cancelled  map[int64]bool
	turboUntil time.Time
}

// NewRunner cria o runner.
func NewRunner(store *Store, st *settings.Store, conv Converter, libPath LibraryPath) *Runner {
	return &Runner{
		store: store, settings: st, conv: conv, libPath: libPath,
		tick: time.Minute, now: time.Now,
		wake:      make(chan struct{}, 1),
		running:   map[int64]context.CancelFunc{},
		cancelled: map[int64]bool{},
	}
}

// Wake pede ao runner que olhe a fila agora.
func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run executa até ctx ser cancelado; jobs em andamento voltam para a fila.
func (r *Runner) Run(ctx context.Context) {
	if n, err := r.store.resetRunning(ctx); err != nil {
		slog.Error("restaurando fila", "err", err)
	} else if n > 0 {
		slog.Info("jobs interrompidos voltaram para a fila", "quantidade", n)
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	r.EnqueueEligible(ctx)
	for {
		r.dispatch(ctx, &wg)
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-ticker.C:
			r.EnqueueEligible(ctx) // PDFs que ficaram estáveis desde a última olhada
		}
	}
}

func (r *Runner) limit(ctx context.Context) int {
	s, err := r.settings.Get(ctx)
	if err != nil {
		return 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.now().Before(r.turboUntil) {
		return s.Conversion.TurboWorkers
	}
	return s.Conversion.Workers
}

func (r *Runner) dispatch(ctx context.Context, wg *sync.WaitGroup) {
	for ctx.Err() == nil {
		r.mu.Lock()
		busy := len(r.running)
		r.mu.Unlock()
		if busy >= r.limit(ctx) {
			return
		}
		j, ok, err := r.store.claim(ctx)
		if err != nil {
			slog.Error("pegando próximo job", "err", err)
			return
		}
		if !ok {
			return
		}
		jctx, cancel := context.WithCancel(ctx)
		r.mu.Lock()
		r.running[j.ID] = cancel
		r.mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.execute(jctx, ctx, j)
			cancel()
			r.mu.Lock()
			delete(r.running, j.ID)
			delete(r.cancelled, j.ID)
			idle := len(r.running) == 0
			r.mu.Unlock()
			if idle {
				// O PDF inteiro passa pela memória; devolve-a ao sistema quando não há mais conversões.
				debug.FreeOSMemory()
				if n, err := r.store.activeCount(context.WithoutCancel(ctx)); err == nil && n == 0 && r.OnIdle != nil {
					r.OnIdle()
				}
			}
			r.Wake()
		}()
	}
}

// SetTurbo liga (pelo tempo configurado) ou desliga o modo turbo. Devolve até quando vale.
func (r *Runner) SetTurbo(ctx context.Context, on bool) (time.Time, error) {
	s, err := r.settings.Get(ctx)
	if err != nil {
		return time.Time{}, err
	}
	r.mu.Lock()
	if on {
		r.turboUntil = r.now().Add(time.Duration(s.Conversion.TurboHours) * time.Hour)
	} else {
		r.turboUntil = time.Time{}
	}
	until := r.turboUntil
	r.mu.Unlock()
	r.Wake()
	return until, nil
}

// Turbo devolve até quando o modo turbo vale (zero = desligado).
func (r *Runner) Turbo() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.now().After(r.turboUntil) {
		return time.Time{}
	}
	return r.turboUntil
}

// Cancel cancela um job na fila ou em execução.
func (r *Runner) Cancel(ctx context.Context, id int64) error {
	ok, err := r.store.cancelQueued(ctx, id)
	if err != nil || ok {
		return err
	}
	r.mu.Lock()
	cancel, running := r.running[id]
	if running {
		r.cancelled[id] = true
	}
	r.mu.Unlock()
	if !running {
		return ErrNotFound
	}
	cancel()
	return nil
}

// EnqueueEligible enfileira os PDFs estáveis das bibliotecas com auto-conversão.
func (r *Runner) EnqueueEligible(ctx context.Context) int {
	s, err := r.settings.Get(ctx)
	if err != nil || !s.Conversion.AutoConvert {
		return 0
	}
	stableBefore := r.now().Add(-time.Duration(s.Conversion.StabilityMinutes) * time.Minute).UnixNano()
	return r.enqueue(ctx, 0, true, stableBefore)
}

// EnqueueLibrary enfileira todos os PDFs ainda não convertidos de uma biblioteca (ação manual).
func (r *Runner) EnqueueLibrary(ctx context.Context, libraryID int64) int {
	return r.enqueue(ctx, libraryID, false, 0)
}

// ErrHasCBZ indica que o PDF já tem um CBZ com o mesmo nome ao lado.
var ErrHasCBZ = errors.New("já existe um CBZ com o mesmo nome")

// EnqueueFile enfileira um PDF específico (ação manual na tela do arquivo). Devolve false se o
// arquivo já tem uma conversão ativa.
func (r *Runner) EnqueueFile(ctx context.Context, libraryID int64, absPath, relPath string, size, mtime int64) (bool, error) {
	if _, err := os.Stat(cbzPath(absPath)); err == nil {
		return false, ErrHasCBZ
	}
	ok, err := r.store.Enqueue(ctx, libraryID, relPath, size, mtime, false)
	if ok {
		r.Wake()
	}
	return ok, err
}

func (r *Runner) enqueue(ctx context.Context, libraryID int64, auto bool, stableBefore int64) int {
	cands, err := r.store.candidates(ctx, libraryID, auto, stableBefore)
	if err != nil {
		slog.Error("procurando PDFs para converter", "err", err)
		return 0
	}
	n := 0
	for _, c := range cands {
		if _, err := os.Stat(cbzPath(filepath.Join(c.LibraryPath, filepath.FromSlash(c.RelPath)))); err == nil {
			continue // já existe um CBZ com o mesmo nome
		}
		if ok, err := r.store.Enqueue(ctx, c.LibraryID, c.RelPath, c.Size, c.Mtime, auto); err != nil {
			slog.Error("enfileirando", "arquivo", c.RelPath, "err", err)
		} else if ok {
			n++
		}
	}
	if n > 0 {
		slog.Info("conversões enfileiradas", "quantidade", n, "automático", auto)
		r.Wake()
	}
	return n
}

func cbzPath(pdf string) string {
	return strings.TrimSuffix(pdf, filepath.Ext(pdf)) + ".cbz"
}

// execute roda um job. jctx é cancelado por Cancel ou pelo desligamento (appCtx).
func (r *Runner) execute(jctx, appCtx context.Context, j Job) {
	// Gravações finais usam um contexto próprio para sobreviver ao cancelamento do job.
	bg := context.WithoutCancel(jctx)
	fail := func(msg string) {
		// Cancelado pelo usuário ou desligamento antes de o conversor rodar não é erro.
		r.mu.Lock()
		userCancelled := r.cancelled[j.ID]
		r.mu.Unlock()
		if userCancelled {
			j.Status, j.Error = Cancelled, "cancelado"
			_ = r.store.finish(bg, j)
			return
		}
		if appCtx.Err() != nil {
			_ = r.store.requeue(bg, j.ID)
			return
		}
		j.Status, j.Error = Failed, msg
		if err := r.store.finish(bg, j); err != nil {
			slog.Error("gravando job", "id", j.ID, "err", err)
		}
		slog.Warn("conversão falhou", "arquivo", j.RelPath, "erro", msg)
		if r.OnFinish != nil {
			r.OnFinish(j)
		}
	}

	root, libName, err := r.libPath(jctx, j.LibraryID)
	if err != nil {
		fail("biblioteca indisponível: " + err.Error())
		return
	}
	src := filepath.Join(root, filepath.FromSlash(j.RelPath))
	fi, err := os.Stat(src)
	if err != nil {
		fail("o PDF não existe mais")
		return
	}
	if fi.Size() != j.SrcSize || fi.ModTime().UnixNano() != j.SrcMtime {
		if j.Auto {
			// Ainda sendo copiado/alterado: cancela; volta como candidato quando ficar estável.
			j.Status, j.Error = Cancelled, "o arquivo mudou desde que entrou na fila"
			_ = r.store.finish(bg, j)
			return
		}
		j.SrcSize, j.SrcMtime = fi.Size(), fi.ModTime().UnixNano()
	}

	dst := cbzPath(src)
	last := time.Time{}
	res, err := r.safeConvert(jctx, src, dst, func(done, total int) {
		if time.Since(last) > 500*time.Millisecond || done == total {
			last = time.Now()
			_ = r.store.setProgress(bg, j.ID, done*100/total)
		}
	})
	if err != nil {
		r.mu.Lock()
		userCancelled := r.cancelled[j.ID]
		r.mu.Unlock()
		switch {
		case userCancelled:
			j.Status, j.Error = Cancelled, "cancelado"
			_ = r.store.finish(bg, j)
		case appCtx.Err() != nil:
			_ = r.store.requeue(bg, j.ID) // desligando: retoma na próxima partida
		case errors.Is(err, convert.ErrExists):
			fail("já existe um CBZ com esse nome")
		default:
			fail(err.Error())
		}
		return
	}

	j.Status, j.Progress = Done, 100
	j.Pages, j.Extracted, j.Rendered = res.Pages, res.Extracted, res.Rendered
	j.OutputRelPath = strings.TrimSuffix(j.RelPath, path.Ext(j.RelPath)) + ".cbz"
	if err := r.handleOriginal(bg, src, libName, j.RelPath); err != nil {
		j.Error = "CBZ criado, mas o PDF original não foi tratado: " + err.Error()
	}
	if err := r.store.finish(bg, j); err != nil {
		slog.Error("gravando job", "id", j.ID, "err", err)
	}
	slog.Info("conversão concluída", "arquivo", j.RelPath, "páginas", res.Pages,
		"extraídas", res.Extracted, "renderizadas", res.Rendered)
	if r.OnFinish != nil {
		r.OnFinish(j)
	}
}

// safeConvert transforma um panic do conversor (PDF malformado, bug de biblioteca) em erro do job,
// para não derrubar a aplicação inteira.
func (r *Runner) safeConvert(ctx context.Context, src, dst string, progress func(int, int)) (res convert.Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("panic na conversão", "arquivo", src, "panic", p, "stack", string(debug.Stack()))
			os.Remove(convert.TempPath(dst))
			err = fmt.Errorf("erro interno ao converter: %v", p)
		}
	}()
	return r.conv.Convert(ctx, src, dst, progress)
}

// handleOriginal aplica a ação configurada ao PDF depois que o CBZ foi validado.
func (r *Runner) handleOriginal(ctx context.Context, src, libName, relPath string) error {
	s, err := r.settings.Get(ctx)
	if err != nil {
		return err
	}
	switch s.Conversion.OriginalAction {
	case "delete":
		return os.Remove(src)
	case "archive":
		dst := filepath.Join(s.Conversion.ArchiveDir, safeName(libName), filepath.FromSlash(relPath))
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("já existe %s na pasta de arquivo", dst)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return moveFile(src, dst)
	default: // keep
		return nil
	}
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
	if s == "" || s == "." || s == ".." {
		return "biblioteca"
	}
	return s
}

// moveFile renomeia; entre discos diferentes, copia, confere o tamanho e só então apaga a origem.
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".qt-tmp")
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	fi, serr := in.Stat()
	if err == nil && serr == nil && n != fi.Size() {
		err = fmt.Errorf("cópia incompleta (%d de %d bytes)", n, fi.Size())
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	in.Close()
	return os.Remove(src)
}
