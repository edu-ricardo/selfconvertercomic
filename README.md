# QuadrinhoTeca

Aplicação self-hosted para homelab (Umbrel) que converte quadrinhos em PDF para CBZ,
organiza a biblioteca e, opcionalmente, busca metadados em português.

> Status: requisitos consolidados em [docs/especificacao.md](docs/especificacao.md) — ver [docs/requisitos.md](docs/requisitos.md).

## Documentação

- [docs/requisitos.md](docs/requisitos.md) — necessidade, requisitos e respostas da entrevista
- [docs/especificacao.md](docs/especificacao.md) — requisitos consolidados, entregas e arquitetura
- [docs/entrega-1.md](docs/entrega-1.md) — plano da entrega 1 (marcos e verificações)
- [docs/stack.md](docs/stack.md) — opções de tecnologia
- [docs/umbrel.md](docs/umbrel.md) — empacotamento para a loja do Umbrel
- [CLAUDE.md](CLAUDE.md) — convenções e diretrizes de desenvolvimento

## Desenvolvimento

Requisitos: Go 1.27+, Node 24+, Docker (para a imagem).

```sh
# Frontend (gera web/build, embutido no binário)
cd web && npm ci && npm run build && cd ..

# Testes e servidor (http://localhost:8080)
go test ./...
go run ./cmd/quadrinhoteca

# UI com recarga automática (proxy de /api para o Go na 8080)
cd web && npm run dev

# Imagem
docker build --build-arg VERSION=dev -t quadrinhoteca:dev .
```

Variáveis: `QT_ADDR` (padrão `:8080`) e `QT_DATA_DIR` (padrão `data`; na imagem, `/data`), onde ficam o banco SQLite e os demais dados.
Publicação: criar a tag `vX.Y.Z` publica `ghcr.io/edu-ricardo/quadrinhoteca:X.Y.Z` (workflow `image.yml`).
