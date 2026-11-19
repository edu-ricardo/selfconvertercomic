package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Tipos de falha de um provedor, para a UI explicar o que fazer.
var (
	ErrUnauthorized = errors.New("chave de API inválida")
	ErrRateLimited  = errors.New("limite de pedidos do serviço atingido; tente mais tarde")
	ErrUnavailable  = errors.New("serviço fora do ar ou inacessível")
)

// ProviderError diz qual provedor falhou e por quê (Unwrap devolve um dos Err* acima).
type ProviderError struct {
	Provider string
	Kind     error
	Detail   string
}

func (e *ProviderError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %v", e.Provider, e.Kind)
	}
	return fmt.Sprintf("%s: %v (%s)", e.Provider, e.Kind, e.Detail)
}

func (e *ProviderError) Unwrap() error { return e.Kind }

// Limits controla o ritmo e o cache de um pedido.
type Limits struct {
	Interval time.Duration // espera mínima entre pedidos ao mesmo provedor
	TTL      time.Duration // validade do cache
}

// Fetcher faz os pedidos de UM provedor: um de cada vez, com intervalo mínimo entre eles, e guarda as
// respostas boas no SQLite.
type Fetcher struct {
	Provider  string
	DB        *sql.DB
	Client    *http.Client
	UserAgent string
	Now       func() time.Time

	mu   sync.Mutex
	last time.Time
}

// MaxBody limita o tamanho de uma resposta.
const MaxBody = 4 << 20

// Get devolve o corpo da resposta de url, do cache quando houver. cacheKey identifica o pedido sem
// segredos (a chave de API não entra). check valida o corpo de uma resposta 200 (ex.: o status_code
// do ComicVine); só respostas aceitas vão para o cache.
func (f *Fetcher) Get(ctx context.Context, cacheKey, url string, lim Limits, check func([]byte) error) ([]byte, error) {
	key := f.Provider + " " + cacheKey
	var body []byte
	if lim.TTL > 0 {
		err := f.DB.QueryRowContext(ctx, "SELECT body FROM metadata_cache WHERE key = ? AND fetched_at > ?",
			key, f.now().Add(-lim.TTL).Unix()).Scan(&body)
		if err == nil {
			return body, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	body, err := f.fetch(ctx, url, lim.Interval)
	if err != nil {
		return nil, err
	}
	if check != nil {
		if err := check(body); err != nil {
			return nil, err
		}
	}
	if lim.TTL <= 0 {
		return body, nil // sem cache (ex.: testar a chave)
	}
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO metadata_cache (key, body, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at`,
		key, body, f.now().Unix()); err != nil {
		return nil, err
	}
	_, _ = f.DB.ExecContext(ctx, "DELETE FROM metadata_cache WHERE fetched_at <= ?", f.now().Add(-lim.TTL).Unix())
	return body, nil
}

func (f *Fetcher) fetch(ctx context.Context, url string, interval time.Duration) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if wait := interval - f.now().Sub(f.last); wait > 0 && !f.last.IsZero() {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() { f.last = f.now() }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := f.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ProviderError{f.Provider, ErrUnavailable, err.Error()}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBody))
	if err != nil {
		return nil, &ProviderError{f.Provider, ErrUnavailable, err.Error()}
	}
	switch {
	case res.StatusCode == http.StatusOK:
		return body, nil
	case res.StatusCode == http.StatusUnauthorized:
		return nil, &ProviderError{f.Provider, ErrUnauthorized, ""}
	case res.StatusCode == 420 || res.StatusCode == http.StatusTooManyRequests:
		return nil, &ProviderError{f.Provider, ErrRateLimited, ""}
	case res.StatusCode == http.StatusNotFound:
		return nil, errNotFound
	default:
		return nil, &ProviderError{f.Provider, ErrUnavailable, res.Status}
	}
}

// errNotFound: o recurso não existe (ex.: página além da última); não é falha do provedor.
var errNotFound = errors.New("não encontrado")

func (f *Fetcher) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}
