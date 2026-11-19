package metadata

import "testing"

// Nomes reais do acervo (rodada 8). Campos vazios no esperado não são conferidos, exceto Number,
// que é sempre conferido ("" = número desconhecido).
func TestParseName(t *testing.T) {
	const gs = "Marvel/guerras_secretas/2015/"
	cases := []struct {
		path string
		want Guess
	}{
		{"Marvel/guerras_secretas/1984/Capítulo 01.pdf", Guess{Series: "Guerras Secretas", Number: "1", Year: 1984}},
		{"Marvel/guerras_secretas/II/Capítulo 04.cbz", Guess{Series: "Guerras Secretas II", Number: "4"}},
		{gs + "00 - Prelúdio/Tempo Esgotado/Cópia de Capítulo 37.pdf", Guess{Series: "Tempo Esgotado", Number: "37"}},
		{gs + "00 - Prelúdio/Tempo Esgotado/Os Novos Vingadores v3 #25 (MK-SQ).pdf", Guess{Series: "Os Novos Vingadores", Volume: "3", Number: "25", Group: "MK-SQ", Year: 2015}},
		{gs + "00 - Prelúdio/Tempo Esgotado/Vingadores Mundiais #17.pdf", Guess{Series: "Vingadores Mundiais", Number: "17"}},
		{gs + "00 - Prelúdio/Tempo Esgotado/Miles-Morales-Ultimate-Homem-Ara (9).pdf", Guess{Series: "Miles Morales Ultimate Homem Ara", Number: "9"}},
		{gs + "01 - Últimos Dias/00.pdf", Guess{Series: "Últimos Dias", Number: "0"}},
		{gs + "01 - Últimos Dias/Capítulo 01.pdf", Guess{Series: "Últimos Dias", Number: "1"}},
		{gs + "01 - Últimos Dias/Justiceiro/018-O-Justiceiro-V9-019.pdf", Guess{Series: "Justiceiro", Volume: "9", Number: "19", Order: 18}},
		{gs + "01 - Últimos Dias/Loki_ Agente de Asgard/008-Loki-Agente-de-Asgard-014.pdf", Guess{Series: "Loki: Agente de Asgard", Number: "14"}},
		{gs + "01 - Últimos Dias/Magneto/014-Magneto-v3-018.cbz", Guess{Series: "Magneto", Volume: "3", Number: "18"}},
		{gs + "01 - Últimos Dias/Ms. Marvel/010-Miss-Marvel-v3-016.pdf", Guess{Series: "Ms. Marvel", Volume: "3", Number: "16"}},
		{gs + "01 - Últimos Dias/Mulher-Aranha/005-Mulher-Aranha-v5-10.pdf", Guess{Series: "Mulher-Aranha", Volume: "5", Number: "10"}},
		{gs + "01 - Últimos Dias/Seda/021-Seda-07.pdf", Guess{Series: "Seda", Number: "7"}},
		{gs + "01 - Últimos Dias/Surfista Prateado/Surfista-Prateado-013-2015-SQ.pdf", Guess{Series: "Surfista Prateado", Number: "13", Year: 2015, Group: "SQ"}},
		{gs + "01 - Últimos Dias/Viúva Negra/Viva-Negra-V5-19-2015.pdf", Guess{Series: "Viúva Negra", Volume: "5", Number: "19", Year: 2015}},
		{gs + "02 - Bem-vindo ao Mundo-De-Batalha/Diário das Guerras Secretas/038-Dirio-das-Guerras-Secretas-0.pdf", Guess{Series: "Diário das Guerras Secretas", Number: ""}},
		{gs + "02 - Bem-vindo ao Mundo-De-Batalha/Fugitivos/070-Fugitivos-V4-001.pdf", Guess{Series: "Fugitivos", Volume: "4", Number: "1"}},
		{gs + "02 - Bem-vindo ao Mundo-De-Batalha/Mestre do Kung-Fu/028-Mestre-do-Kung-Fu-V2-001.pdf", Guess{Series: "Mestre do Kung-Fu", Volume: "2", Number: "1"}},
		{gs + "02 - Bem-vindo ao Mundo-De-Batalha/Zumbis Marvel/052-Zumbis-Marvel-V2-001-Guerras.pdf", Guess{Series: "Zumbis Marvel", Volume: "2", Number: "1"}},
		{gs + "03 - Zonas de Guerra 1/1602_ Angela - A Caçadora de Bruxas/066 - 1602 - Angela A Caçadora de Bruxas 001.pdf", Guess{Series: "1602: Angela - A Caçadora de Bruxas", Number: "1", Order: 66}},
		{gs + "03 - Zonas de Guerra 1/1602_ Angela - A Caçadora de Bruxas/196-1602-Angela-A-Caadora-de-Bru.pdf", Guess{Series: "1602: Angela - A Caçadora de Bruxas", Number: "", Order: 196}},
		{gs + "03 - Zonas de Guerra 1/1872/149 Guerras Secretas 1872 V1 002.pdf", Guess{Series: "1872", Volume: "1", Number: "2"}},
		{gs + "03 - Zonas de Guerra 1/Capitã Marvel & A Tropa da Carol/105-Capit-Marvel-e-a-Tropa-da-Ca.pdf", Guess{Series: "Capitã Marvel & A Tropa da Carol", Number: ""}},
		{gs + "03 - Zonas de Guerra 1/Capitã Marvel & A Tropa da Carol/067 Capitã Marvel e a Tropa da Carol 001.pdf", Guess{Series: "Capitã Marvel & A Tropa da Carol", Number: "1"}},
		{gs + "03 - Zonas de Guerra 1/Desafio Infinito/203 Guerras Secretas - Desafio Infinito V2 005.pdf", Guess{Series: "Desafio Infinito", Volume: "2", Number: "5"}},
		{gs + "03 - Zonas de Guerra 1/Desafio Infinito/034-Guerras-Secretas-Desafio-Inf.pdf", Guess{Series: "Desafio Infinito", Number: ""}},
		{gs + "03 - Zonas de Guerra 1/Espetacular Homem-Aranha_ Renovando Seus Votos/150 O Espetacular Homem-Aranha - Renovando Seus Votos 004.pdf", Guess{Series: "Espetacular Homem-Aranha: Renovando Seus Votos", Number: "4"}},
		{gs + "03 - Zonas de Guerra 1/Espetacular Homem-Aranha_ Renovando Seus Votos/047-O-Espetacular-Homem-Aranha-R.pdf", Guess{Series: "Espetacular Homem-Aranha: Renovando Seus Votos", Number: ""}},
		{gs + "03 - Zonas de Guerra 1/Gigantesca Pequena Marvel_ Vingadores vs. X-Men/128 Gigantesca Pequena Marvel Vingadores vs X-Men 003.pdf", Guess{Series: "Gigantesca Pequena Marvel: Vingadores vs. X-Men", Number: "3"}},
		{gs + "03 - Zonas de Guerra 1/Guerra Das Armadures/182 Guerras Secretas - Guerra das Armaduras 005.pdf", Guess{Series: "Guerra Das Armadures", Number: "5"}},
		{gs + "03 - Zonas de Guerra 1/Inferno/036-Guerras-Secretas-Inferno-V1.pdf", Guess{Series: "Inferno", Volume: "1", Number: ""}},
		{gs + "03 - Zonas de Guerra 1/M.O.D.O.C. Assassino/033-MODOC-Assassino-001.pdf", Guess{Series: "M.O.D.O.C. Assassino", Number: "1"}},
		{gs + "03 - Zonas de Guerra 1/V-Force/083 V-Force V1 002 - Guerras Secretas.pdf", Guess{Series: "V-Force", Volume: "1", Number: "2"}},
		{gs + "04 - Zonas de Guerra 2/2099/037-Guerras-Secretas-2099-V1-001.pdf", Guess{Series: "2099", Volume: "1", Number: "1"}},
		{gs + "04 - Zonas de Guerra 2/X-Men _92/040-X-Men-92-V1-001-Guerras-Secr.pdf", Guess{Series: "X-Men '92", Volume: "1", Number: "1"}},
		{gs + "04 - Zonas de Guerra 2/Onde Os Monstros Habitam/039-Onde-os-Monstros-Habitam-V2.pdf", Guess{Series: "Onde Os Monstros Habitam", Volume: "2", Number: ""}},
		{gs + "05 - Mundo de Guerra/Senhor das Estralas e Kitty Pryde/113-Senhor-das-Estrelas-e-Kitty.pdf", Guess{Series: "Senhor das Estralas e Kitty Pryde", Number: ""}},
		{gs + "05 - Mundo de Guerra/Ultimate_ O FIM/025-Ultimate-O-Fim-01-de-05-2015.pdf", Guess{Series: "Ultimate: O FIM", Number: "1", Count: 5, Year: 2015}},
		{gs + "07 - Tudo Morre_/102-Guerras-Secretas-Cerco-V2-00.pdf", Guess{Series: "Guerras Secretas Cerco", Volume: "2", Number: ""}},
		{gs + "07 - Tudo Morre_/Capítulo 06.pdf", Guess{Series: "Tudo Morre", Number: "6"}},
		{gs + "07 - Tudo Morre_/202-Thors-004.pdf", Guess{Series: "Thors", Number: "4"}},
		{"DC Comics/Unknown author - Batman - Lendas do Cavaleiro das Trevas - Alan Davis Vol. 1.cbr", Guess{Series: "Batman - Lendas do Cavaleiro das Trevas - Alan Davis", Number: "1"}},
		{"Marvel/Grandes Eventos/Mark Millar - Guerra Civil (2022).cbr", Guess{Series: "Mark Millar - Guerra Civil", Number: "", Year: 2022}},
		{"Marvel/Grandes Eventos/Chris Claremont,Len Wein,Dave Cockrum,John Byrne - X-Men Essenciais- Deus Ama, O Homem Mata-TPB 18.cbz", Guess{Number: "18"}},
		{"Marvel/Kitty Pryde/2001/Capítulo 03.pdf", Guess{Series: "Kitty Pryde", Number: "3", Year: 2001}},
		{"Marvel/Kitty Pryde/Mekanix/Capítulo 02.pdf", Guess{Series: "Mekanix", Number: "2"}},
		{"Batman 001.cbz", Guess{Series: "Batman", Number: "1"}},
		{"Marvel/1984/Extras/Batman 001.cbz", Guess{Series: "Batman", Number: "1", Year: 1984}},
	}
	for _, c := range cases {
		got := ParseName(c.path)
		w := c.want
		if (w.Series != "" && got.Series != w.Series) || got.Number != w.Number ||
			(w.Volume != "" && got.Volume != w.Volume) || (w.Year != 0 && got.Year != w.Year) ||
			(w.Group != "" && got.Group != w.Group) || (w.Count != 0 && got.Count != w.Count) ||
			(w.Order != 0 && got.Order != w.Order) {
			t.Errorf("%s\n  quer %+v\n  veio %+v", c.path, w, got)
		}
	}
}

func TestSeriesKey(t *testing.T) {
	a := ParseName("x/Capitã Marvel & A Tropa da Carol/105-Capit-Marvel-e-a-Tropa-da-Ca.pdf")
	b := ParseName("y/Capitã Marvel & A Tropa da Carol/067 Capitã Marvel e a Tropa da Carol 001.pdf")
	if a.Key() == "" || a.Key() != b.Key() {
		t.Errorf("mesma série deveria ter a mesma chave: %q %q", a.Key(), b.Key())
	}
	if ParseName("Magneto/Magneto-v3-018.pdf").Key() != ParseName("Magneto/Magneto 019.pdf").Key() {
		t.Error("volume não deveria separar a série")
	}
}

// Número pela ordem de leitura: só quando os números conhecidos batem com a ordem.
func TestInferNumbers(t *testing.T) {
	const d = "Marvel/guerras_secretas/2015/03 - Zonas de Guerra 1/Desafio Infinito/"
	const dup = "arquivospdf/Quadrinhos/" + d
	paths := []string{
		d + "034-Guerras-Secretas-Desafio-Inf.pdf",
		d + "074-Guerras-Secretas-Desafio-Inf.pdf",
		dup + "074-Guerras-Secretas-Desafio-Inf.pdf", // cópia na árvore duplicada: mesma ordem
		d + "130-Guerras-Secretas-Desafio-Inf.pdf",
		d + "181-Guerras-Secretas-Desafio-Inf.pdf",
		d + "203 Guerras Secretas - Desafio Infinito V2 005.pdf",
		"Marvel/05 - Mundo de Guerra/Desafio Infinito/207-Guerras-Secretas-Desafio-Inf.pdf",
	}
	gs := make([]Guess, len(paths))
	for i, p := range paths {
		gs[i] = ParseName(p)
	}
	InferNumbers(gs)
	want := []string{"1", "2", "2", "3", "4", "5", "6"}
	for i, g := range gs {
		if g.Number != want[i] || (i != 5 && !g.NumberInferred) || (i == 5 && g.NumberInferred) {
			t.Errorf("%s: número %q inferido=%v, quer %q", paths[i], g.Number, g.NumberInferred, want[i])
		}
	}

	// Número conhecido fora da ordem: não infere nada.
	bad := []Guess{ParseName("Serie Truncada/010-Serie-Truncada.pdf"), ParseName("Serie Truncada/020-Serie-Truncada-007.pdf")}
	InferNumbers(bad)
	if bad[0].Number != "" {
		t.Errorf("não deveria inferir com números inconsistentes: %+v", bad[0])
	}
}
