package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Details são os metadados completos de uma edição escolhida, prontos para o ComicInfo.xml.
type Details struct {
	Provider     string   `json:"provider"`
	IssueID      string   `json:"issueId"`
	URL          string   `json:"url"`
	Series       string   `json:"series"`
	Number       string   `json:"number"`
	Title        string   `json:"title"`
	Year         int      `json:"year"`
	Month        int      `json:"month"`
	Publisher    string   `json:"publisher"`
	Language     string   `json:"language"`
	Writers      []string `json:"writers"`
	Pencillers   []string `json:"pencillers"`
	Inkers       []string `json:"inkers"`
	Colorists    []string `json:"colorists"`
	Letterers    []string `json:"letterers"`
	CoverArtists []string `json:"coverArtists"`
	Summary      string   `json:"summary"`
	CoverURL     string   `json:"coverUrl"`
}

// maxSummary limita a sinopse (as do ComicVine podem ser artigos inteiros).
const maxSummary = 2000

// Details busca os metadados completos da edição issueID no provedor ("gcd" ou "comicvine").
func (f *Finder) Details(ctx context.Context, provider, issueID string) (Details, error) {
	s, err := f.Settings(ctx)
	if err != nil {
		return Details{}, err
	}
	lim := limits(s.Metadata)
	switch provider {
	case "gcd":
		return f.GCD.Details(ctx, issueID, lim)
	case "comicvine":
		if s.Metadata.ComicVineAPIKey == "" {
			return Details{}, &ProviderError{Provider: "ComicVine", Kind: ErrUnauthorized, Detail: "nenhuma chave informada"}
		}
		return f.ComicVine.Details(ctx, s.Metadata.ComicVineAPIKey, issueID, lim)
	}
	return Details{}, fmt.Errorf("provedor desconhecido: %q", provider)
}

var seriesYearSuffix = regexp.MustCompile(`\s*\(\d{4} series\)$`)

// Details lê a edição e a série (idioma) no GCD.
func (g *GCD) Details(ctx context.Context, issueID string, lim Limits) (Details, error) {
	var is struct {
		gcdIssue
		Series  string `json:"series"`
		Stories []struct {
			Type     string `json:"type"`
			Title    string `json:"title"`
			Script   string `json:"script"`
			Pencils  string `json:"pencils"`
			Inks     string `json:"inks"`
			Colors   string `json:"colors"`
			Letters  string `json:"letters"`
			Synopsis string `json:"synopsis"`
		} `json:"story_set"`
	}
	if err := g.get(ctx, "/issue/"+issueID+"/", lim, &is); err != nil {
		return Details{}, err
	}
	d := Details{Provider: "gcd", IssueID: issueID, URL: siteURL(is.APIURL), Number: is.Number, Title: is.Title,
		Publisher: is.IndiciaPublisher, CoverURL: is.Cover, Series: seriesYearSuffix.ReplaceAllString(is.SeriesName, "")}
	d.Year, d.Month = dateParts(is.KeyDate)
	// A editora é a da série (o "expediente" de cada edição varia: "Editora Abril S.A.", "Abril Jovem"…) e
	// edições com cadastro incompleto no GCD herdam o ano da série: todas as edições de uma série ficam
	// iguais (mesma pasta ao organizar).
	if is.Series != "" {
		var ser gcdSeries
		if err := g.get(ctx, "/series/"+lastSegment(is.Series)+"/", lim, &ser); err == nil {
			d.Language = ser.Language
			if d.Year == 0 {
				d.Year, _ = strconv.Atoi(fmt.Sprint(ser.YearBegan))
			}
			if ser.Publisher != "" {
				var pub struct {
					Name string `json:"name"`
				}
				if err := g.get(ctx, "/publisher/"+lastSegment(ser.Publisher)+"/", lim, &pub); err == nil && pub.Name != "" {
					d.Publisher = pub.Name
				}
			}
		}
	}
	for _, st := range is.Stories {
		switch st.Type {
		case "cover":
			d.CoverArtists = addCredits(d.CoverArtists, st.Pencils)
		case "comic story":
			if d.Title == "" {
				d.Title = st.Title
			}
			if d.Summary == "" {
				d.Summary = st.Synopsis
			}
			d.Writers = addCredits(d.Writers, st.Script)
			d.Pencillers = addCredits(d.Pencillers, st.Pencils)
			d.Inkers = addCredits(d.Inkers, st.Inks)
			d.Colorists = addCredits(d.Colorists, st.Colors)
			d.Letterers = addCredits(d.Letterers, st.Letters)
		}
	}
	d.Summary = clip(d.Summary)
	return d, nil
}

// Details lê a edição e o volume (editora) no ComicVine.
func (cv *ComicVine) Details(ctx context.Context, key, issueID string, lim Limits) (Details, error) {
	raw, err := cv.get(ctx, key, "issue/4000-"+issueID, url.Values{
		"field_list": {"id,name,issue_number,cover_date,description,person_credits,volume,image,site_detail_url"},
	}, lim)
	if err != nil {
		return Details{}, err
	}
	var is struct {
		cvIssue
		Description string `json:"description"`
		Volume      struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"volume"`
		Credits []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"person_credits"`
	}
	if err := json.Unmarshal(raw, &is); err != nil {
		return Details{}, fmt.Errorf("ComicVine: edição: %w", err)
	}
	d := Details{Provider: "comicvine", IssueID: issueID, URL: is.SiteURL, Series: is.Volume.Name, Number: is.IssueNumber,
		Title: is.Name, Summary: clip(htmlText(is.Description))}
	d.Year, d.Month = dateParts(is.CoverDate)
	if is.Image != nil {
		d.CoverURL = is.Image.MediumURL
	}
	for _, c := range is.Credits {
		for _, role := range strings.Split(c.Role, ",") {
			switch strings.TrimSpace(role) {
			case "writer":
				d.Writers = append(d.Writers, c.Name)
			case "penciler", "penciller", "artist":
				d.Pencillers = append(d.Pencillers, c.Name)
			case "inker":
				d.Inkers = append(d.Inkers, c.Name)
			case "colorist":
				d.Colorists = append(d.Colorists, c.Name)
			case "letterer":
				d.Letterers = append(d.Letterers, c.Name)
			case "cover":
				d.CoverArtists = append(d.CoverArtists, c.Name)
			}
		}
	}
	if is.Volume.ID != 0 {
		raw, err := cv.get(ctx, key, "volume/4050-"+strconv.Itoa(is.Volume.ID), url.Values{"field_list": {"publisher"}}, lim)
		if err == nil {
			var v cvVolume
			if json.Unmarshal(raw, &v) == nil && v.Publisher != nil {
				d.Publisher = v.Publisher.Name
			}
		}
	}
	return d, nil
}

var creditNotes = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)

// addCredits junta os nomes de um campo de créditos do GCD ("Jim Shooter (credited); Bob Layton")
// sem anotações, sem desconhecidos ("?", "None") e sem repetir.
func addCredits(list []string, field string) []string {
	for _, part := range strings.Split(field, ";") {
		name := strings.TrimSpace(creditNotes.ReplaceAllString(part, ""))
		switch strings.ToLower(name) {
		case "", "?", "none", "typeset", "various":
			continue
		}
		dup := false
		for _, n := range list {
			dup = dup || n == name
		}
		if !dup {
			list = append(list, name)
		}
	}
	return list
}

// dateParts lê "1986-08-12" / "1984-05-00" / "1986".
func dateParts(s string) (year, month int) {
	parts := strings.Split(s, "-")
	year, _ = strconv.Atoi(parts[0])
	if len(parts) > 1 {
		month, _ = strconv.Atoi(parts[1])
	}
	return year, month
}

var htmlTags = regexp.MustCompile(`<[^>]*>`)

func htmlText(s string) string {
	s = strings.NewReplacer("</p>", "\n\n", "<br>", "\n", "<br/>", "\n", "<br />", "\n").Replace(s)
	s = html.UnescapeString(htmlTags.ReplaceAllString(s, ""))
	return strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n"))
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxSummary {
		return string(r[:maxSummary-1]) + "…"
	}
	return s
}
