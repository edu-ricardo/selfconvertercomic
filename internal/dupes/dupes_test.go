package dupes

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

func TestFindDuplicates(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	root := t.TempDir()
	const gs = "Marvel/guerras_secretas/1984/"
	files := map[string]string{
		gs + "Capítulo 01.pdf":                  "pdf-1", // árvore duplicada: mesmo conteúdo
		"arquivospdf/" + gs + "Capítulo 01.pdf": "pdf-1",
		gs + "Capítulo 01.cbz":                  "cbz-1",  // mesma edição, mesma pasta: não é duplicado
		gs + "Capítulo 02.pdf":                  "pdf-2",  // só existe uma vez
		"arquivospdf/" + gs + "Capítulo 02.pdf": "pdf-2",  // cópia exata da edição inteira: só conteúdo idêntico
		"HD1/Seda/021-Seda-07.cbz":              "seda-a", // mesma edição, conteúdos diferentes
		"HD2/Seda/Seda 007.cbz":                 "seda-b",
		"vazios/a.cbz":                          "", // dois arquivos vazios contam como conteúdo igual
		"vazios/b.cbr":                          "",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	libs := library.NewStore(conn)
	lib, _ := libs.Create(context.Background(), "HD", root, false)
	if _, err := libs.Scan(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if r, _ := Find(ctx, conn, 50); r.Pending != len(files) || len(r.Groups) != 3 { // sem hash ainda: só as edições
		t.Fatalf("antes do hash: %+v", r)
	}

	h := &Hasher{DB: conn}
	for {
		did, err := h.Step(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !did {
			break
		}
	}
	r, err := Find(ctx, conn, 50)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range r.Groups {
		var paths []string
		for _, f := range g.Files {
			paths = append(paths, f.RelPath)
		}
		sort.Strings(paths)
		got = append(got, g.Kind+" "+g.Label+": "+strings.Join(paths, " | "))
	}
	want := []string{
		"content Conteúdo idêntico · 0 KB: Marvel/guerras_secretas/1984/Capítulo 01.pdf | arquivospdf/Marvel/guerras_secretas/1984/Capítulo 01.pdf",
		"content Conteúdo idêntico · 0 KB: Marvel/guerras_secretas/1984/Capítulo 02.pdf | arquivospdf/Marvel/guerras_secretas/1984/Capítulo 02.pdf",
		"content Conteúdo idêntico · 0 KB: vazios/a.cbz | vazios/b.cbr",
		"edition Guerras Secretas #1 (1984): Marvel/guerras_secretas/1984/Capítulo 01.cbz | Marvel/guerras_secretas/1984/Capítulo 01.pdf | arquivospdf/Marvel/guerras_secretas/1984/Capítulo 01.pdf",
		"edition Seda #7: HD1/Seda/021-Seda-07.cbz | HD2/Seda/Seda 007.cbz",
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") || r.Pending != 0 {
		t.Fatalf("grupos (pendentes %d):\n%s\nquer:\n%s", r.Pending, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Arquivo alterado: o hash é refeito; a cópia deixa de ser idêntica.
	p := filepath.Join(root, "arquivospdf", filepath.FromSlash(gs), "Capítulo 01.pdf")
	os.WriteFile(p, []byte("outro conteúdo"), 0o644)
	os.Chtimes(p, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	libs.Scan(ctx, lib)
	if r, _ := Find(ctx, conn, 50); r.Pending != 1 {
		t.Fatalf("arquivo alterado deveria voltar a pendente: %d", r.Pending)
	}
	h.Step(ctx)
	r, _ = Find(ctx, conn, 50)
	for _, g := range r.Groups {
		if g.Kind == "content" && strings.Contains(g.Files[0].RelPath, "Capítulo 01") {
			t.Errorf("não deveria mais ser conteúdo idêntico: %+v", g)
		}
	}
	if r, _ := Find(ctx, conn, 1); len(r.Groups) != 1 || !r.More {
		t.Errorf("limite: %d grupos, more=%v", len(r.Groups), r.More)
	}
}
