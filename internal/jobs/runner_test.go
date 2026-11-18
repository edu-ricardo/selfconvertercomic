package jobs

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/convert"
	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

type env struct {
	t        *testing.T
	root     string
	libs     *library.Store
	lib      library.Library
	store    *Store
	settings *settings.Store
	now      time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	e := &env{t: t, root: t.TempDir(), libs: library.NewStore(conn), store: NewStore(conn),
		settings: settings.NewStore(conn), now: time.Now()}
	e.lib, err = e.libs.Create(context.Background(), "HD", e.root, true)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) configure(f func(*settings.Settings)) {
	s := settings.Defaults()
	f(&s)
	if err := e.settings.Put(context.Background(), s); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) scan() {
	if _, err := e.libs.Scan(context.Background(), e.lib); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) runner(conv Converter) *Runner {
	r := NewRunner(e.store, e.settings, conv, func(ctx context.Context, id int64) (string, string, error) {
		l, err := e.libs.Get(ctx, id)
		return l.Path, l.Name, err
	})
	r.tick = 50 * time.Millisecond
	r.now = func() time.Time { return e.now }
	return r
}

// pdf grava um PDF real (uma imagem JPEG por página) com mtime antigo (já "estável").
func (e *env) pdf(rel string, pages int) string {
	e.t.Helper()
	var readers []io.Reader
	for i := range pages {
		img := image.NewRGBA(image.Rect(0, 0, 60, 90))
		img.Set(0, 0, color.RGBA{uint8(i), 0, 0, 255})
		var b bytes.Buffer
		jpeg.Encode(&b, img, nil)
		readers = append(readers, &b)
	}
	var out bytes.Buffer
	if err := api.ImportImages(context.Background(), nil, &out, readers, pdfcpu.DefaultImportConfig(), nil); err != nil {
		e.t.Fatal(err)
	}
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, out.Bytes(), 0o644); err != nil {
		e.t.Fatal(err)
	}
	old := e.now.Add(-time.Hour)
	os.Chtimes(p, old, old)
	return p
}

func (e *env) waitJobs(what string, cond func(map[string]int) bool) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c, _ := e.store.Counts(context.Background())
		if cond(c) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	c, _ := e.store.Counts(context.Background())
	e.t.Fatalf("tempo esgotado esperando %s; contagem: %v", what, c)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestRunnerConvertsAndAppliesOriginalAction(t *testing.T) {
	for _, action := range []string{"keep", "delete", "archive"} {
		t.Run(action, func(t *testing.T) {
			e := newEnv(t)
			archive := t.TempDir()
			e.configure(func(s *settings.Settings) {
				s.Conversion.OriginalAction = action
				s.Conversion.ArchiveDir = archive
			})
			src := e.pdf("Marvel/Guerras/Capítulo 01.pdf", 3)
			e.scan()

			var done atomic.Int64
			r := e.runner(convert.Converter{})
			r.OnDone = func(int64) { done.Add(1) }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go r.Run(ctx)

			e.waitJobs("conversão", func(c map[string]int) bool { return c[Done] == 1 })
			cbz := filepath.Join(e.root, "Marvel", "Guerras", "Capítulo 01.cbz")
			if err := convert.Validate(cbz, 3); err != nil {
				t.Fatal(err)
			}
			jobs, _ := e.store.List(ctx, []string{Done}, 10)
			if jobs[0].OutputRelPath != "Marvel/Guerras/Capítulo 01.cbz" || jobs[0].Extracted != 3 || jobs[0].Error != "" {
				t.Fatalf("job: %+v", jobs[0])
			}
			if done.Load() != 1 {
				t.Fatal("OnDone não foi chamado")
			}
			archived := filepath.Join(archive, "HD", "Marvel", "Guerras", "Capítulo 01.pdf")
			switch action {
			case "keep":
				if !exists(src) {
					t.Fatal("PDF deveria ter ficado no lugar")
				}
			case "delete":
				if exists(src) {
					t.Fatal("PDF deveria ter sido apagado")
				}
			case "archive":
				if exists(src) || !exists(archived) {
					t.Fatal("PDF deveria estar na pasta de arquivo")
				}
			}

			// Não converte de novo o mesmo arquivo.
			if n := r.EnqueueEligible(ctx); n != 0 {
				t.Fatalf("enfileirou %d de novo", n)
			}
		})
	}
}

func TestStabilityAndAutoFlag(t *testing.T) {
	e := newEnv(t)
	e.configure(func(s *settings.Settings) { s.Conversion.StabilityMinutes = 2 })
	p := e.pdf("novo.pdf", 1)
	recent := e.now.Add(-30 * time.Second) // ainda "sendo copiado"
	os.Chtimes(p, recent, recent)
	e.scan()
	r := e.runner(convert.Converter{})
	ctx := context.Background()

	if n := r.EnqueueEligible(ctx); n != 0 {
		t.Fatalf("arquivo instável enfileirado (%d)", n)
	}
	e.now = e.now.Add(3 * time.Minute)
	if _, err := e.libs.Update(ctx, e.lib.ID, "HD", false); err != nil {
		t.Fatal(err)
	}
	if n := r.EnqueueEligible(ctx); n != 0 {
		t.Fatal("biblioteca sem auto-conversão não deveria enfileirar")
	}
	if n := r.EnqueueLibrary(ctx, e.lib.ID); n != 1 {
		t.Fatalf("conversão manual da biblioteca: %d", n)
	}
	if n := r.EnqueueLibrary(ctx, e.lib.ID); n != 0 {
		t.Fatal("não pode haver dois jobs ativos para o mesmo arquivo")
	}
}

func TestSkipsWhenCBZAlreadyExists(t *testing.T) {
	e := newEnv(t)
	e.pdf("a.pdf", 1)
	os.WriteFile(filepath.Join(e.root, "a.cbz"), []byte("x"), 0o644)
	e.scan()
	if n := e.runner(convert.Converter{}).EnqueueEligible(context.Background()); n != 0 {
		t.Fatalf("enfileirou %d com CBZ já existente", n)
	}
}

// blockingConv simula uma conversão longa: espera o contexto e conta execuções simultâneas.
type blockingConv struct {
	mu      sync.Mutex
	current int
	peak    int
	started chan struct{}
}

func (b *blockingConv) Convert(ctx context.Context, src, dst string, _ func(int, int)) (convert.Result, error) {
	b.mu.Lock()
	b.current++
	b.peak = max(b.peak, b.current)
	b.mu.Unlock()
	b.started <- struct{}{}
	<-ctx.Done()
	b.mu.Lock()
	b.current--
	b.mu.Unlock()
	return convert.Result{}, ctx.Err()
}

func TestShutdownRequeuesAndRestartResumes(t *testing.T) {
	e := newEnv(t)
	e.pdf("a.pdf", 2)
	e.scan()

	conv := &blockingConv{started: make(chan struct{}, 10)}
	ctx, cancel := context.WithCancel(context.Background())
	r := e.runner(conv)
	stopped := make(chan struct{})
	go func() { r.Run(ctx); close(stopped) }()
	<-conv.started
	cancel() // "queda"/desligamento no meio
	<-stopped

	c, _ := e.store.Counts(context.Background())
	if c[Queued] != 1 || c[Running] != 0 {
		t.Fatalf("job deveria voltar para a fila: %v", c)
	}

	// Simula uma queda dura (processo morto com o job marcado como running).
	if _, err := e.store.db.Exec("UPDATE jobs SET status = 'running'"); err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go e.runner(convert.Converter{}).Run(ctx2)
	e.waitJobs("retomada após queda", func(c map[string]int) bool { return c[Done] == 1 })
	if err := convert.Validate(filepath.Join(e.root, "a.cbz"), 2); err != nil {
		t.Fatal(err)
	}
}

func TestCancelAndTurbo(t *testing.T) {
	e := newEnv(t)
	e.configure(func(s *settings.Settings) { s.Conversion.Workers = 1; s.Conversion.TurboWorkers = 3 })
	for _, n := range []string{"a.pdf", "b.pdf", "c.pdf", "d.pdf"} {
		e.pdf(n, 1)
	}
	e.scan()

	conv := &blockingConv{started: make(chan struct{}, 10)}
	r := e.runner(conv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	<-conv.started
	time.Sleep(100 * time.Millisecond)
	if conv.peak != 1 {
		t.Fatalf("modo normal: %d simultâneas, quer 1", conv.peak)
	}

	until, err := r.SetTurbo(ctx, true)
	if err != nil || until.Sub(e.now) != 4*time.Hour {
		t.Fatalf("turbo até %v (%v)", until, err)
	}
	<-conv.started
	<-conv.started
	time.Sleep(100 * time.Millisecond)
	if conv.peak != 3 {
		t.Fatalf("modo turbo: %d simultâneas, quer 3", conv.peak)
	}

	// Cancela um em execução e um na fila.
	running, _ := e.store.List(ctx, []string{Running}, 10)
	queued, _ := e.store.List(ctx, []string{Queued}, 10)
	if err := r.Cancel(ctx, running[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Cancel(ctx, queued[0].ID); err != nil {
		t.Fatal(err)
	}
	e.waitJobs("cancelamentos", func(c map[string]int) bool { return c[Cancelled] == 2 })

	// Turbo expira sozinho.
	e.now = e.now.Add(5 * time.Hour)
	if !r.Turbo().IsZero() {
		t.Fatal("turbo deveria ter expirado")
	}

	if err := r.Cancel(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelar inexistente: %v", err)
	}
}

type panicConv struct{}

func (panicConv) Convert(context.Context, string, string, func(int, int)) (convert.Result, error) {
	panic("PDF malformado")
}

func TestPanicInConverterFailsJobOnly(t *testing.T) {
	e := newEnv(t)
	e.pdf("ruim.pdf", 1)
	e.scan()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.runner(panicConv{}).Run(ctx)
	e.waitJobs("job com erro", func(c map[string]int) bool { return c[Failed] == 1 })
	jobs, _ := e.store.List(ctx, []string{Failed}, 1)
	if !strings.Contains(jobs[0].Error, "PDF malformado") {
		t.Fatalf("erro: %q", jobs[0].Error)
	}
}
