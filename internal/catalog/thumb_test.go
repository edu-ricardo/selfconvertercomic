package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

func solid(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

func writeCBZ(t *testing.T, path string, pages map[string]image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, img := range pages {
		w, _ := zw.Create(name)
		png.Encode(w, img)
	}
	zw.Create("ComicInfo.xml")
	zw.Close()
	f.Close()
}

func decodeThumb(t *testing.T, p string) image.Image {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xc000 && g < 0x4000 && b < 0x4000
}

func TestThumbnailCBZUsesFirstPageByName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.cbz")
	writeCBZ(t, src, map[string]image.Image{
		"pagina/002.png":   solid(600, 900, color.RGBA{0, 0, 255, 255}),
		"pagina/001.png":   solid(600, 900, color.RGBA{255, 0, 0, 255}), // capa
		"__MACOSX/000.png": solid(10, 10, color.Black),
	})
	th := &Thumbnailer{Dir: filepath.Join(dir, "cache")}
	f := File{ID: 1, Format: "cbz", Size: 10, Mtime: 20}

	p, err := th.Path(context.Background(), f, src)
	if err != nil {
		t.Fatal(err)
	}
	img := decodeThumb(t, p)
	if img.Bounds().Dx() != ThumbWidth || img.Bounds().Dy() != 450 || !isRed(img.At(150, 225)) {
		t.Fatalf("capa inesperada: %v", img.Bounds())
	}

	// Segunda chamada usa o cache (não regera).
	info1, _ := os.Stat(p)
	p2, _ := th.Path(context.Background(), f, src)
	info2, _ := os.Stat(p2)
	if p2 != p || !info2.ModTime().Equal(info1.ModTime()) {
		t.Fatal("deveria reaproveitar a miniatura em cache")
	}

	// Arquivo alterado (mtime diferente) gera outra miniatura.
	f.Mtime = 21
	if p3, _ := th.Path(context.Background(), f, src); p3 == p {
		t.Fatal("arquivo alterado deveria gerar nova miniatura")
	}
}

func TestThumbnailCBRThatIsReallyZip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "b.cbr")
	writeCBZ(t, src, map[string]image.Image{"01.png": solid(400, 600, color.RGBA{255, 0, 0, 255})})
	th := &Thumbnailer{Dir: dir}
	p, err := th.Path(context.Background(), File{ID: 2, Format: "cbr", Size: 1, Mtime: 1}, src)
	if err != nil {
		t.Fatal(err)
	}
	if !isRed(decodeThumb(t, p).At(10, 10)) {
		t.Fatal("capa errada")
	}
}

func TestThumbnailPDFAndConcurrency(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	jpeg.Encode(&buf, solid(800, 1200, color.RGBA{255, 0, 0, 255}), nil)
	var out bytes.Buffer
	if err := api.ImportImages(context.Background(), nil, &out, []io.Reader{&buf}, pdfcpu.DefaultImportConfig(), nil); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "c.pdf")
	os.WriteFile(src, out.Bytes(), 0o644)

	th := &Thumbnailer{Dir: filepath.Join(dir, "cache")}
	f := File{ID: 3, Format: "pdf", Size: 1, Mtime: 1}
	var wg sync.WaitGroup
	paths := make([]string, 8)
	errs := make([]error, 8)
	for i := range 8 { // vários pedidos da mesma capa ao mesmo tempo
		wg.Add(1)
		go func() {
			defer wg.Done()
			paths[i], errs[i] = th.Path(context.Background(), f, src)
		}()
	}
	wg.Wait()
	for i := range 8 {
		if errs[i] != nil || paths[i] != paths[0] {
			t.Fatalf("pedido %d: %q %v", i, paths[i], errs[i])
		}
	}
	img := decodeThumb(t, paths[0])
	if img.Bounds().Dx() != ThumbWidth || !isRed(img.At(150, 200)) {
		t.Fatalf("capa do PDF: %v", img.Bounds())
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "cache"))
	if len(entries) != 1 {
		t.Fatalf("esperava 1 arquivo no cache, há %d", len(entries))
	}
}

func TestThumbnailInvalidArchive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ruim.cbz")
	os.WriteFile(src, []byte("não é zip"), 0o644)
	th := &Thumbnailer{Dir: dir}
	if _, err := th.Path(context.Background(), File{ID: 4, Format: "cbz"}, src); err == nil {
		t.Fatal("esperava erro")
	}
}
