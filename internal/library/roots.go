package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrOutsideRoots indica um caminho fora das raízes permitidas (/external, /storage...).
var ErrOutsideRoots = errors.New("caminho fora das pastas permitidas")

// Roots são as pastas montadas no container sob as quais as bibliotecas podem ficar.
type Roots []string

// ParseRoots lê a lista separada por vírgulas (QT_LIBRARY_ROOTS), limpando cada caminho.
func ParseRoots(s string) Roots {
	var r Roots
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			r = append(r, filepath.Clean(p))
		}
	}
	return r
}

// Resolve limpa p, resolve links simbólicos e confere que o resultado está dentro de uma raiz.
func (r Roots) Resolve(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: informe um caminho absoluto", ErrOutsideRoots)
	}
	clean := filepath.Clean(p)
	if !r.contains(clean) {
		return "", ErrOutsideRoots
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", err
	}
	if !r.contains(real) {
		return "", fmt.Errorf("%w: link simbólico aponta para fora", ErrOutsideRoots)
	}
	return clean, nil
}

func (r Roots) contains(p string) bool {
	for _, root := range r {
		real := root
		if rr, err := filepath.EvalSymlinks(root); err == nil {
			real = rr
		}
		for _, base := range []string{root, real} {
			if p == base || strings.HasPrefix(p, strings.TrimSuffix(base, string(filepath.Separator))+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

// Dir é uma subpasta listada no navegador de pastas.
type Dir struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Listing é o conteúdo de uma pasta no navegador. Parent vazio = voltar às raízes.
type Listing struct {
	Path   string `json:"path"`
	Parent string `json:"parent"`
	Dirs   []Dir  `json:"dirs"`
}

// List devolve as subpastas de p; com p vazio, as raízes que existem.
func (r Roots) List(p string) (Listing, error) {
	if p == "" {
		l := Listing{Dirs: []Dir{}}
		for _, root := range r {
			if fi, err := os.Stat(root); err == nil && fi.IsDir() {
				l.Dirs = append(l.Dirs, Dir{Name: root, Path: root})
			}
		}
		return l, nil
	}
	clean, err := r.Resolve(p)
	if err != nil {
		return Listing{}, err
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return Listing{}, err
	}
	l := Listing{Path: clean, Dirs: []Dir{}}
	if parent := filepath.Dir(clean); parent != clean && r.contains(parent) {
		l.Parent = parent
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			l.Dirs = append(l.Dirs, Dir{Name: e.Name(), Path: filepath.Join(clean, e.Name())})
		}
	}
	sort.Slice(l.Dirs, func(i, j int) bool { return strings.ToLower(l.Dirs[i].Name) < strings.ToLower(l.Dirs[j].Name) })
	return l, nil
}
