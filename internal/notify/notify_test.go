package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

// fakeNtfy grava as mensagens recebidas, como um servidor ntfy de teste.
type fakeNtfy struct {
	mu   sync.Mutex
	msgs []Message
	srv  *httptest.Server
}

func newFakeNtfy(t *testing.T, status int) *fakeNtfy {
	f := &fakeNtfy{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m Message
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&m) != nil {
			http.Error(w, "pedido inválido", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.msgs = append(f.msgs, m)
		f.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeNtfy) received() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.msgs...)
}

func newNotifier(t *testing.T, ntfyURL string, cfg func(*settings.Notifications), openTasks int) *Notifier {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	st := settings.NewStore(conn)
	s := settings.Defaults()
	s.Notifications.NtfyURL = ntfyURL
	s.Notifications.NtfyTopic = "quadrinhos"
	if cfg != nil {
		cfg(&s.Notifications)
	}
	if err := st.Put(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return &Notifier{Settings: st, OpenTasks: func(context.Context) (int, error) { return openTasks, nil }}
}

func TestFlushSendsOneSummary(t *testing.T) {
	ntfy := newFakeNtfy(t, http.StatusOK)
	n := newNotifier(t, ntfy.srv.URL, func(c *settings.Notifications) { c.OnFinish, c.OnError, c.OnTasks = true, true, true }, 2)
	for range 12 {
		n.Record(true)
	}
	n.Record(false)
	n.Record(false)

	if err := n.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	msgs := ntfy.received()
	if len(msgs) != 1 {
		t.Fatalf("esperava 1 resumo, vieram %d", len(msgs))
	}
	m := msgs[0]
	want := "12 quadrinhos convertidos · 2 conversões com erro · 2 tarefas aguardando você"
	if m.Topic != "quadrinhos" || m.Message != want || m.Priority != 4 {
		t.Fatalf("mensagem: %+v", m)
	}

	// Sem novidades: não manda nada.
	if err := n.Flush(context.Background()); err != nil || len(ntfy.received()) != 1 {
		t.Fatalf("não deveria reenviar (%v, %d)", err, len(ntfy.received()))
	}
}

func TestFlushRespectsChosenEvents(t *testing.T) {
	ntfy := newFakeNtfy(t, http.StatusOK)
	// Padrão: só erros.
	n := newNotifier(t, ntfy.srv.URL, nil, 0)
	n.Record(true)
	n.Record(true)
	n.Flush(context.Background())
	if len(ntfy.received()) != 0 {
		t.Fatal("só conversões ok não devem avisar quando 'ao terminar' está desligado")
	}
	n.Record(false)
	n.Flush(context.Background())
	if msgs := ntfy.received(); len(msgs) != 1 || msgs[0].Message != "1 conversão com erro" {
		t.Fatalf("aviso de erro: %+v", msgs)
	}
}

func TestFlushWithoutNtfyConfiguredDoesNothing(t *testing.T) {
	n := newNotifier(t, "", func(c *settings.Notifications) { c.NtfyTopic = "" }, 0)
	n.Record(false)
	if err := n.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTestReportsServerErrors(t *testing.T) {
	ok := newFakeNtfy(t, http.StatusOK)
	if err := newNotifier(t, ok.srv.URL, nil, 0).Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m := ok.received(); len(m) != 1 || !strings.Contains(m[0].Message, "funcionando") {
		t.Fatalf("mensagem de teste: %+v", m)
	}

	denied := newFakeNtfy(t, http.StatusForbidden)
	err := newNotifier(t, denied.srv.URL, nil, 0).Test(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("esperava erro 403, veio %v", err)
	}

	if err := newNotifier(t, "http://127.0.0.1:1", nil, 0).Test(context.Background()); err == nil {
		t.Fatal("esperava erro de conexão")
	}
}
