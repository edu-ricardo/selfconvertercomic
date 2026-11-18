# Especificação — QuadrinhoTeca

Consolidação das rodadas 1–7 de [requisitos.md](requisitos.md) (2026-11-18). Cada requisito tem um id
(`RF-xx` funcional, `RNF-xx` não funcional) e a entrega em que entra: **E1** (conversão + catálogo) ou
**E2** (organização + metadados). A arquitetura da E1 já prevê tudo da E2.

## 1. Bibliotecas

| Id | Requisito | Entrega |
|---|---|---|
| RF-01 | Cadastrar, editar e remover bibliotecas pela UI, escolhendo uma pasta sob `/external` (HDs) ou `/storage` | E1 |
| RF-02 | Várias bibliotecas, inclusive em discos diferentes; HD novo aparece automaticamente para escolha | E1 |
| RF-03 | Detectar arquivos novos/alterados/removidos por eventos do sistema de arquivos + revarredura periódica | E1 |
| RF-04 | Marcar biblioteca como **indisponível** quando o disco sumir, sem apagar o catálogo; retomar ao voltar | E1 |
| RF-05 | Formatos reconhecidos no catálogo: PDF, CBZ, CBR | E1 |

## 2. Conversão PDF → CBZ

| Id | Requisito | Entrega |
|---|---|---|
| RF-10 | Conversão automática dos PDFs novos em bibliotecas com "auto-conversão" ligada | E1 |
| RF-11 | Conversão manual pela UI (um, vários, ou uma pasta inteira) | E1 |
| RF-12 | Só converter arquivos **estáveis** (sem mudança de tamanho/mtime por N minutos, configurável) | E1 |
| RF-13 | 1 PDF → 1 CBZ, no mesmo diretório e com o mesmo nome-base | E1 |
| RF-14 | Extrair as imagens embutidas **sem recompressão**; renderizar só as páginas que não forem uma imagem única ou cujo formato o leitor não abra | E1 |
| RF-15 | Gravar `ComicInfo.xml` no CBZ com o que já se souber (título, nº de páginas, metadados internos do PDF) | E1 |
| RF-16 | Escrita atômica: gerar em arquivo temporário no mesmo disco e renomear ao final; validar o CBZ (abre, nº de páginas = PDF) antes de concluir | E1 |
| RF-17 | Ação sobre o PDF original após sucesso, configurável: **manter**, **mover para pasta de arquivo** ou **apagar** | E1 |
| RF-18 | Fila persistente de jobs (sobrevive a reinício), com status, progresso, erro e "tentar de novo" | E1 |
| RF-19 | Paralelismo padrão **1**; **modo turbo** com N workers e desligamento automático por timer (padrão 4 h) | E1 |
| RF-20 | CBR existentes **não** são reempacotados | E1 |

## 3. Catálogo (UI)

| Id | Requisito | Entrega |
|---|---|---|
| RF-30 | Grade de capas agrupada por série, com miniaturas geradas e guardadas em cache | E1 |
| RF-31 | Filtros: biblioteca, editora, formato (PDF/CBZ/CBR) e status (não convertido, convertendo, erro, sem metadados, aguardando confirmação) | E1 |
| RF-32 | Busca textual por série/título/autor | E1 |
| RF-33 | Página de detalhe com metadados, caminho, tamanho e ações | E1 |
| RF-34 | Link "abrir no Kavita / BookOrbit" a partir de URL base configurável por leitor | E1 |

## 4. Organização

| Id | Requisito | Entrega |
|---|---|---|
| RF-40 | Template configurável de pasta + nome, ex.: `{editora}/{serie} ({ano})/{serie} ({ano}) #{numero}` | E2 |
| RF-41 | Gerar **plano** de renomeação/movimentação com prévia antes/depois; aplicar só o que for aprovado | E2 |
| RF-42 | Histórico de operações com **desfazer** | E2 |
| RF-43 | Mover no mesmo disco = `rename`; entre discos = copiar, verificar e apagar a origem (exFAT/NTFS: sem hardlink/chown) | E2 |
| RF-44 | Detectar duplicados (mesmo conteúdo por hash; mesma edição em formatos diferentes) e **só listar** — nunca apagar sozinho | E2 |

## 5. Metadados

| Id | Requisito | Entrega |
|---|---|---|
| RF-50 | Identificar a edição usando metadados internos do PDF (Info/XMP), nome do arquivo e pastas | E2 (leitura dos metadados internos já na E1) |
| RF-51 | Provedores: **Guia dos Quadrinhos** (scraping, com limite de requisições) e **ComicVine** (API, chave nas configurações) como plano B / edição original | E2 |
| RF-52 | Campos: série, número, título, ano, editora BR, roteiristas, desenhistas, sinopse, capa, vínculo com a edição original | E2 |
| RF-53 | Aplicar automaticamente quando a confiança for alta; senão criar tarefa de confirmação | E2 |
| RF-54 | Gravar em `ComicInfo.xml` dentro do CBZ (CBR/PDF: só no banco, já que CBR não é reempacotado) | E2 |

## 6. Tarefas e notificações

| Id | Requisito | Entrega |
|---|---|---|
| RF-60 | Tela de **tarefas** com tudo que precisa de ação: erros de conversão (E1), confirmação de metadados e de planos de organização (E2) | E1 (base) / E2 |
| RF-61 | Notificações pelo **ntfy** (URL do servidor + tópico), configurável: ao terminar, só em erro, tarefas pendentes | E1 |

## 7. Configurações e backup

| Id | Requisito | Entrega |
|---|---|---|
| RF-70 | Tela de configurações única: bibliotecas, conversão, turbo, ntfy, leitores, provedores de metadados, templates | E1 (+E2) |
| RF-71 | Exportar backup (banco + configurações) sob demanda e agendado, com retenção configurável; restaurar a partir do arquivo | E1 |

## 8. Requisitos não funcionais

| Id | Requisito |
|---|---|
| RNF-01 | Leve: alvo de **< 50 MB de RAM ociosa** e imagem Docker **< 100 MB** |
| RNF-02 | Escala: catálogo responsivo com **dezenas de milhares** de arquivos (paginação no servidor, índices, miniaturas sob demanda) |
| RNF-03 | Revarredura incremental (compara tamanho+mtime; só reprocessa o que mudou) |
| RNF-04 | Nunca perder o original: nenhuma operação destrutiva antes de o resultado ser validado |
| RNF-05 | Autenticação só pelo proxy do Umbrel (`PROXY_AUTH_ADD` ligado) |
| RNF-06 | Empacotamento no padrão da [florencio-store](umbrel.md); imagem só **amd64**, publicada no GHCR |
| RNF-07 | UI em pt-BR, visual moderno e consistente (um design system único), responsiva (celular) |
| RNF-08 | Dados da app em `${APP_DATA_DIR}/data` (disco do sistema), nunca nos HDs |

## 9. Arquitetura proposta

Um único binário Go servindo API + frontend SvelteKit estático:

```
cmd/quadrinhoteca        → main (HTTP, workers, agendador)
internal/library         → bibliotecas, varredura, watcher (fsnotify) e revarredura
internal/jobs            → fila persistente em SQLite + pool de workers (normal/turbo)
internal/convert         → PDF → CBZ (extração de imagens, fallback de renderização, ComicInfo)
internal/catalog         → modelo, consultas, miniaturas
internal/organize   (E2) → templates, planos, aplicar/desfazer, duplicados
internal/metadata   (E2) → provedores (PDF interno, Guia dos Quadrinhos, ComicVine), pontuação
internal/tasks           → tarefas pendentes
internal/notify          → ntfy
internal/backup          → exportar/restaurar (SQLite `VACUUM INTO` + configurações)
web/                     → SvelteKit (adapter-static) + Tailwind
```

Banco: SQLite em modo WAL (FTS5 para busca).

## 10. Pontos em aberto (todos decididos)

1. ~~Biblioteca de PDF~~ — **decidido:** pdfcpu (Go puro, Apache-2.0) para extrair as imagens; `pdftoppm` como
   fallback de renderização (exercitado no M3). Teste em [stack.md](stack.md#teste-do-pdfcpu-2026-11-18).
2. ~~Design system~~ — **decidido:** shadcn-svelte (sobre Tailwind), tema claro/escuro.
3. ~~Limite do Guia dos Quadrinhos~~ — **decidido:** 1 requisição a cada 2 s, cache local das páginas por 30 dias.
4. ~~Padrões iniciais~~ — **decidido:** estabilidade 2 min, revarredura a cada 6 h, backup diário com 7 cópias
   em `${APP_DATA_DIR}/data/backups` (todos ajustáveis na UI).
5. ~~Repositório~~ — **decidido:** código **público** no GitHub; imagem pública no GHCR.
