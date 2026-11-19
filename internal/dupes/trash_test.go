package dupes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/edu-ricardo/selfconvertercomic/internal/db"
	"github.com/edu-ricardo/selfconvertercomic/internal/library"
)

func TestTrash(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "qt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	root := t.TempDir()
	const gs = "Marvel/guerras_secretas/1984/"
	files := map[string]string{
		gs + "Capítulo 01.pdf": "pdf-1", "arquivospdf/" + gs + "Capítulo 01.pdf": "pdf-1", // cópias
		gs + "Capítulo 02.pdf": "pdf-2", gs + "Capítulo 02.cbz": "cbz-2", // PDF já convertido
		gs + "Capítulo 03.pdf":     "pdf-3",                           // único
		"HD1/Seda/021-Seda-07.cbz": "a", "HD2/Seda/Seda 007.cbz": "b", // duas versões da mesma edição
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	libs := library.NewStore(conn)
	lib, _ := libs.Create(context.Background(), "HD", root, false)
	ctx := context.Background()
	scan := func() {
		if _, err := libs.Scan(ctx, lib); err != nil {
			t.Fatal(err)
		}
	}
	scan()
	h := &Hasher{DB: conn}
	for did, _ := h.Step(ctx); did; did, _ = h.Step(ctx) {
	}
	id := func(rel string) int64 {
		var id int64
		conn.QueryRow("SELECT id FROM files WHERE rel_path = ?", rel).Scan(&id)
		return id
	}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	tr := &Trash{DB: conn, Libraries: libs, Exclusive: func(_ int64, fn func() error) error { return fn() }}

	// PDFs que têm CBZ: só o Capítulo 02.
	pdfs, _ := tr.ConvertedPDFs(ctx)
	if len(pdfs) != 1 || pdfs[0].RelPath != gs+"Capítulo 02.pdf" {
		t.Fatalf("PDFs convertidos: %+v", pdfs)
	}

	// Nunca a última cópia: o único, as duas cópias juntas, as duas versões juntas.
	for name, ids := range map[string][]int64{
		"único":           {id(gs + "Capítulo 03.pdf")},
		"as duas cópias":  {id(gs + "Capítulo 01.pdf"), id("arquivospdf/" + gs + "Capítulo 01.pdf")},
		"PDF e CBZ":       {id(gs + "Capítulo 02.pdf"), id(gs + "Capítulo 02.cbz")},
		"as duas versões": {id("HD1/Seda/021-Seda-07.cbz"), id("HD2/Seda/Seda 007.cbz")},
	} {
		if _, err := tr.MoveToTrash(ctx, ids, "teste"); !errors.Is(err, ErrLastCopy) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if n, _ := tr.List(ctx); len(n) != 0 {
		t.Fatalf("recusas não podem mover nada: %+v", n)
	}

	// Permitidos: a cópia da árvore, o PDF convertido e uma das versões.
	n, err := tr.MoveToTrash(ctx, []int64{id("arquivospdf/" + gs + "Capítulo 01.pdf"), id(gs + "Capítulo 02.pdf"), id("HD2/Seda/Seda 007.cbz")}, "teste")
	if err != nil || n != 3 {
		t.Fatalf("mover para a lixeira: %d %v", n, err)
	}
	if exists("arquivospdf/"+gs+"Capítulo 01.pdf") || exists(gs+"Capítulo 02.pdf") || !exists(gs+"Capítulo 02.cbz") {
		t.Fatal("arquivos não foram movidos como esperado")
	}
	if exists("arquivospdf") || exists("HD2") {
		t.Error("pastas que ficaram vazias deveriam sumir")
	}
	// Antes da varredura: a outra cópia não pode ir junto (a que está na lixeira não conta).
	if _, err := tr.MoveToTrash(ctx, []int64{id(gs + "Capítulo 01.pdf")}, "teste"); !errors.Is(err, ErrLastCopy) {
		t.Errorf("última cópia depois da primeira ir para a lixeira: %v", err)
	}
	scan()
	if r, _ := libs.Scan(ctx, lib); r.Total != 4 { // 7 − 3 na lixeira
		t.Errorf("a lixeira (pasta oculta) não pode entrar no catálogo: %d arquivos", r.Total)
	}

	// Restaurar devolve ao lugar; esvaziar apaga de vez.
	items, _ := tr.List(ctx)
	if len(items) != 3 {
		t.Fatalf("lixeira: %+v", items)
	}
	var seda, pdf2 int64
	for _, it := range items {
		switch it.RelPath {
		case "HD2/Seda/Seda 007.cbz":
			seda = it.ID
		case gs + "Capítulo 02.pdf":
			pdf2 = it.ID
		}
	}
	if n, err := tr.Restore(ctx, []int64{seda}); err != nil || n != 1 || !exists("HD2/Seda/Seda 007.cbz") {
		t.Fatalf("restaurar: %d %v", n, err)
	}
	os.WriteFile(filepath.Join(root, filepath.FromSlash(gs+"Capítulo 02.pdf")), []byte("outro"), 0o644)
	if _, err := tr.Restore(ctx, []int64{pdf2}); !errors.Is(err, ErrOccupied) {
		t.Errorf("restaurar com o lugar ocupado: %v", err)
	}
	if n, err := tr.Empty(ctx, nil); err != nil || n != 2 {
		t.Fatalf("esvaziar: %d %v", n, err)
	}
	if exists(TrashDir) {
		t.Error("a pasta da lixeira vazia deveria sumir")
	}
	if items, _ := tr.List(ctx); len(items) != 0 {
		t.Errorf("lixeira depois de esvaziar: %+v", items)
	}
}
