// Package notify envia avisos pelo ntfy (https://ntfy.sh, também self-hosted no Umbrel).
//
// Para não inundar o celular, as conversões não geram um aviso cada: o Notifier conta as concluídas
// e as que falharam e manda um resumo único quando a fila esvazia.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

// Message é um aviso no formato JSON de publicação do ntfy (aceita UTF-8 no título e no texto).
type Message struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority,omitempty"` // 1 (mín.) a 5 (máx.); 3 é o padrão
	Tags     []string `json:"tags,omitempty"`
}

// Notifier envia avisos conforme as configurações.
type Notifier struct {
	Settings  *settings.Store
	OpenTasks func(context.Context) (int, error)
	Client    *http.Client

	mu             sync.Mutex
	done, failed   int
	lastTasksCount int
}

// Record conta uma conversão concluída (ok=true) ou com falha, para o próximo resumo.
func (n *Notifier) Record(ok bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ok {
		n.done++
	} else {
		n.failed++
	}
}

// Flush manda o resumo do que aconteceu desde o último (chamado quando a fila esvazia).
// Não manda nada se o ntfy não está configurado ou se nenhum evento escolhido aconteceu.
func (n *Notifier) Flush(ctx context.Context) error {
	n.mu.Lock()
	done, failed := n.done, n.failed
	n.done, n.failed = 0, 0
	n.mu.Unlock()

	s, err := n.Settings.Get(ctx)
	if err != nil {
		return err
	}
	cfg := s.Notifications
	if cfg.NtfyURL == "" {
		return nil
	}

	var parts []string
	priority := 3
	tags := []string{"books"}
	if cfg.OnFinish && done > 0 {
		parts = append(parts, plural(done, "quadrinho convertido", "quadrinhos convertidos"))
	}
	if cfg.OnError && failed > 0 {
		parts = append(parts, plural(failed, "conversão com erro", "conversões com erro"))
		priority = 4
		tags = []string{"warning"}
	}
	if cfg.OnTasks && n.OpenTasks != nil {
		if open, err := n.OpenTasks(ctx); err == nil && open > 0 && (failed > 0 || open != n.lastTasks()) {
			parts = append(parts, plural(open, "tarefa aguardando você", "tarefas aguardando você"))
			n.setLastTasks(open)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return n.Send(ctx, cfg, Message{
		Title:    "QuadrinhoTeca",
		Message:  strings.Join(parts, " · "),
		Priority: priority,
		Tags:     tags,
	})
}

func (n *Notifier) lastTasks() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastTasksCount
}

func (n *Notifier) setLastTasks(v int) {
	n.mu.Lock()
	n.lastTasksCount = v
	n.mu.Unlock()
}

// Test manda um aviso de teste com as configurações atuais (botão "Enviar teste").
func (n *Notifier) Test(ctx context.Context) error {
	s, err := n.Settings.Get(ctx)
	if err != nil {
		return err
	}
	if s.Notifications.NtfyURL == "" || s.Notifications.NtfyTopic == "" {
		return errors.New("configure a URL do servidor e o tópico do ntfy")
	}
	return n.Send(ctx, s.Notifications, Message{
		Title:   "QuadrinhoTeca",
		Message: "Notificações funcionando 👍",
		Tags:    []string{"white_check_mark"},
	})
}

// Send publica msg no servidor/tópico configurados.
func (n *Notifier) Send(ctx context.Context, cfg settings.Notifications, msg Message) error {
	msg.Topic = cfg.NtfyTopic
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.NtfyURL, "/"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível falar com o ntfy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("o ntfy respondeu %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
