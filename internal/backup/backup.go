// Package backup exporta e restaura o banco da QuadrinhoTeca (catálogo, fila, tarefas e configurações).
//
// Um backup é um .zip com manifest.json e uma cópia consistente do banco feita com VACUUM INTO (não
// precisa parar a app). A restauração usa a API de backup do SQLite para copiar o arquivo enviado
// sobre o banco em uso, sem reiniciar.
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
	"modernc.org/sqlite"
)

const (
	dbEntry       = "quadrinhoteca.db"
	manifestEntry = "manifest.json"
	appName       = "quadrinhoteca"
	// MaxUpload limita o .zip enviado para restaurar.
	MaxUpload = 2 << 30
)

// Nome: quadrinhoteca-AAAAMMDD-HHMMSS-mmm.zip (UTC, com milissegundos).
var nameRe = regexp.MustCompile(`^quadrinhoteca-(\d{8}-\d{6})-(\d{3})\.zip$`)

const nameLayout = "20060102-150405.000"

func backupName(t time.Time) string {
	return "quadrinhoteca-" + strings.Replace(t.UTC().Format(nameLayout), ".", "-", 1) + ".zip"
}

// ErrNotFound indica backup inexistente (ou nome inválido).
var ErrNotFound = errors.New("backup não encontrado")

// ErrBusy indica que algo em andamento (conversões) impede restaurar agora.
var ErrBusy = errors.New("há conversões em andamento; espere terminarem ou cancele-as antes de restaurar")

// ErrInvalid indica arquivo que não é um backup da QuadrinhoTeca utilizável.
var ErrInvalid = errors.New("arquivo de backup inválido")

// Info descreve um backup guardado.
type Info struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

// Manifest vai dentro do .zip.
type Manifest struct {
	App           string    `json:"app"`
	AppVersion    string    `json:"appVersion"`
	SchemaVersion int       `json:"schemaVersion"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Manager cria, lista, poda e restaura backups em Dir.
type Manager struct {
	DB       *sql.DB
	Dir      string
	Settings *settings.Store
	Version  string // versão da app, gravada no manifest
	// CanRestore impede a restauração enquanto algo está em andamento (ex.: conversões).
	CanRestore func(context.Context) error
	// AfterRestore é chamado depois de restaurar (ex.: revarrer as bibliotecas).
	AfterRestore func()

	Now   func() time.Time
	Every time.Duration // intervalo de checagem do backup diário (padrão 1 h)
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// Create gera um backup agora e aplica a retenção configurada.
func (m *Manager) Create(ctx context.Context) (Info, error) {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return Info{}, err
	}
	now := m.now().UTC().Truncate(time.Millisecond)
	// Dois backups no mesmo instante (ex.: backup manual e logo o de segurança da restauração):
	// avança 1 ms até achar um nome livre.
	name := backupName(now)
	for {
		if _, err := os.Stat(filepath.Join(m.Dir, name)); err != nil {
			break
		}
		now = now.Add(time.Millisecond)
		name = backupName(now)
	}
	final := filepath.Join(m.Dir, name)

	snap := filepath.Join(m.Dir, ".snapshot-"+strings.TrimSuffix(name, ".zip")+".db")
	os.Remove(snap)
	defer os.Remove(snap)
	if _, err := m.DB.ExecContext(ctx, "VACUUM INTO ?", snap); err != nil {
		return Info{}, fmt.Errorf("copiando o banco: %w", err)
	}

	tmp := filepath.Join(m.Dir, ".tmp-"+name)
	if err := writeZip(tmp, snap, Manifest{App: appName, AppVersion: m.Version, SchemaVersion: db.LatestVersion(), CreatedAt: now}); err != nil {
		os.Remove(tmp)
		return Info{}, err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return Info{}, err
	}
	if err := m.prune(ctx); err != nil {
		slog.Warn("aplicando retenção dos backups", "err", err)
	}
	fi, err := os.Stat(final)
	if err != nil {
		return Info{}, err
	}
	slog.Info("backup criado", "arquivo", name, "bytes", fi.Size())
	return Info{Name: name, Size: fi.Size(), CreatedAt: now}, nil
}

func writeZip(path, dbFile string, man Manifest) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	w, err := zw.Create(manifestEntry)
	if err == nil {
		err = json.NewEncoder(w).Encode(man)
	}
	if err == nil {
		var in *os.File
		if in, err = os.Open(dbFile); err == nil {
			if w, err = zw.Create(dbEntry); err == nil {
				_, err = io.Copy(w, in)
			}
			in.Close()
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// List devolve os backups, mais recentes primeiro.
func (m *Manager) List() ([]Info, error) {
	entries, err := os.ReadDir(m.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	list := []Info{}
	for _, e := range entries {
		match := nameRe.FindStringSubmatch(e.Name())
		if e.IsDir() || match == nil {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		created, _ := time.Parse(nameLayout, match[1]+"."+match[2])
		list = append(list, Info{Name: e.Name(), Size: fi.Size(), CreatedAt: created.UTC()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name > list[j].Name })
	return list, nil
}

// Path devolve o caminho de um backup pelo nome (recusa nomes fora do padrão).
func (m *Manager) Path(name string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", ErrNotFound
	}
	p := filepath.Join(m.Dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

// prune apaga os backups mais antigos além da quantidade configurada.
func (m *Manager) prune(ctx context.Context) error {
	s, err := m.Settings.Get(ctx)
	if err != nil {
		return err
	}
	list, err := m.List()
	if err != nil {
		return err
	}
	for i := s.Backup.Keep; i < len(list); i++ {
		if err := os.Remove(filepath.Join(m.Dir, list[i].Name)); err != nil {
			return err
		}
	}
	return nil
}

// Run faz o backup diário (se ligado nas configurações) até ctx ser cancelado.
func (m *Manager) Run(ctx context.Context) {
	every := m.Every
	if every == 0 {
		every = time.Hour
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := m.dailyIfDue(ctx); err != nil && ctx.Err() == nil {
			slog.Error("backup diário falhou", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) dailyIfDue(ctx context.Context) error {
	s, err := m.Settings.Get(ctx)
	if err != nil || !s.Backup.Daily {
		return err
	}
	list, err := m.List()
	if err != nil {
		return err
	}
	if len(list) > 0 && m.now().Sub(list[0].CreatedAt) < 24*time.Hour {
		return nil
	}
	_, err = m.Create(ctx)
	return err
}

// Restore substitui o banco em uso pelo do backup enviado. Antes, cria um backup do estado atual
// (para dar para voltar atrás). Devolve o nome desse backup de segurança.
func (m *Manager) Restore(ctx context.Context, upload io.Reader) (string, error) {
	if m.CanRestore != nil {
		if err := m.CanRestore(ctx); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return "", err
	}
	zipPath, err := saveUpload(m.Dir, upload)
	if err != nil {
		return "", err
	}
	defer os.Remove(zipPath)
	dbPath, err := extractDB(zipPath, m.Dir)
	if err != nil {
		return "", err
	}
	defer os.Remove(dbPath)
	if err := checkDB(ctx, dbPath); err != nil {
		return "", err
	}

	safety, err := m.Create(ctx)
	if err != nil {
		return "", fmt.Errorf("criando backup de segurança antes de restaurar: %w", err)
	}

	conn, err := m.DB.Conn(ctx)
	if err != nil {
		return "", err
	}
	err = conn.Raw(func(dc any) error {
		r, ok := dc.(interface {
			NewRestore(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("driver SQLite sem suporte a restauração")
		}
		b, err := r.NewRestore("file:" + dbPath + "?mode=ro")
		if err != nil {
			return err
		}
		for {
			more, err := b.Step(-1)
			if err != nil {
				b.Finish()
				return err
			}
			if !more {
				break
			}
		}
		return b.Finish()
	})
	conn.Close()
	if err != nil {
		return safety.Name, fmt.Errorf("restaurando: %w (o backup de segurança %s tem o estado anterior)", err, safety.Name)
	}
	// Backup de uma versão anterior da app: leva o esquema à versão atual.
	if err := db.Migrate(m.DB); err != nil {
		return safety.Name, fmt.Errorf("atualizando o esquema restaurado: %w", err)
	}
	slog.Info("backup restaurado", "backup_de_seguranca", safety.Name)
	if m.AfterRestore != nil {
		m.AfterRestore()
	}
	return safety.Name, nil
}

func saveUpload(dir string, r io.Reader) (string, error) {
	f, err := os.CreateTemp(dir, ".restore-*.zip")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(r, MaxUpload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxUpload {
		err = fmt.Errorf("%w: maior que %d GB", ErrInvalid, MaxUpload>>30)
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func extractDB(zipPath, dir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("%w: não é um .zip (%v)", ErrInvalid, err)
	}
	defer zr.Close()
	var man Manifest
	var dbFile *zip.File
	for _, f := range zr.File {
		switch f.Name {
		case manifestEntry:
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			err = json.NewDecoder(rc).Decode(&man)
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("%w: manifest.json ilegível", ErrInvalid)
			}
		case dbEntry:
			dbFile = f
		}
	}
	if man.App != appName || dbFile == nil {
		return "", fmt.Errorf("%w: não é um backup da QuadrinhoTeca", ErrInvalid)
	}
	if man.SchemaVersion > db.LatestVersion() {
		return "", fmt.Errorf("%w: feito por uma versão mais nova da app (%s); atualize a app antes", ErrInvalid, man.AppVersion)
	}
	out, err := os.CreateTemp(dir, ".restore-*.db")
	if err != nil {
		return "", err
	}
	rc, err := dbFile.Open()
	if err == nil {
		_, err = io.Copy(out, rc)
		rc.Close()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

// checkDB abre o banco extraído só para leitura e confere integridade e versão.
func checkDB(ctx context.Context, path string) error {
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer conn.Close()
	var ok string
	if err := conn.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok); err != nil || ok != "ok" {
		return fmt.Errorf("%w: banco corrompido (%v %s)", ErrInvalid, err, ok)
	}
	var v int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v < 1 || v > db.LatestVersion() {
		return fmt.Errorf("%w: versão de esquema %d não suportada", ErrInvalid, v)
	}
	return nil
}
