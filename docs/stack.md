# Opções de stack

Critérios: baixo consumo de RAM/CPU, imagem Docker pequena, conversão rápida de PDF, multi-arquitetura
(amd64/arm64 — Umbrel roda em ambos), manutenção simples.

O gargalo real é a **conversão PDF → imagens**: qualquer opção usa uma biblioteca nativa
(MuPDF ou Poppler). A escolha da linguagem afeta principalmente memória ociosa, tamanho da imagem e
produtividade.

| Opção | Backend | Frontend | RAM ociosa (aprox.) | Imagem | Prós | Contras |
|---|---|---|---|---|---|---|
| **A. Go** (recomendada) | Go + `go-fitz` (MuPDF) ou `pdftoppm`; SQLite | SvelteKit (estático) ou HTMX | ~15–30 MB | ~30–60 MB | Binário único, concorrência nativa para filas de conversão, multi-arch fácil | CGO ao usar MuPDF embutido |
| B. Rust | Rust (Axum) + `pdfium`/`mupdf` bindings; SQLite | SvelteKit (estático) | ~5–20 MB | ~20–50 MB | Mais leve e rápido de todos | Desenvolvimento mais lento, compilação pesada |
| C. Python | FastAPI + PyMuPDF; SQLite | SvelteKit ou Vue | ~60–120 MB | ~150–250 MB | PyMuPDF excelente, ecossistema de metadados/scraping | Mais memória, imagem maior |
| D. Node/Bun | Bun/Hono + `pdftoppm`/mupdf CLI; SQLite | SvelteKit (full-stack) | ~50–90 MB | ~100–150 MB | Uma linguagem só (TS) no front e back | Mais pesado que Go/Rust; PDF via processo externo |

Frontend: **SvelteKit gerado como estático** e servido pelo próprio backend é a opção mais leve com
visual moderno (sem servidor Node em produção). Design system sugerido: Tailwind CSS + componentes
próprios (ou shadcn-svelte) para manter padronização.

Formato de saída: **CBZ** (ZIP) é aberto e suportado universalmente; CBR (RAR) exige ferramenta
proprietária para *criar* arquivos — recomendação é gerar só CBZ.

**Decisão (2026-11-18): Opção A — Go no backend + SvelteKit estático no frontend, saída somente CBZ.**
Design system: **shadcn-svelte** (decidido). Biblioteca de PDF: MuPDF/`go-fitz` descartado por ser AGPL-3.0 + CGO; **pdfcpu** (Go puro, Apache-2.0) + `pdftoppm` como fallback — decidido após o teste abaixo.

## Teste do pdfcpu (2026-11-18)

pdfcpu v0.16.4 (CLI), amostra `Vingadores Mundiais #21.pdf` (17,5 MB, 24 páginas, PDF 1.4, gerado por adultpdf):

| Verificação | Resultado |
|---|---|
| Estrutura | 1 imagem por página, JPEG (`DCTDecode`), 1280×1968, tamanho da página = tamanho da imagem |
| Metadados internos | Título/autor/assunto **vazios** (só produtor e data) — identificação dependerá de nome e pastas |
| Extração `pdfcpu extract -m image` | 24 arquivos, **0,16 s**, todos JPEG válidos |
| Perda | Nenhuma: imagens somam 17,48 MB vs. PDF 17,50 MB (fluxo JPEG copiado sem recompressão) |

**Decisão:** adotar **pdfcpu** (biblioteca Go, sem CGO) para extração. Manter `pdftoppm` (poppler-utils)
como fallback para páginas que não sejam "uma imagem JPEG/PNG por página" (texto vetorial, várias imagens
por página, JBIG2/CCITT, CMYK, imagem que não cobre a página). Fallback exercitado no M3 com um PDF só de
texto (renderizado a ~2400 px de altura). A segunda amostra real (`Capítulo 01.pdf`, Guerras Secretas)
também é 100% "uma imagem JPEG por página" e traz título nos metadados internos.
