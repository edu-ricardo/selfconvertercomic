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

**Decisão: [aberto]**
