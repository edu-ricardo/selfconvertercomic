// Package fsutil move arquivos com segurança entre pastas e discos (RF-43, RNF-04).
package fsutil

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrExists indica que o destino já existe (nunca sobrescrevemos).
var ErrExists = errors.New("o destino já existe")

// rename é trocável nos testes (para simular discos diferentes).
var rename = os.Rename

// Move leva src para dst, criando as pastas de dst. No mesmo disco é um rename; entre discos copia
// para um temporário oculto ao lado do destino, confere o SHA-256 contra a origem e só então apaga a
// origem. Recusa se dst já existir — a não ser que seja o próprio src com outra caixa ("guerras" →
// "Guerras"), que em exFAT/NTFS é o mesmo arquivo e é renomeado em dois passos.
func Move(src, dst string) error {
	if src == dst {
		return nil
	}
	caseOnly := strings.EqualFold(src, dst)
	if di, err := os.Lstat(dst); err == nil {
		// Só a caixa muda e o destino é o próprio arquivo (exFAT/NTFS): segue. Num disco que diferencia
		// maiúsculas, "Guerras.cbz" pode ser OUTRO arquivo: recusa como qualquer destino existente.
		si, serr := os.Lstat(src)
		if !caseOnly || serr != nil || !os.SameFile(si, di) {
			return fmt.Errorf("%w: %s", ErrExists, dst)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if caseOnly {
		tmp := filepath.Join(filepath.Dir(src), ".qt-move-"+filepath.Base(src))
		if err := rename(src, tmp); err != nil {
			return err
		}
		if err := rename(tmp, dst); err != nil {
			_ = rename(tmp, src)
			return err
		}
		return nil
	}
	err := rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	return copyVerifyRemove(src, dst)
}

func copyVerifyRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".qt-tmp")
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	srcHash := sha256.New()
	_, err = io.Copy(out, io.TeeReader(in, srcHash))
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		var dstSum []byte
		dstSum, err = fileHash(tmp)
		if err == nil && string(dstSum) != string(srcHash.Sum(nil)) {
			err = errors.New("a cópia não confere com a origem (SHA-256 diferente)")
		}
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	in.Close()
	return os.Remove(src)
}

func fileHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// RemoveEmptyDirs apaga dir e as pastas acima dele que ficaram vazias, sem passar de root.
func RemoveEmptyDirs(root, dir string) {
	root = filepath.Clean(root)
	for dir = filepath.Clean(dir); dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil { // não vazia (ou já sumiu): para
			return
		}
	}
}
