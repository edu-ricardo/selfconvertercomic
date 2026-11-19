// Package convert transforma PDF de quadrinho em CBZ sem recompressão das páginas.
//
// Para cada página: se ela é uma única imagem JPEG/PNG ocupando a página inteira, os bytes da imagem
// são copiados como estão; senão a página é renderizada com o pdftoppm (Poppler). O CBZ é escrito num
// arquivo temporário oculto no mesmo diretório, validado e só então renomeado para o nome final.
package convert

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // DecodeConfig de JPEG
	_ "image/png"  // DecodeConfig de PNG
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func init() {
	// Sem isso o pdfcpu tenta criar $HOME/.config/pdfcpu (inexistente no container) e entra em panic.
	api.DisableConfigDir()
}

// ErrExists indica que o CBZ de destino já existe (nunca sobrescrevemos).
var ErrExists = errors.New("o arquivo de destino já existe")

// Result descreve uma conversão concluída.
type Result struct {
	Pages     int    `json:"pages"`
	Extracted int    `json:"extracted"` // páginas copiadas sem recompressão
	Rendered  int    `json:"rendered"`  // páginas renderizadas pelo pdftoppm
	Bytes     int64  `json:"bytes"`
	Title     string `json:"title"`
}

// Converter guarda as opções da conversão.
type Converter struct {
	// Pdftoppm é o executável usado no fallback (padrão: "pdftoppm" no PATH).
	Pdftoppm string
	// TargetHeight é a altura, em pixels, das páginas renderizadas (padrão 2400).
	TargetHeight int
}

// TempPath devolve o arquivo temporário usado para dst (oculto, ignorado pela varredura).
func TempPath(dst string) string {
	return filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".qt-tmp")
}

// Convert converte src (PDF) em dst (CBZ). progress, se não nulo, recebe (páginas prontas, total).
func (cv Converter) Convert(ctx context.Context, src, dst string, progress func(done, total int)) (res Result, err error) {
	if _, err := os.Stat(dst); err == nil {
		return res, ErrExists
	}

	f, err := os.Open(src)
	if err != nil {
		return res, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	pdf, err := api.ReadValidateAndOptimize(ctx, f, conf, nil)
	if err != nil {
		return res, fmt.Errorf("não foi possível ler o PDF (arquivo corrompido ou incompleto?): %w", err)
	}
	res.Pages = pdf.PageCount
	if res.Pages == 0 {
		return res, errors.New("o PDF não tem páginas")
	}
	dims, err := pdf.PageDims(ctx)
	if err != nil {
		return res, fmt.Errorf("lendo tamanho das páginas: %w", err)
	}

	tmp := TempPath(dst)
	_ = os.Remove(tmp) // sobra de uma execução interrompida
	out, err := os.Create(tmp)
	if err != nil {
		return res, err
	}
	defer func() {
		if out != nil {
			out.Close()
		}
		if err != nil {
			os.Remove(tmp)
		}
	}()

	zw := zip.NewWriter(out)
	digits := len(fmt.Sprint(res.Pages))
	if digits < 3 {
		digits = 3
	}

	for p := 1; p <= res.Pages; p++ {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		data, ext, ok := extractPage(ctx, pdf, p, dims[p-1].Width, dims[p-1].Height)
		if ok {
			res.Extracted++
		} else {
			if data, err = cv.render(ctx, src, p, dims[p-1].Height); err != nil {
				return res, fmt.Errorf("página %d: %w", p, err)
			}
			ext = "jpg"
			res.Rendered++
		}
		// Imagens já são comprimidas: Store evita gastar CPU recomprimindo.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("%0*d.%s", digits, p, ext), Method: zip.Store})
		if err != nil {
			return res, err
		}
		if _, err := w.Write(data); err != nil {
			return res, err
		}
		if progress != nil {
			progress(p, res.Pages)
		}
	}

	res.Title = strings.TrimSpace(pdf.Title)
	if res.Title == "" {
		res.Title = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	if err := writeComicInfo(zw, res, pdf); err != nil {
		return res, err
	}
	if err := zw.Close(); err != nil {
		return res, err
	}
	if err := out.Sync(); err != nil {
		return res, err
	}
	if err := out.Close(); err != nil {
		out = nil
		return res, err
	}
	out = nil

	if err := Validate(tmp, res.Pages); err != nil {
		return res, fmt.Errorf("CBZ gerado inválido: %w", err)
	}
	if _, err := os.Stat(dst); err == nil {
		return res, ErrExists // apareceu durante a conversão
	}
	if err := os.Rename(tmp, dst); err != nil {
		return res, err
	}
	if fi, err := os.Stat(dst); err == nil {
		res.Bytes = fi.Size()
	}
	return res, nil
}

// FirstPage devolve a imagem da primeira página do PDF (usada como capa): a imagem embutida quando
// possível, senão uma renderização em baixa resolução (altura ~targetHeight).
func (cv Converter) FirstPage(ctx context.Context, src string, targetHeight int) ([]byte, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	pdf, err := api.ReadValidateAndOptimize(ctx, f, conf, nil)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler o PDF (arquivo corrompido ou incompleto?): %w", err)
	}
	if pdf.PageCount == 0 {
		return nil, errors.New("o PDF não tem páginas")
	}
	dims, err := pdf.PageDims(ctx)
	if err != nil {
		return nil, err
	}
	if data, _, ok := extractPage(ctx, pdf, 1, dims[0].Width, dims[0].Height); ok {
		return data, nil
	}
	cv.TargetHeight = targetHeight
	return cv.render(ctx, src, 1, dims[0].Height)
}

// extractPage devolve os bytes originais da imagem quando a página é uma única imagem JPEG/PNG
// (RGB ou tons de cinza) com a mesma proporção da página.
func extractPage(ctx context.Context, pdf *model.Context, page int, pageW, pageH float64) ([]byte, string, bool) {
	imgs, err := pdfcpu.ExtractPageImages(ctx, pdf, page, false)
	if err != nil {
		return nil, "", false
	}
	var only *model.Image
	for _, im := range imgs {
		if im.Thumb || im.IsImgMask {
			continue
		}
		if only != nil {
			return nil, "", false // mais de uma imagem: composição, precisa renderizar
		}
		im := im
		only = &im
	}
	if only == nil || only.HasSMask || (only.FileType != "jpg" && only.FileType != "png") {
		return nil, "", false
	}
	data, err := io.ReadAll(only.Reader)
	if err != nil {
		return nil, "", false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.ColorModel == color.CMYKModel {
		return nil, "", false // CMYK costuma aparecer invertido nos leitores
	}
	if pageW <= 0 || pageH <= 0 || math.Abs(float64(cfg.Width)/float64(cfg.Height)-pageW/pageH) > 0.03*pageW/pageH {
		return nil, "", false // imagem não cobre a página (há texto/vetor em volta)
	}
	return data, only.FileType, true
}

func (cv Converter) render(ctx context.Context, src string, page int, pageHeightPt float64) ([]byte, error) {
	bin := cv.Pdftoppm
	if bin == "" {
		bin = "pdftoppm"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, errors.New("a página precisa ser renderizada, mas o pdftoppm não está instalado")
	}
	target := cv.TargetHeight
	if target == 0 {
		target = 2400
	}
	dpi := 150
	if pageHeightPt > 0 {
		dpi = int(math.Round(float64(target) / (pageHeightPt / 72)))
	}
	dpi = min(max(dpi, 72), 400)

	dir, err := os.MkdirTemp("", "qt-render-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "p")
	cmd := exec.CommandContext(ctx, bin, "-f", fmt.Sprint(page), "-l", fmt.Sprint(page),
		"-r", fmt.Sprint(dpi), "-jpeg", "-jpegopt", "quality=90", "-singlefile", src, prefix)
	if outp, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm falhou: %v: %s", err, strings.TrimSpace(string(outp)))
	}
	return os.ReadFile(prefix + ".jpg")
}

// ComicInfo é o ComicInfo.xml (esquema 2.0, o que Kavita e BookOrbit leem). Campos vazios ficam de fora.
type ComicInfo struct {
	XMLName     xml.Name `xml:"ComicInfo"`
	XSI         string   `xml:"xmlns:xsi,attr"`
	XSD         string   `xml:"xmlns:xsd,attr"`
	Title       string   `xml:"Title,omitempty"`
	Series      string   `xml:"Series,omitempty"`
	Number      string   `xml:"Number,omitempty"`
	Count       int      `xml:"Count,omitempty"`
	Volume      int      `xml:"Volume,omitempty"`
	Summary     string   `xml:"Summary,omitempty"`
	Notes       string   `xml:"Notes,omitempty"`
	Year        int      `xml:"Year,omitempty"`
	Month       int      `xml:"Month,omitempty"`
	Writer      string   `xml:"Writer,omitempty"`
	Penciller   string   `xml:"Penciller,omitempty"`
	Inker       string   `xml:"Inker,omitempty"`
	Colorist    string   `xml:"Colorist,omitempty"`
	Letterer    string   `xml:"Letterer,omitempty"`
	CoverArtist string   `xml:"CoverArtist,omitempty"`
	Publisher   string   `xml:"Publisher,omitempty"`
	Tags        string   `xml:"Tags,omitempty"`
	Web         string   `xml:"Web,omitempty"`
	PageCount   int      `xml:"PageCount"`
	LanguageISO string   `xml:"LanguageISO,omitempty"`
}

func encodeComicInfo(w io.Writer, ci ComicInfo) error {
	ci.XSI, ci.XSD = "http://www.w3.org/2001/XMLSchema-instance", "http://www.w3.org/2001/XMLSchema"
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(ci)
}

func writeComicInfo(zw *zip.Writer, res Result, pdf *model.Context) error {
	w, err := zw.Create("ComicInfo.xml")
	if err != nil {
		return err
	}
	return encodeComicInfo(w, ComicInfo{
		Title:     res.Title,
		Summary:   strings.TrimSpace(pdf.Subject),
		Writer:    strings.TrimSpace(pdf.Author),
		Tags:      strings.TrimSpace(pdf.Keywords),
		PageCount: res.Pages,
		Notes:     "Convertido de PDF pela QuadrinhoTeca.",
	})
}

// WriteComicInfo troca o ComicInfo.xml de um CBZ existente: as outras entradas são copiadas sem
// descompactar (páginas idênticas byte a byte), num temporário do mesmo disco renomeado no fim.
// PageCount é a quantidade de entradas além do ComicInfo.xml.
func WriteComicInfo(path string, ci ComicInfo) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	tmp := TempPath(path)
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			out.Close()
			os.Remove(tmp)
		}
	}()
	zw := zip.NewWriter(out)
	ci.PageCount = 0
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, "ComicInfo.xml") {
			continue
		}
		if err := zw.Copy(f); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if !f.FileInfo().IsDir() {
			ci.PageCount++
		}
	}
	w, err := zw.Create("ComicInfo.xml")
	if err != nil {
		return err
	}
	if err := encodeComicInfo(w, ci); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	zr.Close()
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// Validate confere que o CBZ abre, tem exatamente pages imagens e que cada uma é JPEG ou PNG.
func Validate(path string, pages int) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if f.Name == "ComicInfo.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		head := make([]byte, 512)
		k, _ := io.ReadFull(rc, head)
		rc.Close()
		switch http.DetectContentType(head[:k]) {
		case "image/jpeg", "image/png":
			n++
		default:
			return fmt.Errorf("%s não é uma imagem JPEG/PNG", f.Name)
		}
	}
	if n != pages {
		return fmt.Errorf("%d imagens no CBZ, o PDF tem %d páginas", n, pages)
	}
	return nil
}
