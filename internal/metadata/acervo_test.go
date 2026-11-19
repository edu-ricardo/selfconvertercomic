package metadata

import (
	"bufio"
	"os"
	"sort"
	"testing"
)

// TestAcervoReal mede a identificação numa lista de edições reais, fora do repositório (o acervo do
// usuário não é publicado). Formato: linhas ">pasta" seguidas dos nomes-base dos arquivos dela.
//
//	QT_ACERVO=caminho/acervo.txt go test -run TestAcervoReal -v ./internal/metadata/
func TestAcervoReal(t *testing.T) {
	p := os.Getenv("QT_ACERVO")
	if p == "" {
		t.Skip("defina QT_ACERVO com a lista de edições")
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var paths []string
	dir := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
		case line[0] == '>':
			dir = line[1:] + "/"
		default:
			paths = append(paths, dir+line+".pdf")
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	gs := make([]Guess, len(paths))
	for i, p := range paths {
		gs[i] = ParseName(p)
	}
	InferNumbers(gs)

	ok, inferred := 0, 0
	series := map[string]int{}
	for i, g := range gs {
		if g.Series != "" && g.Number != "" {
			ok++
		} else {
			t.Logf("sem número: %s → %+v", paths[i], g)
		}
		if g.NumberInferred {
			inferred++
		}
		series[g.Series]++
	}
	names := make([]string, 0, len(series))
	for s := range series {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		t.Logf("série %-50q %d", s, series[s])
	}
	pct := 100 * float64(ok) / float64(len(gs))
	t.Logf("%d edições, %d com série e número (%.1f%%), %d números inferidos pela ordem, %d séries",
		len(gs), ok, pct, inferred, len(series))
	if pct < 90 {
		t.Errorf("identificação abaixo de 90%%: %.1f%%", pct)
	}
}
