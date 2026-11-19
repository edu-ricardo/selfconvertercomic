package convert

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

// jpegPage gera um JPEG w×h com uma cor por página (para distinguir as páginas).
func jpegPage(t *testing.T, w, h int, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{shade, uint8(x % 256), uint8(y % 256), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// imagePDF cria um PDF com uma imagem JPEG por página (página = tamanho da imagem).
func imagePDF(t *testing.T, path string, pages [][]byte) {
	t.Helper()
	readers := make([]io.Reader, len(pages))
	for i, p := range pages {
		readers[i] = bytes.NewReader(p)
	}
	var out bytes.Buffer
	if err := api.ImportImages(context.Background(), nil, &out, readers, pdfcpu.DefaultImportConfig(), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// textPDF escreve à mão um PDF de uma página só com texto (sem imagem): exige renderização.
func textPDF(t *testing.T, path string) {
	t.Helper()
	content := "BT /F1 48 Tf 72 700 Td (Pagina de texto) Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	m := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		m[f.Name], _ = io.ReadAll(rc)
		rc.Close()
	}
	return m
}

// Critério do M3: páginas idênticas byte a byte às imagens embutidas no PDF.
func TestConvertCopiesImagesByteForByte(t *testing.T) {
	dir := t.TempDir()
	pages := [][]byte{jpegPage(t, 400, 600, 10), jpegPage(t, 400, 600, 120), jpegPage(t, 400, 615, 250)}
	src := filepath.Join(dir, "Guerras Secretas #01.pdf")
	imagePDF(t, src, pages)
	dst := filepath.Join(dir, "Guerras Secretas #01.cbz")

	var progress []int
	res, err := Converter{}.Convert(context.Background(), src, dst, func(done, total int) { progress = append(progress, done) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 3 || res.Extracted != 3 || res.Rendered != 0 || res.Title != "Guerras Secretas #01" {
		t.Fatalf("resultado: %+v", res)
	}
	if len(progress) != 3 || progress[2] != 3 {
		t.Fatalf("progresso: %v", progress)
	}

	files := readZip(t, dst)
	for i, want := range pages {
		got := files[fmt.Sprintf("%03d.jpg", i+1)]
		if !bytes.Equal(got, want) {
			t.Errorf("página %d difere da imagem original (%d vs %d bytes)", i+1, len(got), len(want))
		}
	}
	ci := string(files["ComicInfo.xml"])
	if !strings.Contains(ci, "<PageCount>3</PageCount>") || !strings.Contains(ci, "<Title>Guerras Secretas #01</Title>") {
		t.Errorf("ComicInfo.xml inesperado:\n%s", ci)
	}
	if _, err := os.Stat(TempPath(dst)); !os.IsNotExist(err) {
		t.Error("arquivo temporário não foi removido")
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("o conversor não deve mexer no PDF original")
	}
}

func TestConvertRendersTextPageWithPdftoppm(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm não instalado")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "texto.pdf")
	textPDF(t, src)
	dst := filepath.Join(dir, "texto.cbz")

	res, err := Converter{}.Convert(context.Background(), src, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 1 || res.Rendered != 1 {
		t.Fatalf("resultado: %+v", res)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(readZip(t, dst)["001.jpg"]))
	if err != nil || cfg.Height < 2000 {
		t.Fatalf("página renderizada: %+v, %v", cfg, err)
	}
}

func TestConvertWithoutPdftoppmFailsCleanly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "texto.pdf")
	textPDF(t, src)
	dst := filepath.Join(dir, "texto.cbz")

	_, err := Converter{Pdftoppm: "nao-existe-pdftoppm"}.Convert(context.Background(), src, dst, nil)
	if err == nil || !strings.Contains(err.Error(), "pdftoppm") {
		t.Fatalf("esperava erro do pdftoppm, veio %v", err)
	}
	for _, p := range []string{dst, TempPath(dst)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s não deveria existir após falha", p)
		}
	}
}

func TestConvertCancelledLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	imagePDF(t, src, [][]byte{jpegPage(t, 200, 300, 1), jpegPage(t, 200, 300, 2), jpegPage(t, 200, 300, 3)})
	dst := filepath.Join(dir, "a.cbz")

	ctx, cancel := context.WithCancel(context.Background())
	_, err := Converter{}.Convert(ctx, src, dst, func(done, _ int) {
		if done == 1 {
			cancel() // "queda" no meio da conversão
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, veio %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("só o PDF deveria restar; há %d arquivos", len(entries))
	}
}

func TestConvertNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	imagePDF(t, src, [][]byte{jpegPage(t, 100, 150, 1)})
	dst := filepath.Join(dir, "a.cbz")
	os.WriteFile(dst, []byte("existente"), 0o644)

	if _, err := (Converter{}).Convert(context.Background(), src, dst, nil); !errors.Is(err, ErrExists) {
		t.Fatalf("esperava ErrExists, veio %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "existente" {
		t.Fatal("CBZ existente foi alterado")
	}
}

// Amostra real (fora do git): samples/*.pdf na raiz do repositório.
func TestConvertRealSamples(t *testing.T) {
	samples, _ := filepath.Glob(filepath.Join("..", "..", "samples", "*.pdf"))
	if len(samples) == 0 {
		t.Skip("sem amostras em samples/")
	}
	for _, src := range samples {
		t.Run(filepath.Base(src), func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "out.cbz")
			res, err := Converter{}.Convert(context.Background(), src, dst, nil)
			if err != nil {
				t.Fatal(err)
			}
			fi, _ := os.Stat(src)
			t.Logf("%+v (PDF %d bytes)", res, fi.Size())
		})
	}
}

func TestWriteComicInfoKeepsPages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Capítulo 01.cbz")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	pages := map[string][]byte{}
	for i := range 3 {
		name := fmt.Sprintf("%03d.jpg", i+1)
		pages[name] = bytes.Repeat([]byte{0xFF, 0xD8, byte(i)}, 1000)
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		w.Write(pages[name])
	}
	w, _ := zw.Create("ComicInfo.xml")
	w.Write([]byte("<ComicInfo><Title>antigo</Title></ComicInfo>"))
	zw.Close()
	f.Close()
	before := zipEntries(t, path)

	ci := ComicInfo{Series: "Secret Wars (Guerras Secretas)", Number: "1", Title: "A Guerra Se Inicia", Year: 1986, Month: 8,
		Writer: "Jim Shooter", Penciller: "Mike Zeck", Publisher: "Editora Abril", LanguageISO: "pt",
		Web: "https://www.comics.org/issue/726769/", PageCount: 99}
	if err := WriteComicInfo(path, ci); err != nil {
		t.Fatal(err)
	}
	after := zipEntries(t, path)
	for name := range pages {
		if before[name] != after[name] {
			t.Errorf("%s mudou: %s → %s", name, before[name], after[name])
		}
	}
	info := readEntry(t, path, "ComicInfo.xml")
	for _, want := range []string{"<Series>Secret Wars (Guerras Secretas)</Series>", "<Number>1</Number>", "<Writer>Jim Shooter</Writer>",
		"<Penciller>Mike Zeck</Penciller>", "<LanguageISO>pt</LanguageISO>", "<PageCount>3</PageCount>", "<Month>8</Month>"} {
		if !strings.Contains(info, want) {
			t.Errorf("ComicInfo sem %s:\n%s", want, info)
		}
	}
	if strings.Contains(info, "antigo") || len(after) != 4 {
		t.Errorf("ComicInfo antigo deveria ser trocado: %d entradas\n%s", len(after), info)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("sobrou temporário: %v", entries)
	}
}

// zipEntries resume cada entrada (método, CRC e tamanho compactado): iguais = bytes idênticos.
func zipEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	m := map[string]string{}
	for _, f := range zr.File {
		m[f.Name] = fmt.Sprintf("%d/%08x/%d", f.Method, f.CRC32, f.CompressedSize64)
	}
	return m
}

func readEntry(t *testing.T, path, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatalf("%s sem %s", path, name)
	return ""
}
