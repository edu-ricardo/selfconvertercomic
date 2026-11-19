package organize

import (
	"fmt"
	"strings"
	"testing"
)

const def = "{editora}/{serie} ({ano})/{serie} ({ano}) #{numero}"

func TestRender(t *testing.T) {
	cases := []struct {
		tpl     string
		v       map[string]string
		want    string
		missing string
	}{
		{def, map[string]string{"editora": "Editora Abril S.A.", "serie": "Secret Wars (Guerras Secretas)", "ano": "1986", "numero": "1"},
			"Editora Abril S.A/Secret Wars (Guerras Secretas) (1986)/Secret Wars (Guerras Secretas) (1986) #1", ""},
		// Sem ano: os parênteses vazios somem.
		{def, map[string]string{"editora": "Marvel", "serie": "Magneto", "numero": "18"}, "Marvel/Magneto/Magneto #18", ""},
		// Sem editora: a pasta vazia some.
		{def, map[string]string{"serie": "Seda", "ano": "2015", "numero": "7"}, "Seda (2015)/Seda (2015) #7", ""},
		// Caracteres proibidos em exFAT/NTFS e barra no valor.
		{"{serie} #{numero3}", map[string]string{"serie": `Loki: Agente de Asgard? "1/2"`, "numero": "14"},
			"Loki - Agente de Asgard '1-2' #014", ""},
		{"{serie} #{numero3}", map[string]string{"serie": "X", "numero": "120"}, "X #120", ""},
		{"{serie} #{numero3}", map[string]string{"serie": "X", "numero": "1.5"}, "X #1.5", ""},
		// Faltando o obrigatório.
		{def, map[string]string{"serie": "Mark Millar - Guerra Civil", "ano": "2022"}, "Mark Millar - Guerra Civil (2022)/Mark Millar - Guerra Civil (2022)", "número"},
		{def, map[string]string{"numero": "1"}, "1", "série"},
	}
	for _, c := range cases {
		v := c.v
		if n, ok := v["numero"]; ok {
			v["numero3"] = pad3(n)
		}
		got, missing := Render(c.tpl, v)
		if got != c.want || strings.Join(missing, ",") != c.missing {
			t.Errorf("%q %v:\n  veio %q faltando %v\n  quer %q faltando %q", c.tpl, c.v, got, missing, c.want, c.missing)
		}
	}
	long, _ := Render("{serie}", map[string]string{"serie": strings.Repeat("Ação ", 60)})
	if len(long) > maxSegment {
		t.Errorf("nome longo demais: %d bytes", len(long))
	}
}

func TestCheckTemplate(t *testing.T) {
	for tpl, ok := range map[string]bool{def: true, "{serie}/{original}": true, "{serie}/{autor}": false, "{editora}/{numero}": false} {
		if err := CheckTemplate(tpl); (err == nil) != ok {
			t.Errorf("%q: %v", tpl, err)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	list := []string{"S #10", "S #2", "S #1", "R #3", "S #1.5"}
	sortNatural(list)
	if fmt.Sprint(list) != "[R #3 S #1 S #1.5 S #2 S #10]" {
		t.Errorf("ordem: %v", list)
	}
}

func sortNatural(l []string) {
	for i := range l {
		for j := i + 1; j < len(l); j++ {
			if naturalLess(l[j], l[i]) {
				l[i], l[j] = l[j], l[i]
			}
		}
	}
}
