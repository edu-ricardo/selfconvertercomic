package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// GCD busca na API REST do Grand Comics Database (comics.org/api), sem chave. Os dados são
// CC BY-SA 4.0: a UI mostra a atribuição.
type GCD struct {
	F       *Fetcher
	BaseURL string // padrão: https://www.comics.org/api
}

type gcdSeries struct {
	APIURL      string   `json:"api_url"`
	Name        string   `json:"name"`
	Country     string   `json:"country"`
	Language    string   `json:"language"`
	YearBegan   any      `json:"year_began"` // número ou texto, conforme o recurso
	Issues      []string `json:"active_issues"`
	Descriptors []string `json:"issue_descriptors"`
}

type gcdIssue struct {
	APIURL           string `json:"api_url"`
	SeriesName       string `json:"series_name"`
	Number           string `json:"number"`
	Title            string `json:"title"`
	KeyDate          string `json:"key_date"`
	IndiciaPublisher string `json:"indicia_publisher"`
	Cover            string `json:"cover"`
	Stories          []struct {
		Type  string `json:"type"`
		Title string `json:"title"`
	} `json:"story_set"`
}

func (g *GCD) base() string {
	if g.BaseURL != "" {
		return g.BaseURL
	}
	return "https://www.comics.org/api"
}

func (g *GCD) get(ctx context.Context, path string, lim Limits, v any) error {
	body, err := g.F.Get(ctx, path, g.base()+path, lim, func(b []byte) error {
		if !json.Valid(b) {
			return &ProviderError{"GCD", ErrUnavailable, "resposta inválida"}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// Search procura séries pelo nome (o GCD busca por trecho do nome) e, nas mais parecidas, a edição de
// número gs.Number.
func (g *GCD) Search(ctx context.Context, gs Guess, lim Limits) ([]Candidate, error) {
	var page struct {
		Results []gcdSeries `json:"results"`
	}
	err := g.get(ctx, "/series/name/"+url.PathEscape(gs.Series)+"/?page=1", lim, &page)
	if errors.Is(err, errNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byID := map[string]gcdSeries{}
	series := make([]Candidate, 0, len(page.Results))
	for _, s := range page.Results {
		id := lastSegment(s.APIURL)
		byID[id] = s
		c := Candidate{Provider: "gcd", SeriesID: id, Series: s.Name, Language: s.Language,
			IssueCount: len(s.Issues), URL: siteURL(s.APIURL)}
		c.Year, _ = strconv.Atoi(fmt.Sprint(s.YearBegan))
		series = append(series, c)
	}
	return bestIssues(ctx, gs, series, func(ctx context.Context, s Candidate) (*Candidate, error) {
		ser := byID[s.SeriesID]
		issueURL := ""
		for i, d := range ser.Descriptors {
			if f := strings.Fields(d); len(f) > 0 && trimZeros(f[0]) == gs.Number && i < len(ser.Issues) {
				issueURL = ser.Issues[i]
				break
			}
		}
		if issueURL == "" {
			return nil, nil
		}
		var is gcdIssue
		if err := g.get(ctx, "/issue/"+lastSegment(issueURL)+"/", lim, &is); err != nil {
			return nil, err
		}
		c := s
		c.IssueID, c.Number, c.Publisher, c.CoverURL = lastSegment(is.APIURL), is.Number, is.IndiciaPublisher, is.Cover
		c.URL = siteURL(is.APIURL)
		c.Title = is.Title
		for _, st := range is.Stories {
			if c.Title == "" && st.Type == "comic story" && st.Title != "" {
				c.Title = st.Title
			}
		}
		if y, err := strconv.Atoi(firstN(is.KeyDate, 4)); err == nil && y > 0 {
			c.Year = y
		}
		return &c, nil
	})
}

// lastSegment: "https://www.comics.org/api/issue/76314/" → "76314".
func lastSegment(u string) string {
	parts := strings.Split(strings.Trim(u, "/"), "/")
	return parts[len(parts)-1]
}

// siteURL: endereço da API → página do site ("/api/issue/1/" → "/issue/1/").
func siteURL(api string) string { return strings.Replace(api, "/api/", "/", 1) }
