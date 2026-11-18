package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootsResolve(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "external")
	outside := filepath.Join(base, "secreto")
	for _, d := range []string{filepath.Join(root, "HD", "comics"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r := ParseRoots(" " + root + " ,")

	if _, err := r.Resolve(filepath.Join(root, "HD", "comics")); err != nil {
		t.Fatalf("dentro da raiz: %v", err)
	}
	for _, p := range []string{
		outside,
		filepath.Join(root, "..", "secreto"),
		root + "-irmao",
		"relativo/comics",
	} {
		if _, err := r.Resolve(p); !errors.Is(err, ErrOutsideRoots) {
			t.Errorf("%s: esperava ErrOutsideRoots, veio %v", p, err)
		}
	}

	link := filepath.Join(root, "atalho")
	if err := os.Symlink(outside, link); err == nil { // no Windows pode exigir privilégio
		if _, err := r.Resolve(link); !errors.Is(err, ErrOutsideRoots) {
			t.Errorf("link para fora: esperava ErrOutsideRoots, veio %v", err)
		}
	}
}

func TestRootsList(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"Marvel", "dc", ".oculta"} {
		os.Mkdir(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "arquivo.pdf"), nil, 0o644)
	r := Roots{root, filepath.Join(root, "nao-existe")}

	top, err := r.List("")
	if err != nil || len(top.Dirs) != 1 || top.Dirs[0].Path != root {
		t.Fatalf("raízes: %+v, %v", top, err)
	}
	l, err := r.List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Dirs) != 2 || l.Dirs[0].Name != "dc" || l.Dirs[1].Name != "Marvel" || l.Parent != "" {
		t.Fatalf("listagem inesperada: %+v", l)
	}
	sub, _ := r.List(filepath.Join(root, "Marvel"))
	if sub.Parent != root {
		t.Fatalf("parent = %q, quer %q", sub.Parent, root)
	}
}
