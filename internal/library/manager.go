package library

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Manager agenda as varreduras: na partida, sob demanda, por eventos do sistema de arquivos
// (com espera para juntar rajadas), na revarredura periódica e quando um disco volta.
// As varreduras rodam uma de cada vez.
type Manager struct {
	store       *Store
	rescanEvery func() time.Duration
	debounce    time.Duration
	tick        time.Duration

	queue chan int64

	mu         sync.Mutex
	pending    map[int64]bool
	scanning   int64
	timers     map[int64]*time.Timer
	watched    map[string]int64 // pasta → biblioteca
	watchError string

	watcher *fsnotify.Watcher
	// OnScan é chamado após cada varredura bem-sucedida (o M3 usa para enfileirar conversões).
	OnScan func(Library, Result)
}

// NewManager cria o gerenciador. rescanEvery é lido a cada ciclo (vem das configurações).
func NewManager(store *Store, rescanEvery func() time.Duration) *Manager {
	return &Manager{
		store:       store,
		rescanEvery: rescanEvery,
		debounce:    3 * time.Second,
		tick:        time.Minute,
		queue:       make(chan int64, 1024),
		pending:     map[int64]bool{},
		timers:      map[int64]*time.Timer{},
		watched:     map[string]int64{},
	}
}

// Run executa até ctx ser cancelado.
func (m *Manager) Run(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		m.setWatchError("observador de arquivos indisponível: " + err.Error())
	} else {
		m.watcher = w
		defer w.Close()
		go m.watchLoop(ctx)
	}

	m.enqueueAll(ctx)
	ticker := time.NewTicker(m.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-m.queue:
			m.scan(ctx, id)
		case <-ticker.C:
			m.periodic(ctx)
		}
	}
}

// Enqueue pede a varredura de uma biblioteca (pedidos repetidos se juntam).
func (m *Manager) Enqueue(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending[id] {
		return
	}
	select {
	case m.queue <- id:
		m.pending[id] = true
	default: // fila cheia: a revarredura periódica cobre
	}
}

// Status devolve as bibliotecas pendentes/em varredura e o erro do observador (se houver).
func (m *Manager) Status() (busy map[int64]bool, watchError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	busy = map[int64]bool{}
	for id := range m.pending {
		busy[id] = true
	}
	if m.scanning != 0 {
		busy[m.scanning] = true
	}
	return busy, m.watchError
}

// Forget para de observar as pastas de uma biblioteca removida.
func (m *Manager) Forget(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for dir, lib := range m.watched {
		if lib == id {
			if m.watcher != nil {
				_ = m.watcher.Remove(dir)
			}
			delete(m.watched, dir)
		}
	}
}

func (m *Manager) enqueueAll(ctx context.Context) {
	libs, err := m.store.List(ctx)
	if err != nil {
		slog.Error("listando bibliotecas", "err", err)
		return
	}
	for _, l := range libs {
		m.Enqueue(l.ID)
	}
}

// periodic revarre bibliotecas vencidas e tenta de novo as indisponíveis (disco reconectado).
func (m *Manager) periodic(ctx context.Context) {
	libs, err := m.store.List(ctx)
	if err != nil {
		return
	}
	every := m.rescanEvery()
	for _, l := range libs {
		due := !l.Available || l.LastScanAt == nil
		if !due {
			if t, err := time.Parse(time.RFC3339Nano, *l.LastScanAt); err != nil || time.Since(t) >= every {
				due = true
			}
		}
		if due {
			m.Enqueue(l.ID)
		}
	}
}

func (m *Manager) scan(ctx context.Context, id int64) {
	m.mu.Lock()
	delete(m.pending, id)
	m.scanning = id
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.scanning = 0
		m.mu.Unlock()
	}()

	lib, err := m.store.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			slog.Error("lendo biblioteca", "id", id, "err", err)
		}
		return
	}
	res, err := m.store.Scan(ctx, lib)
	if errors.Is(err, ErrUnavailable) {
		// O disco sumiu: as observações caíram junto. Esquece-as para refazê-las quando ele voltar.
		m.Forget(id)
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("varredura falhou", "biblioteca", lib.Name, "err", err)
		}
		return
	}
	slog.Info("varredura concluída", "biblioteca", lib.Name, "total", res.Total,
		"novos", res.Added, "alterados", res.Updated, "removidos", res.Removed, "duração", res.Duration)
	m.watchDirs(id, res.Dirs)
	if m.OnScan != nil {
		m.OnScan(lib, res)
	}
}

func (m *Manager) watchDirs(id int64, dirs []string) {
	if m.watcher == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range dirs {
		if _, ok := m.watched[d]; ok {
			continue
		}
		if err := m.watcher.Add(d); err != nil {
			// Ex.: limite de inotify (ENOSPC). A revarredura periódica continua valendo.
			m.watchError = "nem todas as pastas são observadas (" + err.Error() + "); mudanças podem demorar até a próxima revarredura"
			return
		}
		m.watched[d] = id
	}
}

func (m *Manager) setWatchError(msg string) {
	m.mu.Lock()
	m.watchError = msg
	m.mu.Unlock()
	slog.Warn(msg)
}

func (m *Manager) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if id := m.libraryFor(ev.Name); id != 0 {
				m.debounced(id)
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("observador de arquivos", "err", err)
		}
	}
}

func (m *Manager) libraryFor(path string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.watched[filepath.Dir(path)]; ok {
		return id
	}
	if id, ok := m.watched[path]; ok {
		return id
	}
	// Pasta nova ainda não observada: procura a pasta observada mais próxima.
	best, bestLen := int64(0), 0
	for dir, id := range m.watched {
		if strings.HasPrefix(path, dir+string(filepath.Separator)) && len(dir) > bestLen {
			best, bestLen = id, len(dir)
		}
	}
	return best
}

// debounced junta rajadas de eventos (cópia de muitos arquivos) numa só varredura.
func (m *Manager) debounced(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[id]; ok {
		t.Reset(m.debounce)
		return
	}
	m.timers[id] = time.AfterFunc(m.debounce, func() {
		m.mu.Lock()
		delete(m.timers, id)
		m.mu.Unlock()
		m.Enqueue(id)
	})
}
