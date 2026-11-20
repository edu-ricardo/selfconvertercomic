package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Link é o que um endereço colado pelo usuário aponta: uma edição ou uma série de uma fonte.
type Link struct {
	Provider string // gcd | comicvine
	Kind     string // issue | series
	ID       string
}

// ErrLink indica endereço que não é de edição/série do ComicVine nem do GCD.
var ErrLink = errors.New("link não reconhecido: use o endereço da página da edição (ou da série) no ComicVine ou no GCD")

// ErrNoIssue indica que a série não tem a edição pedida (ou, sem número, que tem mais de uma).
var ErrNoIssue = errors.New("essa série não tem a edição com esse número")

var (
	cvID  = regexp.MustCompile(`/(4000|4050)-(\d+)/?$`)
	gcdID = regexp.MustCompile(`^/(issue|series)/(\d+)/?$`)
)

// ParseLink reconhece:
//
//	https://comicvine.gamespot.com/secret-wars-1-the-war-begins/4000-31744/  (edição)
//	https://comicvine.gamespot.com/secret-wars/4050-2895/                    (série/volume)
//	https://www.comics.org/issue/726769/                                     (edição)
//	https://www.comics.org/series/41527/                                     (série)
func ParseLink(raw string) (Link, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return Link{}, ErrLink
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	switch host {
	case "comicvine.gamespot.com":
		if m := cvID.FindStringSubmatch(u.Path); m != nil {
			kind := "issue"
			if m[1] == "4050" {
				kind = "series"
			}
			return Link{"comicvine", kind, m[2]}, nil
		}
	case "comics.org":
		if m := gcdID.FindStringSubmatch(u.Path); m != nil {
			return Link{"gcd", m[1], m[2]}, nil
		}
	}
	return Link{}, ErrLink
}

// IssueInSeries acha a edição de número number numa série já conhecida de uma fonte (candidato que é
// só a série, ou link de série). Sem número, serve a edição única da série (encadernado, especial).
func (f *Finder) IssueInSeries(ctx context.Context, provider, seriesID, number string) (Candidate, error) {
	number = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(number), "#"))
	if number != "" {
		number = trimZeros(number)
	}
	s, err := f.Settings(ctx)
	if err != nil {
		return Candidate{}, err
	}
	lim := limits(s.Metadata)
	var c *Candidate
	switch provider {
	case "gcd":
		c, err = f.GCD.IssueInSeries(ctx, seriesID, number, lim)
	case "comicvine":
		if s.Metadata.ComicVineAPIKey == "" {
			return Candidate{}, &ProviderError{Provider: "ComicVine", Kind: ErrUnauthorized, Detail: "nenhuma chave informada"}
		}
		c, err = f.ComicVine.IssueInSeries(ctx, s.Metadata.ComicVineAPIKey, seriesID, number, lim)
	default:
		return Candidate{}, fmt.Errorf("fonte desconhecida: %q", provider)
	}
	if err != nil {
		return Candidate{}, err
	}
	if c == nil && number == "" {
		return Candidate{}, fmt.Errorf("%w: a série tem mais de uma edição, informe o número", ErrNoIssue)
	}
	if c == nil {
		return Candidate{}, fmt.Errorf("%w (#%s)", ErrNoIssue, number)
	}
	return *c, nil
}
