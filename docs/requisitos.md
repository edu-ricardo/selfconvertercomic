# Requisitos

## Necessidade (enunciado original)

Biblioteca de quadrinhos no homelab (Umbrel), servida aos dispositivos pelo Bookorbit.

Objetivos principais:

1. Converter quadrinhos em PDF para CBZ/CBR para facilitar a leitura.
2. Organizar a biblioteca.

Objetivo secundário (se possível):

- Localizar metadados para os quadrinhos. Hoje é manual no Bookorbit, cujos provedores só cobrem inglês;
  os quadrinhos são em português.

## Requisitos técnicos

- O mais leve e rápida possível.
- Roda no servidor Umbrel e será publicada na loja própria do Umbrel (app em Docker).
- Frontend para configurações e visualização da biblioteca, com visual moderno e padronizado.
- Ciclo de desenvolvimento segue as diretrizes Karpathy (ver `CLAUDE.md`).

## Entrevista

Perguntas e respostas registradas por rodada. Itens marcados **[aberto]** ainda não foram decididos.

### Rodada 1 — escopo e stack (2026-11-18)

| Tema | Decisão |
|---|---|
| Formato de saída | **Somente CBZ** (CBR descartado: criação exige RAR proprietário) |
| Gatilho da conversão | **Automática por pasta monitorada** + **manual pela interface** (sem varredura agendada) |
| Organização | **Renomear arquivos**, **estrutura de pastas**, **detectar duplicados**, **catálogo na UI** |
| Stack | **Go + SvelteKit (estático)** — ver [stack.md](stack.md) |

### Rodada 2 — conversão e fontes (2026-11-18)

| Tema | Decisão |
|---|---|
| PDF original após converter | **Configurável** (manter / mover p/ pasta de arquivo / apagar) |
| Geração das páginas | **Extrair imagens embutidas sem recompressão**; renderizar só páginas sem imagem única |
| Padrão atual da biblioteca | Inspecionado (somente leitura) — ver "Estado atual da biblioteca" abaixo |
| Fontes de metadados pt-BR | **Guia dos Quadrinhos** (scraping, sem API) + **ComicVine** (API em inglês, dados da edição original) |

#### Estado atual da biblioteca (inspeção em 2026-11-18)

- Local: `/External/Untitled/comics` (HD USB externo no Umbrel). Subpastas: `Darkhorse`, `DC Comics`,
  `Marvel`, `Manga` (vazia), `Downloads` (vazia).
- Estrutura irregular: às vezes `Editora/Série/Ano/` (ex.: `Marvel/guerras_secretas/1984/`), às vezes
  `Editora/Agrupamento/` com arquivos soltos (ex.: `Marvel/Grandes Eventos/`).
- Nomes inconsistentes: `Capítulo 01.pdf` (série só no nome da pasta),
  `Mark Millar - Guerra Civil (2022).cbr`, `Unknown author - Batman - ... Vol. 1.cbr`.
- Formatos encontrados: PDF, CBR e CBZ.
- Leitores no Umbrel: BookOrbit e Kavita (loja própria `florencio-store-*`).

### Rodada 3 — organização e metadados (2026-11-18)

| Tema | Decisão |
|---|---|
| Pasta com vários PDFs (capítulos) | **1 CBZ por PDF** (cada PDF é uma edição) |
| Padrão de organização | **Template configurável** com variáveis (ex.: `{editora}/{serie} ({ano})/{serie} ({ano}) #{numero}`) |
| CBR já existentes | **Manter como estão** (só organizar/renomear, sem reempacotar) |
| Gravação de metadados | **ComicInfo.xml dentro do CBZ** |

### Rodada 4 — acesso, pastas e entregas (2026-11-18)

| Tema | Decisão |
|---|---|
| Autenticação | **Só o login do Umbrel** (`PROXY_AUTH_ADD` padrão/ligado); sem login próprio por enquanto |
| Pastas da biblioteca | **Configuráveis pela UI**, com **várias bibliotecas** em discos diferentes (inclusive discos adicionados no futuro) |
| Guia dos Quadrinhos | Aceito o risco do scraping (bloqueio/mudança de layout); **ComicVine como plano B** |
| Entregas | **Entrega 1:** conversão PDF→CBZ + catálogo. **Entrega 2:** organização + metadados. Requisitos levantados por completo agora e arquitetura pensada para o todo desde a entrega 1 |
| Empacotamento | Seguir o padrão da loja [edu-ricardo/florencio-store](https://github.com/edu-ricardo/florencio-store) — ver [umbrel.md](umbrel.md) |
