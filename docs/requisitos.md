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

### Rodada 5 — montagem, segurança das alterações e imagem (2026-11-18)

| Tema | Decisão |
|---|---|
| Montagem das bibliotecas | **Bind amplo**: `/external` (todos os HDs) + `/storage`; bibliotecas cadastradas na UI |
| Aplicar organização | **Prévia + aprovação** (antes/depois), com **histórico para desfazer** |
| Duplicados | **Só listar na UI**; a app nunca apaga sozinha |
| Arquitetura da imagem | **Só amd64** |

### Rodada 6 — metadados, catálogo, avisos e nome (2026-11-18)

| Tema | Decisão |
|---|---|
| Identificação da edição | Automática usando **metadados internos do PDF** (título, autor, assunto, palavras-chave, XMP), **nome do arquivo** e **pastas**; pedir confirmação **só quando houver dúvida** |
| Fila de confirmação | **Tela de tarefas** com tudo que precisa de confirmação/ajuste, para resolver aos poucos |
| Campos de metadados | Série, número, título, ano, editora brasileira, roteiristas, desenhistas, sinopse, capa e vínculo com a edição original (EUA) |
| ComicVine | Usuário **tem chave de API** (informada nas configurações) |
| Catálogo | **Grade de capas por série**; filtros por **editora**, **formato** e **status**; **link** para abrir no Kavita/BookOrbit (sem leitor embutido) |
| Notificações | **ntfy**, configurável: ao terminar, só em erro e/ou quando houver tarefas de ajuste pendentes |
| Nome do app | **QuadrinhoTeca** (id sugerido na loja: `florencio-store-quadrinhoteca`) |

### Rodada 7 — operação e desempenho (2026-11-18)

| Tema | Decisão |
|---|---|
| Volume | Centenas hoje; projetar para **dezenas de milhares** de arquivos |
| Paralelismo | **1 conversão por vez** por padrão; **modo turbo** opcional com mais paralelismo e **desligamento automático por timer** (ex.: 4 h) |
| Detecção | **Eventos do sistema de arquivos** + **revarredura periódica** de segurança (cobre HD desconectado/reconectado) |
| Arquivo em cópia | Só converter após o arquivo ficar **estável** por um tempo configurável (padrão na casa de **minutos**) |
| Links para leitores | **URL base configurável** por leitor (Kavita, BookOrbit) |
| Backup | A app **exporta backups** próprios (banco + configurações) |

### Rodada 8 — fontes de metadados e acervo (2026-11-19)

Descobertas antes de planejar a E2:

- O **Guia dos Quadrinhos** passou a ficar atrás do desafio anti-robô do Cloudflare (*managed challenge*):
  qualquer cliente automático recebe 403 com "Just a moment…". Contornar o desafio é burlar a proteção do
  site, o que a app não fará. O site do GCD tem o mesmo bloqueio, mas a **API do GCD** (`comics.org/api/`)
  responde sem chave e tem séries brasileiras em pt-BR (Panini), com cobertura rala (muitas edições sem
  créditos, sinopse ou capa).
- Os metadados internos dos PDFs quase não ajudam (ex.: só `Title = Guerras Secretas`, `Author =
  HQOnline.com.br`; outro PDF sem nada). A identificação depende do **nome do arquivo e das pastas**.
- Os nomes misturam padrões: `015-Magneto-v3-019`, `Os Novos Vingadores v3 #25 (MK-SQ)`,
  `Surfista-Prateado-013-2015-SQ`, `Capítulo 01` (série = pasta), `Cópia de Capítulo 37`, nomes truncados
  (`189-Capit-Marvel-e-a-Tropa-da-Ca`). Há uma árvore `arquivospdf/Quadrinhos/Marvel/...` que parece
  repetir `Marvel/...`.

| Tema | Decisão |
|---|---|
| Fontes de metadados | **ComicVine** (chave do usuário) para a edição original americana + **API do GCD** para a edição brasileira quando existir. O **Guia dos Quadrinhos sai do escopo** |
| Acervo | **Misturado**: traduções de fã de edições americanas e digitalizações de edições brasileiras. A app tenta a edição BR (GCD) e cai para a original (ComicVine) |
| Ordem da E2 | **Metadados primeiro**; a organização usa os dados identificados |

### Rodada 9 — depois de usar a 0.2.0 (2026-11-19)

Pedidos do usuário depois de testar a versão 0.2.0 no Umbrel:

| Tema | Decisão |
|---|---|
| Excluir duplicados | Pela tela de Duplicados, **a pedido do usuário** (a app continua sem apagar nada sozinha). Vai para uma **lixeira da app** (pasta oculta na própria biblioteca), com **restaurar** e **esvaziar**; nos HDs exFAT não há lixeira do sistema |
| Opções de exclusão | **PDFs que já têm CBZ**; **uma pasta inteira** (manter só a cópia de fora dela, ex.: a árvore `arquivospdf/`); **escolher um a um**. Nunca deixa um conteúdo sem nenhuma cópia |
| Metadados manuais | Formulário para preencher/corrigir os metadados de uma edição; gravados no `ComicInfo.xml` como os das fontes |
| Botão "Usar este" desativado | Acontecia com candidatos que são só a **série** (arquivo sem número, ex.: `Mark Millar - Guerra Civil (2022)`): passa a pedir o número e buscar a edição nessa série |
| Metadados por link | Colar o link da página da edição (ou da série + número) no ComicVine ou no GCD e aplicar o que vier de lá |

### Rodada 10 — depois de usar a 0.3.0 (2026-11-19)

| Tema | Decisão |
|---|---|
| Encadernados e edições especiais | Não têm número: a fonte acha a série, mas confirmar pedia um número (o padrão "1" não casa com o "[nn]" do GCD). Sem número, vale a **edição única da série**; se a série tiver várias edições, continua pedindo o número. Na busca sem número, a série de uma edição só já vem com a edição (botão "Usar este") |
