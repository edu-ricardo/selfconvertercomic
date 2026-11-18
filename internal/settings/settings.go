// Package settings guarda as configurações da aplicação (documento JSON único no SQLite).
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Settings são as configurações editáveis pela UI. Os padrões estão em Defaults.
type Settings struct {
	Conversion    Conversion    `json:"conversion"`
	Scan          Scan          `json:"scan"`
	Notifications Notifications `json:"notifications"`
	Readers       Readers       `json:"readers"`
	Metadata      Metadata      `json:"metadata"`
	Backup        Backup        `json:"backup"`
}

type Conversion struct {
	AutoConvert      bool   `json:"autoConvert"`
	StabilityMinutes int    `json:"stabilityMinutes"`
	OriginalAction   string `json:"originalAction"` // keep | archive | delete
	ArchiveDir       string `json:"archiveDir"`     // usado quando OriginalAction = archive
	Workers          int    `json:"workers"`
	TurboWorkers     int    `json:"turboWorkers"`
	TurboHours       int    `json:"turboHours"`
}

type Scan struct {
	RescanHours int `json:"rescanHours"`
}

type Notifications struct {
	NtfyURL   string `json:"ntfyUrl"`
	NtfyTopic string `json:"ntfyTopic"`
	OnFinish  bool   `json:"onFinish"`
	OnError   bool   `json:"onError"`
	OnTasks   bool   `json:"onTasks"`
}

type Readers struct {
	KavitaURL    string `json:"kavitaUrl"`
	BookOrbitURL string `json:"bookOrbitUrl"`
}

type Metadata struct {
	ComicVineAPIKey string `json:"comicVineApiKey"`
	GuiaRateSeconds int    `json:"guiaRateSeconds"`
	CacheDays       int    `json:"cacheDays"`
}

type Backup struct {
	Daily bool `json:"daily"`
	Keep  int  `json:"keep"`
}

// Defaults são os valores decididos em docs/especificacao.md.
func Defaults() Settings {
	return Settings{
		Conversion: Conversion{
			AutoConvert:      true,
			StabilityMinutes: 2,
			OriginalAction:   "keep",
			Workers:          1,
			TurboWorkers:     4,
			TurboHours:       4,
		},
		Scan:          Scan{RescanHours: 6},
		Notifications: Notifications{OnError: true},
		Metadata:      Metadata{GuiaRateSeconds: 2, CacheDays: 30},
		Backup:        Backup{Daily: true, Keep: 7},
	}
}

// ValidationError lista os campos inválidos (mensagens em pt-BR para a UI).
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return "configurações inválidas: " + strings.Join(parts, "; ")
}

// Validate confere limites e combinações.
func (s Settings) Validate() error {
	f := map[string]string{}
	between := func(name string, v, min, max int) {
		if v < min || v > max {
			f[name] = fmt.Sprintf("deve estar entre %d e %d", min, max)
		}
	}
	between("conversion.stabilityMinutes", s.Conversion.StabilityMinutes, 0, 1440)
	between("conversion.workers", s.Conversion.Workers, 1, 16)
	between("conversion.turboWorkers", s.Conversion.TurboWorkers, 1, 16)
	between("conversion.turboHours", s.Conversion.TurboHours, 1, 72)
	between("scan.rescanHours", s.Scan.RescanHours, 1, 168)
	between("metadata.guiaRateSeconds", s.Metadata.GuiaRateSeconds, 1, 60)
	between("metadata.cacheDays", s.Metadata.CacheDays, 1, 365)
	between("backup.keep", s.Backup.Keep, 1, 365)

	switch s.Conversion.OriginalAction {
	case "keep", "delete":
	case "archive":
		if !filepath.IsAbs(s.Conversion.ArchiveDir) {
			f["conversion.archiveDir"] = "informe um caminho absoluto"
		}
	default:
		f["conversion.originalAction"] = "use keep, archive ou delete"
	}

	for name, u := range map[string]string{
		"notifications.ntfyUrl": s.Notifications.NtfyURL,
		"readers.kavitaUrl":     s.Readers.KavitaURL,
		"readers.bookOrbitUrl":  s.Readers.BookOrbitURL,
	} {
		if u == "" {
			continue
		}
		p, err := url.Parse(u)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			f[name] = "URL inválida (use http:// ou https://)"
		}
	}
	if s.Notifications.NtfyURL != "" && s.Notifications.NtfyTopic == "" {
		f["notifications.ntfyTopic"] = "obrigatório quando o ntfy está configurado"
	}

	if len(f) > 0 {
		return &ValidationError{Fields: f}
	}
	return nil
}

// Store lê e grava as configurações no banco.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Get devolve as configurações salvas sobre os padrões (campos novos recebem o padrão).
func (st *Store) Get(ctx context.Context) (Settings, error) {
	s := Defaults()
	var data string
	err := st.db.QueryRowContext(ctx, "SELECT data FROM settings WHERE id = 1").Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return s, fmt.Errorf("configurações corrompidas no banco: %w", err)
	}
	return s, nil
}

// Put valida e grava as configurações.
func (st *Store) Put(ctx context.Context, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = st.db.ExecContext(ctx, `
		INSERT INTO settings (id, data) VALUES (1, ?)
		ON CONFLICT (id) DO UPDATE SET data = excluded.data,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`, string(data))
	return err
}
