# Plano — Entrega 1 (conversão + catálogo)

Escopo: os requisitos marcados **E1** em [especificacao.md](especificacao.md). Cada marco termina com algo
que roda e tem verificação objetiva (diretriz "execução guiada por objetivos" do [CLAUDE.md](../CLAUDE.md)).
Um marco = uma branch/PR.

## Escolhas técnicas da E1

| Tema | Escolha | Motivo |
|---|---|---|
| Módulo Go | `github.com/edu-ricardo/selfconvertercomic`, binário `quadrinhoteca` | — |
| SQLite | `modernc.org/sqlite` (Go puro, FTS5, WAL) | mantém o build **sem CGO** |
| HTTP | `net/http` da biblioteca padrão (roteamento do Go 1.22+) | sem framework |
| Watcher | `fsnotify` + revarredura periódica | RF-03 |
| PDF | `pdfcpu` como biblioteca; `pdftoppm` (poppler-utils) no fallback | ver [stack.md](stack.md) |
| Miniaturas | JPEG 300 px de largura via `image/jpeg` + `golang.org/x/image/draw`, em `/data/cache/thumbs` | sem dependência nativa |
| Frontend | SvelteKit `adapter-static` + Tailwind + shadcn-svelte, embutido no binário com `go:embed` | um só processo |
| Imagem | multi-stage → `alpine` + `poppler-utils`, só `linux/amd64` | RNF-01, RNF-06 |

**Suposição da E1:** antes dos metadados (E2), a **série** de um arquivo é a pasta onde ele está, e a
**editora** é a primeira pasta abaixo da raiz da biblioteca (ex.: `Marvel/guerras_secretas/1984/…` →
editora "Marvel", série "guerras_secretas / 1984"). Na E2 os metadados substituem essa dedução.

## Marcos

### M0 — Esqueleto e imagem Docker ✅
- `go.mod`, `cmd/quadrinhoteca`, `GET /api/health`, frontend SvelteKit mínimo embutido.
- `Dockerfile` multi-stage, `Makefile` (ou `justfile`) com `dev`, `test`, `build`, `docker`.
- Workflow do GitHub Actions: testes em PR; publica `ghcr.io/edu-ricardo/quadrinhoteca:<versão>` em tag.
- **Verificar:** `docker run` responde 200 em `/api/health` e serve a UI; imagem < 100 MB; RAM ociosa < 50 MB
  (`docker stats`); CI verde.
- **Resultado (2026-11-18):** imagem **60 MB** (amd64, com poppler-utils 25.04), **9 MB de RAM** ociosa,
  health 200, UI 200, roda como `1000:1000`, healthcheck `healthy`. CI ainda não executado no GitHub.
  Sem `Makefile`: os comandos estão no README (a máquina de desenvolvimento é Windows).

### M1 — Banco, configurações e casca da UI ✅
- Migrações SQLite versionadas; tabela de configurações com os padrões decididos.
- `GET/PUT /api/settings`; layout da UI (menu lateral: Catálogo, Tarefas, Fila, Configurações), tema
  claro/escuro, responsivo.
- **Verificar:** testes de migração (banco vazio → versão atual); configuração salva sobrevive a reinício;
  UI utilizável em 375 px de largura.
- **Resultado (2026-11-18):** SQLite em WAL com migrações por `PRAGMA user_version`; configurações como
  documento JSON único, com padrões, validação (422 com erros por campo) e `PUT` parcial. UI com
  shadcn-svelte (preset vega), menu lateral (gaveta no celular), tema claro/escuro e tela de Configurações.
  Verificado: valor salvo pela UI sobrevive a reinício (binário e container), 375 px sem rolagem
  horizontal, imagem 67 MB, 7 MB de RAM.

### M2 — Bibliotecas e varredura ✅
- CRUD de bibliotecas; navegador de pastas restrito a `/external` e `/storage` (sem escapar com `..`).
- Varredura incremental (tamanho + mtime), registra PDF/CBZ/CBR; watcher + revarredura a cada 6 h.
- Biblioteca indisponível quando o disco some (RF-04); volta sozinha.
- **Verificar:** testes com pasta temporária (criar/alterar/remover arquivo é refletido); 10 mil arquivos
  falsos varridos em < 30 s e revarredura sem mudanças em < 5 s; remover a pasta raiz marca "indisponível"
  sem apagar o catálogo.
- **Resultado (2026-11-18):** 10 mil arquivos varridos em **0,23 s**, revarredura sem mudanças em
  **0,14 s**. Pasta ausente — ou vazia com catálogo cheio (disco desmontado) — marca "indisponível" sem
  apagar nada, e a checagem a cada minuto a reativa quando o disco volta, refazendo as observações do
  watcher. Rajadas de eventos se juntam (espera de 3 s). Caminhos fora de `QT_LIBRARY_ROOTS` (inclusive via
  link simbólico) e bibliotecas sobrepostas são recusados. Testado também no container Linux (inotify),
  com 10 MB de RAM. Tela **Bibliotecas** com seletor de pastas, contagem por formato e "varrer agora".

### M3 — Fila de jobs e conversão PDF → CBZ ✅
- Fila persistente (status, progresso, erro, tentar de novo); 1 worker; **modo turbo** com N workers e
  desligamento por timer.
- Estabilidade do arquivo (2 min padrão) antes de enfileirar conversão automática.
- Conversor: extrai imagens (pdfcpu) → fallback `pdftoppm` por página → `ComicInfo.xml` (título, páginas,
  metadados internos) → zip em arquivo temporário no mesmo disco → valida (abre, nº de páginas) → renomeia →
  ação sobre o original (manter / mover p/ arquivo / apagar).
- **Verificar:** teste "golden" com PDF de amostra (páginas idênticas byte a byte às imagens do PDF);
  teste com PDF que exige fallback; matar o processo no meio → nenhum CBZ parcial fica na biblioteca e o job
  é retomado; PDF original só é tocado após validação.
- **Resultado (2026-11-18):** páginas copiadas **byte a byte** (teste com JPEGs gerados); página só com
  texto renderizada pelo `pdftoppm` (teste no Linux); cancelamento/queda no meio não deixa CBZ parcial
  (temporário oculto `.nome.cbz.qt-tmp` + `rename`) e o job volta para a fila na partida seguinte; nunca
  sobrescreve CBZ existente. Amostras reais: `Capítulo 01.pdf` (40 págs.) e `Vingadores Mundiais #21.pdf`
  (24 págs.), 100% extraídas sem recompressão. No container: ação "arquivar" moveu os PDFs mantendo a
  estrutura, pico de **46 MB** durante a conversão e **9,8 MB** ociosa depois. Modo turbo testado (1 → 3
  simultâneas, expira sozinho). Bugs achados no caminho: o pdfcpu entrava em `panic` sem `$HOME` no
  container (corrigido com `api.DisableConfigDir()`) e um `panic` derrubava a app (agora vira erro do job).

### M4 — Catálogo ✅
- Miniaturas sob demanda com cache; grade de capas por série com paginação no servidor.
- Filtros: biblioteca, editora, formato, status; busca FTS5 por série/título.
- Página de detalhe com ações (converter, abrir no Kavita/BookOrbit pela URL base configurada).
- **Verificar:** com 10 mil itens, listagem paginada responde em < 200 ms; miniatura gerada uma vez só;
  filtros combinados corretos (teste de API).
- **Resultado (2026-11-18):** com 10 mil arquivos (2 mil com histórico de conversão), 1ª página de séries em
  **11 ms**, última em 18 ms, filtro por status em 60 ms, busca em 2 ms. Busca FTS5 sem acentos e por
  prefixo ("capitulo" acha "Capítulo"). Status por arquivo: não convertido, convertendo, erro, convertido
  (PDF com CBZ ao lado), pronto (CBZ/CBR). Capas: 1ª página por nome (CBZ/CBR, inclusive `.cbr` que é ZIP),
  imagem da página 1 do PDF (renderização só se preciso), 300 px, cache por tamanho+mtime, no máximo 2
  gerações simultâneas e pedidos repetidos juntados; PDF real de 22 MB em 0,13 s (1,5 ms do cache).
  Banco do M3 migrado: a varredura seguinte preencheu série/editora/título. Os links do Kavita e do BookOrbit
  abrem só o endereço base configurado (sem link direto para o quadrinho). No container: catálogo, busca e
  capas ok; a RAM ficou em 125 MB depois de gerar capas (cada página é decodificada inteira) e caiu para
  **18 MB** devolvendo a memória ao sistema quando nenhuma capa está sendo gerada. **Não verificado:**
  capa de CBR em RAR de verdade (não há como gerar RAR aqui; validar com a biblioteca real no M7).

### M5 — Tarefas e notificações ✅
- Tela de tarefas (na E1: erros de conversão com "tentar de novo" / "ignorar").
- ntfy: URL + tópico + eventos escolhidos (terminou, erro, tarefas pendentes); botão "enviar teste".
- **Verificar:** erro de conversão gera tarefa; notificação chega num servidor ntfy de teste (`httptest`).
- **Resultado (2026-11-18):** tabela `tasks` genérica (a E2 reaproveita para metadados e organização), no
  máximo uma tarefa aberta por arquivo e tipo. Falha de conversão abre tarefa; sucesso posterior a resolve
  sozinha; "Tentar de novo" volta o job para a fila; "Ignorar" fecha. Avisos pelo ntfy (publicação JSON,
  UTF-8): um **resumo único quando a fila esvazia** em vez de um aviso por arquivo, respeitando "ao
  terminar", "em erro" e "tarefas pendentes"; prioridade alta quando há erro; botão "Salvar e enviar
  teste". Ponta a ponta com um PDF bom e um corrompido e um ntfy falso: 1 tarefa e o resumo "1 quadrinho
  convertido · 1 conversão com erro · 1 tarefa aguardando você". Contador de tarefas no menu. Container:
  23 MB de RAM. Bug corrigido: cancelar uma conversão antes de o conversor começar a marcava como erro.
  **Não testado** com o ntfy real do Umbrel (não mandei aviso ao seu celular).

### M6 — Backup
- Exportar sob demanda e agendado (diário, 7 cópias) via `VACUUM INTO` + configurações num `.zip` em
  `/data/backups`; baixar pela UI; restaurar a partir de arquivo enviado.
- **Verificar:** exportar → apagar banco → restaurar → catálogo e configurações idênticos (teste
  automatizado).

### M7 — Empacotamento e instalação no Umbrel
- App `florencio-store-quadrinhoteca` na loja (porta 8821, binds `/external` e `/storage`
  com `create_host_path: false`, dados em `${APP_DATA_DIR}/data`) — PR no repositório da loja.
- **Verificar:** instalar no Umbrel, cadastrar `/external/Untitled/comics`, converter os 12 PDFs de
  `Marvel/guerras_secretas/1984` com o original **mantido**, abrir os CBZ no Kavita.

## Pronto da E1

Todos os marcos verificados, CI verde, imagem publicada e app instalado no Umbrel convertendo a biblioteca
real sem perda de arquivos.

## Riscos

| Risco | Mitigação |
|---|---|
| Limite de *watches* do inotify com dezenas de milhares de pastas | Detectar o erro, cair para revarredura e avisar na UI |
| PDFs que o pdfcpu não extrai | Fallback `pdftoppm`; se falhar, tarefa de erro (nunca CBZ parcial) |
| HD exFAT/NTFS sem permissões POSIX | Não usar `chown`/`chmod`; gravar como `1000:1000` |
| Escrita na biblioteca enquanto Kavita/BookOrbit varrem | Escrita atômica (temporário + `rename`) |
