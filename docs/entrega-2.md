# Plano — Entrega 2 (metadados + organização)

Escopo: os requisitos marcados **E2** em [especificacao.md](especificacao.md), com as decisões da rodada 8
de [requisitos.md](requisitos.md#rodada-8--fontes-de-metadados-e-acervo-2026-11-19): fontes **ComicVine**
(edição original) e **API do GCD** (edição brasileira), acervo misturado, metadados antes da organização.
Mesmo formato da [entrega 1](entrega-1.md): um marco = uma branch/PR, cada um com verificação objetiva.

## Escolhas técnicas da E2

| Tema | Escolha | Motivo |
|---|---|---|
| Unidade de metadados | A **edição**: arquivos da mesma pasta com o mesmo nome-base (`Capítulo 01.pdf` + `Capítulo 01.cbz`) compartilham um registro | a biblioteca inteira já foi convertida mantendo os PDFs |
| Identificação local | Analisador de nome e pastas (série, volume, número, ano, grupo) + `Title` do PDF | é a única pista na maioria dos arquivos |
| ComicVine | API JSON (`/search`, `/volumes`, `/issues`, `/issue/4000-<id>`), chave do usuário, User-Agent próprio | limite do serviço: 200 pedidos/hora por recurso |
| GCD | API REST (`/api/series/name/<nome>/`, `/api/issue/<id>/`), sem chave | dados CC BY-SA 4.0: mostrar a atribuição na UI |
| Ritmo e cache | Um pedido por vez por provedor, intervalo configurável (padrão 2 s), respostas guardadas no SQLite por 30 dias | gentileza com os serviços; reidentificar não refaz pedidos |
| Confiança | Pontuação 0–100 (série, número, ano, editora, nº de páginas); **≥ 85 aplica sozinho**, abaixo vira tarefa | RF-53 |
| ComicInfo | Reescrever só o `ComicInfo.xml` do CBZ (temporário + `rename`, páginas copiadas sem recompressão) | RF-54, RNF-04 |
| Mover arquivos | `rename` no mesmo disco; entre discos copiar → conferir hash → apagar a origem | RF-43 (exFAT/NTFS) |

As configurações `metadata.guiaRateSeconds` passam a valer para os provedores (renomear para
`metadata.rateSeconds` com migração do valor salvo).

## Marcos

### M8 — Identificação local ✅
- Tabela `editions` (biblioteca, pasta, nome-base, série, volume, número, ano, título, grupo, fonte,
  confiança, status) ligada aos arquivos; preenchida na varredura.
- Analisador de nomes para os padrões vistos no acervo (`NNN-Serie-vN-NNN`, `Serie vN #NN (GRUPO)`,
  `Serie-NNN-AAAA-GRUPO`, `Capítulo NN` → série da pasta, prefixo `Cópia de`, nomes truncados) e `Title`
  do PDF quando houver.
- Catálogo passa a agrupar por série identificada (com a pasta como reserva) e mostra o número da edição.
- **Verificar:** tabela de casos com ~40 nomes reais do acervo (série, volume, número esperados) passa
  100%; rodando na biblioteca real (via backup do banco do Umbrel) ≥ 90% das edições com série + número.
- **Resultado (2026-11-19):** tabela `editions` (uma por pasta + nome-base, então PDF e CBZ da mesma
  edição compartilham o registro) preenchida na varredura, com reanálise automática quando o analisador
  muda (`ParserVersion`) e limpeza das edições sem arquivos. Analisador em `internal/metadata`: marcadores
  `vN`, `#N`, `Vol. N`, `Capítulo N`, `NN de NN`, ano, grupo (`(MK-SQ)`, `-SQ`), prefixo de ordem
  (`034-`), autores antes de ` - `; usa o **nome da pasta** quando ele corresponde ao do arquivo (recupera
  acentos e nomes cortados: `Capit-Marvel-e-a-Tropa-da-Ca` → "Capitã Marvel & A Tropa da Carol"); série
  pela pasta para `Capítulo NN` (pasta-ano vira ano: `guerras_secretas/1984` → "Guerras Secretas", 1984).
  Os nomes com hífens do acervo são **cortados em 32 caracteres** e muitos perdem o número; ele é
  **deduzido pela ordem de leitura** quando todas as edições da série têm o prefixo e os números
  conhecidos batem com a posição (marcado como "deduzido" na UI). O `Title` interno do PDF ficou de fora:
  no acervo ele é genérico ("Guerras Secretas") e lê-lo custaria abrir cada PDF na varredura.
  Casos de teste com 48 nomes reais: 100%. Acervo real (295 edições, lista fora do repositório,
  `QT_ACERVO`): **294 com série e número (99,7%)**, 91 números deduzidos pela ordem, 70 séries; a que
  falta é a edição única `Mark Millar - Guerra Civil (2022)`. Catálogo agrupa por série identificada,
  ordena por número e busca também pela série; 10 mil arquivos: séries em 29–65 ms (a 1ª versão levou 3 s
  na última página — capa calculada para todas as séries; corrigido calculando só para as da página).

### M9 — Provedores ComicVine e GCD
- Clientes com ritmo, cache, tempo-limite e erros claros (chave inválida, limite atingido, serviço fora).
- Busca: volume por nome (+ ano) → edição por número; GCD por nome da série em pt.
- Pontuação dos candidatos; botão "testar chave" nas configurações.
- **Verificar:** testes com servidores falsos (`httptest`) para busca, cache (2ª busca sem pedido),
  ritmo e limite (HTTP 420/429); teste real único com a sua chave: `Secret Wars (1984) #1` encontrado.

### M10 — Aplicar metadados e confirmar
- Identificação em segundo plano (fila própria, retomável), automática para edições novas e manual
  ("identificar série", "identificar biblioteca").
- Confiança alta → aplica; senão abre tarefa "confirmar metadados" com até 5 candidatos (capa, série,
  número, ano) e busca manual.
- Gravar `ComicInfo.xml` no CBZ (série, número, título, ano, editora, roteiro, desenho, sinopse, link
  da edição original, `Web` com a URL da fonte); PDF/CBR só no banco.
- Detalhe da edição mostra os metadados e a fonte; catálogo usa série/editora/ano identificados.
- **Verificar:** as 12 edições de `Marvel/guerras_secretas/1984` identificadas como *Secret Wars* (1984)
  #1–12 com roteiro e desenho; `ComicInfo.xml` regravado e válido (páginas intactas, mesmo hash); caso
  ambíguo vira tarefa e "confirmar" aplica.

### M11 — Organização (plano, aplicar, desfazer)
- Template configurável (`{editora}/{serie} ({ano})/{serie} ({ano}) #{numero}`), com prévia ao digitar.
- Plano por biblioteca/série: lista antes → depois, conflitos (destino existe, nomes iguais) marcados;
  aplicar só os itens aprovados; edição move PDF e CBZ juntos.
- Histórico de operações com **desfazer**; movimento entre discos com conferência de hash.
- **Verificar:** testes de plano/aplicar/desfazer (mesmo disco e entre discos, conflito, falha no meio
  deixa tudo consistente); na pasta do 1984: aplicar e desfazer restauram nomes e caminhos exatos.

### M12 — Duplicados
- Hash do conteúdo calculado em segundo plano (sem bloquear a varredura); grupos por conteúdo igual e
  por mesma edição em formatos/pastas diferentes.
- Tela "Duplicados" só lista e leva ao arquivo; nada é apagado (RF-44).
- **Verificar:** testes com cópias idênticas e mesma edição em formatos diferentes; na biblioteca real,
  a árvore `arquivospdf/Quadrinhos/Marvel/...` aparece como duplicada de `Marvel/...`.

### M13 — Versão 0.2.0 no Umbrel
- Tag `v0.2.0`, atualização do app na loja (digest novo).
- **Verificar:** atualizar no Umbrel sem perder o banco; identificar a biblioteca real; conferir as
  tarefas criadas; organizar e desfazer a pasta do 1984.

## Fora deste plano
- **Kavita lendo o HD externo:** ajuste no pacote do Kavita na florencio-store (outro repositório).
- **Aviso pelo ntfy real:** continua sem teste.

## Riscos

| Risco | Mitigação |
|---|---|
| Cobertura brasileira rala no GCD | Cair para a edição original (ComicVine); campos BR ficam vazios em vez de inventados |
| Limite do ComicVine (200/h por recurso) numa biblioteca grande | Cache por série (um volume serve todas as edições), fila retomável, progresso visível |
| GCD bloquear a API como fez no site | Provedor opcional; desligável nas configurações sem afetar o resto |
| Nomes truncados ou ambíguos | Nunca aplicar abaixo do limiar; vira tarefa |
| Mover arquivos que Kavita/BookOrbit estão lendo | Só com aprovação; desfazer disponível |
