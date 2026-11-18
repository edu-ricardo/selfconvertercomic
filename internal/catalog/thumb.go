package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // capas em GIF dentro de CBZ
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/edu-ricardo/selfconvertercomic/internal/convert"
	"github.com/nwaples/rardecode/v2"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // capas em WebP dentro de CBZ
)

// ThumbWidth é a largura das miniaturas, em pixels.
const ThumbWidth = 300

// maxPageBytes limita a página lida de um arquivo compactado (proteção contra arquivos malformados).
const maxPageBytes = 64 << 20

// Thumbnailer gera e guarda em cache as capas (primeira página) dos arquivos.
type Thumbnailer struct {
	Dir  string // pasta do cache (ex.: /data/cache/thumbs)
	Conv convert.Converter

	once     sync.Once
	sem      chan struct{}
	mu       sync.Mutex
	inflight map[string]*thumbCall
}

type thumbCall struct {
	done chan struct{}
	err  error
}

// Path devolve o arquivo JPEG da capa, gerando-o se preciso. O nome inclui tamanho e mtime, então um
// arquivo alterado gera capa nova e a antiga deixa de ser usada.
func (t *Thumbnailer) Path(ctx context.Context, f File, absPath string) (string, error) {
	t.once.Do(func() {
		t.sem = make(chan struct{}, 2) // no máximo 2 capas geradas ao mesmo tempo
		t.inflight = map[string]*thumbCall{}
	})
	out := filepath.Join(t.Dir, fmt.Sprintf("%d-%d-%d.jpg", f.ID, f.Size, f.Mtime))
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	t.mu.Lock()
	if c, ok := t.inflight[out]; ok { // outro pedido já está gerando a mesma capa
		t.mu.Unlock()
		select {
		case <-c.done:
			return out, c.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	c := &thumbCall{done: make(chan struct{})}
	t.inflight[out] = c
	t.mu.Unlock()

	c.err = t.generate(ctx, f.Format, absPath, out)
	close(c.done)
	t.mu.Lock()
	delete(t.inflight, out)
	idle := len(t.inflight) == 0
	t.mu.Unlock()
	if idle {
		// Cada capa decodifica a página inteira (~10 MB); devolve a memória quando não há mais nenhuma
		// sendo gerada, para a app voltar ao consumo de repouso.
		debug.FreeOSMemory()
	}
	return out, c.err
}

func (t *Thumbnailer) generate(ctx context.Context, format, src, out string) error {
	select {
	case t.sem <- struct{}{}:
		defer func() { <-t.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	data, err := t.firstPage(ctx, format, src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decodificando a capa: %w", err)
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return errors.New("capa vazia")
	}
	w := min(ThumbWidth, b.Dx())
	h := b.Dy() * w / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)

	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(t.Dir, ".thumb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := jpeg.Encode(tmp, dst, &jpeg.Options{Quality: 80}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), out)
}

func (t *Thumbnailer) firstPage(ctx context.Context, format, src string) ([]byte, error) {
	switch format {
	case "pdf":
		return t.Conv.FirstPage(ctx, src, 2*ThumbWidth*3/2)
	case "cbz", "cbr":
		// Muitos .cbr são ZIP renomeados (e vice-versa): tenta os dois.
		first, second := firstInZip, firstInRar
		if format == "cbr" {
			first, second = firstInRar, firstInZip
		}
		data, err := first(src)
		if err == nil {
			return data, nil
		}
		if data, err2 := second(src); err2 == nil {
			return data, nil
		}
		return nil, err
	}
	return nil, fmt.Errorf("formato sem capa: %s", format)
}

func isPage(name string) bool {
	base := path.Base(name)
	if strings.HasPrefix(base, ".") || strings.Contains(name, "__MACOSX") {
		return false
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}

// firstName escolhe a primeira página pela ordem do nome (como os leitores fazem).
func firstName(names []string) (string, error) {
	if len(names) == 0 {
		return "", errors.New("nenhuma imagem no arquivo")
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names[0], nil
}

func firstInZip(src string) ([]byte, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var names []string
	byName := map[string]*zip.File{}
	for _, f := range zr.File {
		if isPage(f.Name) {
			names = append(names, f.Name)
			byName[f.Name] = f
		}
	}
	name, err := firstName(names)
	if err != nil {
		return nil, err
	}
	rc, err := byName[name].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxPageBytes))
}

func firstInRar(src string) ([]byte, error) {
	// RAR é sequencial: uma passada para listar, outra para ler a página escolhida.
	var names []string
	if err := eachRar(src, func(h *rardecode.FileHeader, _ io.Reader) (bool, error) {
		if !h.IsDir && isPage(h.Name) {
			names = append(names, h.Name)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	name, err := firstName(names)
	if err != nil {
		return nil, err
	}
	var data []byte
	err = eachRar(src, func(h *rardecode.FileHeader, r io.Reader) (bool, error) {
		if h.Name != name {
			return false, nil
		}
		data, err = io.ReadAll(io.LimitReader(r, maxPageBytes))
		return true, err
	})
	return data, err
}

func eachRar(src string, fn func(*rardecode.FileHeader, io.Reader) (stop bool, err error)) error {
	rc, err := rardecode.OpenReader(src)
	if err != nil {
		return err
	}
	defer rc.Close()
	for {
		h, err := rc.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if stop, err := fn(h, rc); stop || err != nil {
			return err
		}
	}
}
