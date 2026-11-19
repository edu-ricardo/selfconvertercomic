package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSameDisk(t *testing.T) {
	d := t.TempDir()
	src, dst := filepath.Join(d, "a", "x.cbz"), filepath.Join(d, "b", "c", "y.cbz")
	write(t, src, "conteúdo")
	if err := Move(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "conteúdo" {
		t.Errorf("destino: %q", b)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("origem deveria sumir")
	}

	// Nunca sobrescreve.
	write(t, src, "outro")
	if err := Move(src, dst); !errors.Is(err, ErrExists) {
		t.Errorf("destino existente: %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "conteúdo" {
		t.Error("o destino foi sobrescrito")
	}
}

// Entre discos (rename devolve EXDEV): copia, confere e apaga a origem; sem sobras.
func TestMoveAcrossDisks(t *testing.T) {
	rename = func(o, n string) error { return &os.LinkError{Op: "rename", Old: o, New: n, Err: syscall.EXDEV} }
	defer func() { rename = os.Rename }()
	d := t.TempDir()
	src, dst := filepath.Join(d, "hd1", "x.cbz"), filepath.Join(d, "hd2", "Série", "x.cbz")
	write(t, src, "páginas")
	if err := Move(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "páginas" {
		t.Errorf("destino: %q", b)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("origem deveria sumir depois da cópia conferida")
	}
	if entries, _ := os.ReadDir(filepath.Dir(dst)); len(entries) != 1 {
		t.Errorf("sobrou temporário: %v", entries)
	}
}

func TestMoveCaseOnly(t *testing.T) {
	d := t.TempDir()
	src, dst := filepath.Join(d, "guerras.cbz"), filepath.Join(d, "Guerras.cbz")
	write(t, src, "x")
	if err := Move(src, dst); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(d)
	if len(entries) != 1 || entries[0].Name() != "Guerras.cbz" {
		t.Errorf("depois de trocar a caixa: %v", entries)
	}
}

func TestRemoveEmptyDirs(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "keep.txt"), "x")
	os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755)
	RemoveEmptyDirs(root, filepath.Join(root, "a", "b", "c"))
	if _, err := os.Stat(filepath.Join(root, "a", "b")); !errors.Is(err, os.ErrNotExist) {
		t.Error("pastas vazias deveriam sumir")
	}
	if _, err := os.Stat(filepath.Join(root, "a", "keep.txt")); err != nil {
		t.Error("pasta com arquivo não pode sumir")
	}
	RemoveEmptyDirs(root, root)
	if _, err := os.Stat(root); err != nil {
		t.Error("a raiz nunca é apagada")
	}
}

// Dois arquivos que só diferem na caixa (possível em ext4): nunca sobrescreve o outro.
func TestMoveCaseOnlyOtherFile(t *testing.T) {
	d := t.TempDir()
	src, dst := filepath.Join(d, "x", "guerras.cbz"), filepath.Join(d, "y", "Guerras.cbz")
	write(t, src, "a")
	write(t, dst, "b")
	if err := Move(src, filepath.Join(d, "y", "guerras.cbz")); err == nil {
		if b, _ := os.ReadFile(dst); string(b) != "b" {
			t.Fatal("sobrescreveu um arquivo de outra caixa")
		}
	}
}
