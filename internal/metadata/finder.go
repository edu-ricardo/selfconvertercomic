package metadata

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/settings"
)

// seriesToOpen é quantas séries, das mais parecidas, têm a edição buscada (cada uma custa um pedido).
const seriesToOpen = 3

// bestIssues ordena as séries pela semelhança com g e, nas seriesToOpen melhores, busca a edição de
// número g.Number com issue. Sem número, devolve só as séries (candidatos sem IssueID).
func bestIssues(ctx context.Context, g Guess, series []Candidate,
	issue func(context.Context, Candidate) (*Candidate, error)) ([]Candidate, error) {
	for i := range series {
		series[i].Score = Score(g, series[i])
	}
	sort.SliceStable(series, func(i, j int) bool { return series[i].Score > series[j].Score })
	if g.Number == "" {
		return series[:min(len(series), seriesToOpen)], nil
	}
	var out []Candidate
	for _, s := range series[:min(len(series), seriesToOpen)] {
		c, err := issue(ctx, s)
		if err != nil {
			return out, err
		}
		if c != nil {
			c.Score = Score(g, *c)
			out = append(out, *c)
		}
	}
	return out, nil
}

// Finder consulta os provedores ligados nas configurações.
type Finder struct {
	ComicVine *ComicVine
	GCD       *GCD
	Settings  func(context.Context) (settings.Settings, error)
}

// Result junta os candidatos de todos os provedores (melhores primeiro) e as falhas de cada um.
type Result struct {
	Candidates []Candidate `json:"candidates"`
	Errors     []string    `json:"errors"`
	Errs       []error     `json:"-"` // as mesmas falhas, para errors.Is
}

// AutoApplyScore é a nota a partir da qual os metadados são aplicados sem perguntar (RF-53).
const AutoApplyScore = 85

// ErrNoProvider: nenhum provedor ligado (sem chave do ComicVine e GCD desligado).
var ErrNoProvider = errors.New("nenhuma fonte de metadados ligada: informe a chave do ComicVine ou ligue o GCD nas configurações")

func limits(m settings.Metadata) Limits {
	return Limits{Interval: time.Duration(m.RateSeconds) * time.Second, TTL: time.Duration(m.CacheDays) * 24 * time.Hour}
}

// Search procura g em todos os provedores ligados. A falha de um não impede o outro.
func (f *Finder) Search(ctx context.Context, g Guess) (Result, error) {
	res := Result{Candidates: []Candidate{}, Errors: []string{}}
	s, err := f.Settings(ctx)
	if err != nil {
		return res, err
	}
	m := s.Metadata
	if m.ComicVineAPIKey == "" && !m.GCDEnabled {
		return res, ErrNoProvider
	}
	lim := limits(m)
	collect := func(c []Candidate, err error) error {
		res.Candidates = append(res.Candidates, c...)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			res.Errors = append(res.Errors, err.Error())
			res.Errs = append(res.Errs, err)
		}
		return nil
	}
	if m.GCDEnabled {
		if err := collect(f.GCD.Search(ctx, g, lim)); err != nil {
			return res, err
		}
	}
	if m.ComicVineAPIKey != "" {
		if err := collect(f.ComicVine.Search(ctx, m.ComicVineAPIKey, g, lim)); err != nil {
			return res, err
		}
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool { return res.Candidates[i].Score > res.Candidates[j].Score })
	return res, nil
}

// TestComicVine confere a chave salva.
func (f *Finder) TestComicVine(ctx context.Context) error {
	s, err := f.Settings(ctx)
	if err != nil {
		return err
	}
	if s.Metadata.ComicVineAPIKey == "" {
		return &ProviderError{Provider: "ComicVine", Kind: ErrUnauthorized, Detail: "nenhuma chave informada"}
	}
	return f.ComicVine.Test(ctx, s.Metadata.ComicVineAPIKey, limits(s.Metadata))
}
