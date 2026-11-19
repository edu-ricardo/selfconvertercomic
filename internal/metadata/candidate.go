package metadata

import (
	"slices"
	"strings"
	"unicode"
)

// Candidate é uma edição encontrada num provedor para uma edição do acervo.
type Candidate struct {
	Provider   string `json:"provider"` // comicvine | gcd
	IssueID    string `json:"issueId"`  // "" quando só a série foi encontrada
	SeriesID   string `json:"seriesId"`
	Series     string `json:"series"`
	Number     string `json:"number"`
	Title      string `json:"title"`
	Year       int    `json:"year"`
	Publisher  string `json:"publisher"`
	Language   string `json:"language"` // "pt", "en"... ("" = não informado)
	IssueCount int    `json:"issueCount"`
	CoverURL   string `json:"coverUrl"`
	URL        string `json:"url"` // página da edição (ou série) no site do provedor
	Score      int    `json:"score"`
}

// Score dá de 0 a 100 para o quanto c parece ser a edição g: nome da série (até 60), número (20),
// ano (+15 igual … −20 se longe) e idioma português (+5, a edição brasileira é a preferida). Sem ano
// de um dos lados, no máximo maxWithoutYear: séries diferentes com o mesmo nome e número são comuns
// (ex.: "Desafio Infinito" de 1995 e de 2015).
func Score(g Guess, c Candidate) int {
	s := int(nameSimilarity(g.Series, c.Series) * 60)
	if g.Number != "" && trimZeros(c.Number) == g.Number {
		s += 20
	}
	if g.Year != 0 && c.Year != 0 {
		switch d := abs(g.Year - c.Year); {
		case d == 0:
			s += 15
		case d <= 2:
			s += 10
		case d <= 5:
			s += 5
		default:
			s -= 20
		}
	}
	if c.Language == "pt" {
		s += 5
	}
	if g.Year == 0 || c.Year == 0 {
		s = min(s, maxWithoutYear)
	}
	return max(0, min(100, s))
}

// nameSimilarity compara o nome buscado com o do candidato, sem acentos nem pontuação: 1 = iguais;
// o nome buscado inteiro, palavra por palavra, dentro do candidato ("Guerras Secretas" em "Secret Wars
// (Guerras Secretas)") vale entre 0,6 e 1 conforme o tamanho; senão, a distância de edição. O
// contrário não conta: "Infinito" dentro de "Desafio Infinito" é uma série mais genérica, e
// "Os Vingadores" não está em "Os Novos Vingadores" (as palavras não são seguidas).
func nameSimilarity(query, cand string) float64 {
	fq, fc := Fold(query), Fold(cand)
	if fq == "" || fc == "" {
		return 0
	}
	if fq == fc {
		return 1
	}
	r := 1 - float64(levenshtein(fq, fc))/float64(max(len(fq), len(fc)))
	if len(fq) >= 4 && len(fq) < len(fc) && containsWords(words(cand), words(query)) {
		r = max(r, 0.6+0.4*float64(len(fq))/float64(len(fc)))
	}
	return max(0, r)
}

// words separa um nome em palavras já sem acentos ("Secret Wars (Guerras Secretas)" →
// [secret wars guerras secretas]).
func words(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if f := Fold(w); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// containsWords diz se sub aparece em seq como palavras seguidas.
func containsWords(seq, sub []string) bool {
	for i := 0; i+len(sub) <= len(seq); i++ {
		if slices.Equal(seq[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

const maxWithoutYear = 75

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
