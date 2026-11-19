// Package metadata identifica as edições de quadrinhos: primeiro pelo nome do arquivo e pelas pastas
// (este arquivo) e depois pelos provedores.
package metadata

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ParserVersion muda quando o analisador de nomes muda: as edições identificadas por uma versão
// anterior são reanalisadas na próxima varredura.
const ParserVersion = 1

// Guess é o que dá para saber de uma edição só pelo caminho.
type Guess struct {
	Series         string `json:"series"`         // nome para exibição
	Volume         string `json:"volume"`         // "3" em "v3"
	Number         string `json:"number"`         // sem zeros à esquerda; "" = desconhecido
	NumberInferred bool   `json:"numberInferred"` // número deduzido pela ordem de leitura
	Count          int    `json:"count"`          // total da minissérie ("01 de 05")
	Year           int    `json:"year"`
	Group          string `json:"group"` // grupo de tradução ("MK-SQ")
	Order          int    `json:"order"` // prefixo de ordem de leitura ("034-..."); 0 = sem
}

// Key agrupa edições da mesma série (sem acentos, maiúsculas, espaços e pontuação; o volume não
// separa).
func (g Guess) Key() string { return Fold(g.Series) }

var (
	trailingParens = regexp.MustCompile(`\s*\(([^()]*)\)\s*$`)
	volumeToken    = regexp.MustCompile(`^[vV](\d{1,2})$`)
	folderPrefix   = regexp.MustCompile(`^\d{1,3} - `)
	romanNumeral   = regexp.MustCompile(`^(II|III|IV|V|VI|VII|VIII|IX|X)$`)
	groupToken     = regexp.MustCompile(`^[A-Z]{2,4}$`)
)

var chapterWords = map[string]bool{"capítulo": true, "capitulo": true, "cap": true, "cap.": true,
	"edição": true, "edicao": true, "parte": true, "ep": true, "ep.": true}

// truncatedLen é o tamanho em que os nomes com hífens do acervo aparecem cortados
// ("034-Guerras-Secretas-Desafio-Inf"): um número curto no fim pode ser só o começo do número.
const truncatedLen = 32

// ParseName deduz série, volume, número, ano e grupo do caminho relativo de um arquivo.
func ParseName(relPath string) Guess {
	var g Guess
	dir, name := path.Split(relPath)
	stem := strings.TrimSpace(strings.TrimSuffix(name, path.Ext(name)))
	for _, p := range []string{"Cópia de ", "cópia de ", "Copia de ", "copia de "} {
		stem = strings.TrimPrefix(stem, p)
	}

	parenNum := ""
	for {
		m := trailingParens.FindStringSubmatchIndex(stem)
		if m == nil {
			break
		}
		inner := strings.TrimSpace(stem[m[2]:m[3]])
		switch {
		case isYear(inner):
			g.Year = atoi(inner)
		case isDigits(inner):
			parenNum = trimZeros(inner)
		case inner != "" && g.Group == "":
			g.Group = inner
		}
		stem = strings.TrimSpace(stem[:m[0]])
	}
	// "Autor1,Autor2 - Título" e "Unknown author - Título": os autores não fazem parte da série.
	if i := strings.Index(stem, " - "); i > 0 {
		if first := stem[:i]; strings.Contains(first, ",") || strings.EqualFold(first, "unknown author") {
			stem = stem[i+3:]
		}
	}

	spaced := strings.Contains(stem, " ")
	var tokens []string
	if spaced {
		tokens = strings.Fields(stem)
	} else {
		tokens = strings.FieldsFunc(stem, func(r rune) bool { return r == '-' || r == '_' })
	}
	truncated := !spaced && len(stem) >= truncatedLen

	if len(tokens) > 1 && isDigits(tokens[0]) && len(tokens[0]) <= 3 {
		g.Order = atoi(tokens[0])
		tokens = tokens[1:]
		if len(tokens) > 0 && tokens[0] == "-" {
			tokens = tokens[1:]
		}
	}

	seriesEnd, numIdx, chapter := len(tokens), -1, false
	for i, t := range tokens {
		low := strings.ToLower(t)
		switch {
		case volumeToken.MatchString(t) && g.Volume == "":
			g.Volume = trimZeros(volumeToken.FindStringSubmatch(t)[1])
			seriesEnd = min(seriesEnd, i)
			if i+1 < len(tokens) && isDigits(tokens[i+1]) && numIdx < 0 {
				numIdx = i + 1
			}
		case (low == "vol" || low == "vol.") && i+1 < len(tokens) && isDigits(tokens[i+1]):
			seriesEnd = min(seriesEnd, i)
			if numIdx < 0 {
				numIdx = i + 1
			}
		case strings.HasPrefix(t, "#") && isDigits(t[1:]) && numIdx < 0:
			numIdx = i
			seriesEnd = min(seriesEnd, i)
		case chapterWords[low] && i+1 < len(tokens) && isDigits(tokens[i+1]) && numIdx < 0:
			numIdx, chapter = i+1, true
			seriesEnd = min(seriesEnd, i)
		}
	}
	if numIdx < 0 {
		numIdx = pickNumber(tokens)
		if numIdx >= 0 {
			seriesEnd = min(seriesEnd, numIdx)
		}
	}
	if numIdx >= 0 {
		g.Number = trimZeros(strings.TrimPrefix(tokens[numIdx], "#"))
		if truncated && numIdx == len(tokens)-1 && len(tokens[numIdx]) < 3 {
			g.Number = "" // "...-Secretas-0" cortado: o número de verdade é "00x"
		}
		rest := tokens[numIdx+1:]
		if len(rest) >= 2 && strings.EqualFold(rest[0], "de") && isDigits(rest[1]) {
			g.Count = atoi(rest[1])
		}
		for _, t := range rest {
			if isYear(t) && g.Year == 0 {
				g.Year = atoi(t)
			}
		}
		if last := rest; len(last) > 0 && g.Group == "" && groupToken.MatchString(last[len(last)-1]) {
			g.Group = last[len(last)-1]
		}
	}
	if g.Number == "" && numIdx < 0 && parenNum != "" {
		g.Number = parenNum
	}

	words := joinSeries(tokens[:seriesEnd])
	dirs := strings.Split(strings.Trim(dir, "/"), "/")
	if dirs[0] == "" {
		dirs = nil
	}
	switch {
	case (chapter || words == "") && len(dirs) > 0:
		last := dirs[len(dirs)-1]
		switch {
		case isYear(last) && len(dirs) > 1:
			g.Series = humanizeFolder(dirs[len(dirs)-2])
			if g.Year == 0 {
				g.Year = atoi(last)
			}
		case romanNumeral.MatchString(last) && len(dirs) > 1:
			g.Series = humanizeFolder(dirs[len(dirs)-2]) + " " + last
		default:
			g.Series = humanizeFolder(last)
		}
	case len(dirs) > 0 && folderMatches(tokens[:seriesEnd], humanizeFolder(dirs[len(dirs)-1])):
		g.Series = humanizeFolder(dirs[len(dirs)-1])
	default:
		g.Series = words
	}
	return g
}

// pickNumber escolhe o número da edição quando nada o marca: o primeiro número de até 3 dígitos
// depois da primeira palavra; senão qualquer número curto; senão um de 4 dígitos.
func pickNumber(tokens []string) int {
	for _, minIdx := range []int{1, 0} {
		for i := minIdx; i < len(tokens); i++ {
			if isDigits(tokens[i]) && len(tokens[i]) <= 3 {
				return i
			}
		}
	}
	for i, t := range tokens {
		if isDigits(t) && i > 0 {
			return i
		}
	}
	return -1
}

func joinSeries(tokens []string) string {
	s := strings.Join(tokens, " ")
	return strings.Trim(s, " -,_")
}

// humanizeFolder transforma o nome de uma pasta em nome de série: tira o prefixo de ordem
// ("01 - "), troca os "_" que substituem caracteres proibidos ("Loki_ Agente" → "Loki: Agente",
// "X-Men _92" → "X-Men '92", "Tudo Morre_" → "Tudo Morre") e capitaliza nomes todos em minúsculas.
func humanizeFolder(s string) string {
	s = folderPrefix.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "_ ", ": ")
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		if r != '_' {
			b.WriteRune(r)
			continue
		}
		switch {
		case i == len(rs)-1:
		case i > 0 && rs[i-1] == ' ' && unicode.IsDigit(rs[i+1]):
			b.WriteRune('\'')
		default:
			b.WriteRune(' ')
		}
	}
	s = strings.TrimSpace(b.String())
	if s == strings.ToLower(s) {
		s = titleCase(s)
	}
	return s
}

var smallWords = map[string]bool{"a": true, "o": true, "e": true, "de": true, "da": true, "do": true,
	"das": true, "dos": true, "em": true, "no": true, "na": true, "vs": true}

func titleCase(s string) string {
	ws := strings.Fields(s)
	for i, w := range ws {
		if i > 0 && smallWords[w] {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		ws[i] = string(r)
	}
	return strings.Join(ws, " ")
}

// folderMatches diz se as palavras da série no nome do arquivo correspondem à pasta, mesmo com
// prefixo a mais ("Guerras Secretas Inferno" ↔ "Inferno"), nome cortado ("Desafio Inf") ou
// acentos perdidos ("Capit Marvel" ↔ "Capitã Marvel").
func folderMatches(tokens []string, folder string) bool {
	fb := Fold(folder)
	if fb == "" {
		return false
	}
	for i := range tokens {
		s := Fold(strings.Join(tokens[i:], ""))
		if len(s) < min(6, len(fb)) {
			break
		}
		if strings.HasPrefix(fb, s) {
			return true
		}
		if len(s) >= 5 {
			for l := max(1, len(s)-2); l <= min(len(fb), len(s)+2); l++ {
				if levenshtein(s, fb[:l]) <= len(s)/5 {
					return true
				}
			}
		}
	}
	return false
}

var accents = map[rune]rune{'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'é': 'e', 'è': 'e', 'ê': 'e',
	'ë': 'e', 'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ç': 'c', 'ñ': 'n'}

// Fold reduz um texto a letras e dígitos ASCII minúsculos, sem acentos ("Capitã Marvel &" → "capitamarvel").
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if a, ok := accents[r]; ok {
			r = a
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// InferNumbers completa os números desconhecidos pela ordem de leitura: numa série em que todas as
// edições têm o prefixo de ordem e os números conhecidos coincidem com a posição na ordem, cada edição
// sem número recebe a sua posição. Cópias com a mesma ordem (árvores duplicadas) contam uma vez.
func InferNumbers(gs []Guess) {
	groups := map[string][]int{}
	for i, g := range gs {
		groups[g.Key()] = append(groups[g.Key()], i)
	}
	for _, idx := range groups {
		var orders []int
		seen := map[int]bool{}
		missing, ok := false, true
		for _, i := range idx {
			o := gs[i].Order
			if o == 0 {
				ok = false
				break
			}
			if !seen[o] {
				seen[o] = true
				orders = append(orders, o)
			}
			missing = missing || gs[i].Number == ""
		}
		if !ok || !missing {
			continue
		}
		sort.Ints(orders)
		rank := make(map[int]int, len(orders))
		for r, o := range orders {
			rank[o] = r + 1
		}
		for _, i := range idx {
			if n := gs[i].Number; n != "" && !gs[i].NumberInferred && atoi(n) != rank[gs[i].Order] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		for _, i := range idx {
			if gs[i].Number == "" {
				gs[i].Number = strconv.Itoa(rank[gs[i].Order])
				gs[i].NumberInferred = true
			}
		}
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isYear(s string) bool {
	if len(s) != 4 || !isDigits(s) {
		return false
	}
	y := atoi(s)
	return y >= 1930 && y <= 2035
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func trimZeros(s string) string {
	t := strings.TrimLeft(s, "0")
	if t == "" && s != "" {
		return "0"
	}
	return t
}
