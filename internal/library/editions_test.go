package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type editionView struct {
	series, number string
	inferred       bool
	files          int
}

func editionsOf(t *testing.T, s *Store, libID int64) map[string]editionView {
	t.Helper()
	rows, err := s.db.Query(`SELECT e.dir || '/' || e.stem, e.series, e.number, e.number_inferred,
		(SELECT count(*) FROM files f WHERE f.edition_id = e.id) FROM editions e WHERE e.library_id = ?`, libID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	m := map[string]editionView{}
	for rows.Next() {
		var k string
		var v editionView
		rows.Scan(&k, &v.series, &v.number, &v.inferred, &v.files)
		m[k] = v
	}
	return m
}

func TestScanEditions(t *testing.T) {
	s := newStore(t)
	root := t.TempDir()
	const d = "Marvel/Desafio Infinito/"
	write(t, filepath.Join(root, d+"034-Guerras-Secretas-Desafio-Inf.pdf"), "a")
	write(t, filepath.Join(root, d+"034-Guerras-Secretas-Desafio-Inf.cbz"), "a")
	write(t, filepath.Join(root, d+"074-Guerras-Secretas-Desafio-Inf.pdf"), "b")
	write(t, filepath.Join(root, d+"203 Guerras Secretas - Desafio Infinito V2 003.pdf"), "c")
	write(t, filepath.Join(root, "Marvel/guerras_secretas/1984/Capítulo 01.pdf"), "d")
	lib, err := s.Create(context.Background(), "HD", root, false)
	if err != nil {
		t.Fatal(err)
	}
	scan(t, s, lib)

	got := editionsOf(t, s, lib.ID)
	want := map[string]editionView{
		"Marvel/Desafio Infinito/034-Guerras-Secretas-Desafio-Inf":               {"Desafio Infinito", "1", true, 2},
		"Marvel/Desafio Infinito/074-Guerras-Secretas-Desafio-Inf":               {"Desafio Infinito", "2", true, 1},
		"Marvel/Desafio Infinito/203 Guerras Secretas - Desafio Infinito V2 003": {"Desafio Infinito", "3", false, 1},
		"Marvel/guerras_secretas/1984/Capítulo 01":                               {"Guerras Secretas", "1", false, 1},
	}
	if len(got) != len(want) {
		t.Fatalf("edições: %+v", got)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %+v, quer %+v", k, got[k], w)
		}
	}

	// Some o PDF da edição 034: a edição continua (tem o CBZ). Some a 074 inteira: a edição é apagada e a
	// inferência é refeita (a 034 continua 1; a 203 passa a ser a 2ª da ordem, mas tem número 3 próprio,
	// então a inferência é desfeita para a série toda).
	os.Remove(filepath.Join(root, d+"034-Guerras-Secretas-Desafio-Inf.pdf"))
	os.Remove(filepath.Join(root, d+"074-Guerras-Secretas-Desafio-Inf.pdf"))
	scan(t, s, lib)
	got = editionsOf(t, s, lib.ID)
	if len(got) != 3 {
		t.Fatalf("edição sem arquivos deveria sumir: %+v", got)
	}
	if e := got[d+"034-Guerras-Secretas-Desafio-Inf"]; e.files != 1 || e.number != "" || e.inferred {
		t.Errorf("034 depois de perder a 074: %+v", e)
	}

	// Revarredura sem mudanças não reprocessa as edições (a marca abaixo sobrevive).
	s.db.Exec("UPDATE editions SET series = 'marca' WHERE library_id = ?", lib.ID)
	scan(t, s, lib)
	if e := editionsOf(t, s, lib.ID)[d+"034-Guerras-Secretas-Desafio-Inf"]; e.series != "marca" {
		t.Errorf("revarredura sem mudanças reprocessou as edições: %+v", e)
	}

	// Versão antiga do analisador: reanalisa na próxima varredura.
	s.db.Exec("UPDATE editions SET parser_version = 0 WHERE library_id = ?", lib.ID)
	scan(t, s, lib)
	if e := editionsOf(t, s, lib.ID)[d+"034-Guerras-Secretas-Desafio-Inf"]; e.series != "Desafio Infinito" {
		t.Errorf("edição de versão antiga não foi reanalisada: %+v", e)
	}
}
