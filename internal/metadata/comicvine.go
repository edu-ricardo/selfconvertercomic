package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ComicVine busca na API do ComicVine (comicvine.gamespot.com/api), que exige chave e limita a
// 200 pedidos por hora por recurso.
type ComicVine struct {
	F       *Fetcher
	BaseURL string // padrão: https://comicvine.gamespot.com/api
}

type cvResponse struct {
	StatusCode int             `json:"status_code"`
	Error      string          `json:"error"`
	Results    json.RawMessage `json:"results"`
}

type cvVolume struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	StartYear     string `json:"start_year"`
	CountOfIssues int    `json:"count_of_issues"`
	SiteURL       string `json:"site_detail_url"`
	Publisher     *struct {
		Name string `json:"name"`
	} `json:"publisher"`
}

type cvIssue struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	IssueNumber string `json:"issue_number"`
	CoverDate   string `json:"cover_date"`
	SiteURL     string `json:"site_detail_url"`
	Image       *struct {
		MediumURL string `json:"medium_url"`
	} `json:"image"`
}

func (cv *ComicVine) base() string {
	if cv.BaseURL != "" {
		return cv.BaseURL
	}
	return "https://comicvine.gamespot.com/api"
}

// get chama um recurso; params não deve conter a chave (ela é acrescentada aqui e fica fora do cache).
func (cv *ComicVine) get(ctx context.Context, key, resource string, params url.Values, lim Limits) (json.RawMessage, error) {
	params.Set("format", "json")
	cacheKey := resource + "?" + params.Encode()
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("api_key", key)
	body, err := cv.F.Get(ctx, cacheKey, cv.base()+"/"+resource+"/?"+q.Encode(), lim, func(b []byte) error {
		var r cvResponse
		if err := json.Unmarshal(b, &r); err != nil {
			return &ProviderError{"ComicVine", ErrUnavailable, "resposta inválida"}
		}
		switch r.StatusCode {
		case 1:
			return nil
		case 100:
			return &ProviderError{"ComicVine", ErrUnauthorized, ""}
		case 107:
			return &ProviderError{"ComicVine", ErrRateLimited, ""}
		default:
			return &ProviderError{"ComicVine", ErrUnavailable, r.Error}
		}
	})
	if err != nil {
		return nil, err
	}
	var r cvResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Results, nil
}

// Test confere a chave com um pedido barato.
func (cv *ComicVine) Test(ctx context.Context, key string, lim Limits) error {
	lim.TTL = 0 // não aproveita o cache: a chave pode ter mudado
	_, err := cv.get(ctx, key, "types", url.Values{}, lim)
	return err
}

// Search procura volumes pelo nome da série e, nos mais parecidos, a edição de número g.Number.
func (cv *ComicVine) Search(ctx context.Context, key string, g Guess, lim Limits) ([]Candidate, error) {
	raw, err := cv.get(ctx, key, "search", url.Values{
		"resources":  {"volume"},
		"query":      {g.Series},
		"limit":      {"10"},
		"field_list": {"id,name,start_year,count_of_issues,publisher,site_detail_url"},
	}, lim)
	if err != nil {
		return nil, err
	}
	var vols []cvVolume
	if err := json.Unmarshal(raw, &vols); err != nil {
		return nil, fmt.Errorf("ComicVine: volumes: %w", err)
	}
	series := make([]Candidate, 0, len(vols))
	for _, v := range vols {
		c := Candidate{Provider: "comicvine", SeriesID: strconv.Itoa(v.ID), Series: v.Name, IssueCount: v.CountOfIssues,
			URL: v.SiteURL}
		c.Year, _ = strconv.Atoi(v.StartYear)
		if v.Publisher != nil {
			c.Publisher = v.Publisher.Name
		}
		series = append(series, c)
	}
	return bestIssues(ctx, g, series, func(ctx context.Context, s Candidate) (*Candidate, error) {
		raw, err := cv.get(ctx, key, "issues", url.Values{
			"filter":     {"volume:" + s.SeriesID + ",issue_number:" + g.Number},
			"field_list": {"id,name,issue_number,cover_date,image,site_detail_url"},
			"limit":      {"1"},
		}, lim)
		if err != nil {
			return nil, err
		}
		var issues []cvIssue
		if err := json.Unmarshal(raw, &issues); err != nil || len(issues) == 0 {
			return nil, err
		}
		is := issues[0]
		c := s
		c.IssueID, c.Number, c.Title, c.URL = strconv.Itoa(is.ID), is.IssueNumber, is.Name, is.SiteURL
		if y, err := strconv.Atoi(firstN(is.CoverDate, 4)); err == nil {
			c.Year = y
		}
		if is.Image != nil {
			c.CoverURL = is.Image.MediumURL
		}
		return &c, nil
	})
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
