# Empacotamento para o Umbrel

Padrões observados na loja própria [edu-ricardo/florencio-store](https://github.com/edu-ricardo/florencio-store)
(2026-11-18), a seguir neste app.

## Estrutura do app na loja

```
florencio-store-<app>/
  umbrel-app.yml       # manifesto (id, name, version, port, description em pt-BR, ...)
  docker-compose.yml   # serviço app_proxy + serviço(s) do app
  icon.svg|png
  data/.gitkeep        # pastas de dados versionadas vazias
```

- `id` com prefixo `florencio-store-`; descrição e comentários em pt-BR.
- `app_proxy.APP_HOST` = `<app-id>_<serviço>_1`; `APP_PORT` = porta interna.
- Portas da loja vão de 8746 a 8820 — **próxima livre: 8821** (confirmar antes de publicar).
- Imagem fixada por versão **e** digest (`imagem:1.2.3@sha256:...`).
- `user: "1000:1000"`, `TZ: America/Sao_Paulo`, `restart: on-failure`.
- Dados da app (SQLite, cache, miniaturas) em `${APP_DATA_DIR}/data` — **disco do sistema, nunca no HD**.
- Hardening usado no BookOrbit: `read_only: true`, `tmpfs: /tmp`, `cap_drop: ALL`, `no-new-privileges`.

## Imagem Docker

O Umbrel não compila imagens: é preciso publicar no GHCR (`ghcr.io/edu-ricardo/<app>`) via GitHub Actions
(`docker/build-push-action`, `platforms: linux/amd64` (decisão: só amd64), pacote público).
Ex.: `.github/workflows/opengym-coach-image.yml` da loja.

## Acesso às bibliotecas (várias pastas, vários discos)

Mecanismos disponíveis:

| Mecanismo | Como funciona | Atende "adicionar bibliotecas depois"? |
|---|---|---|
| `folderAccess` (manifest 1.1, usado no RomM) | O umbrelOS pergunta a pasta na instalação e permite trocar nas configurações do app | Parcial: número fixo de pastas declarado no manifesto |
| Bind amplo (usado no Navidrome) | Monta a raiz dos HDs (`/home/umbrel/umbrel/external` → umbrelOS monta cada HD em `external/<nome>`) e `data/storage` | **Sim**: a app lista os discos e o usuário escolhe as bibliotecas na UI |

**Proposta:** bind amplo — `${UMBREL_ROOT}/data/storage` → `/storage` e `${UMBREL_ROOT}/external` → `/external`
(`create_host_path: false`), e a UI da app cadastra as bibliotecas como subpastas desses pontos.
Um HD novo aparece automaticamente em `/external/<nome>`. **Decidido na rodada 5.**

Atenção: HDs em exFAT/NTFS não suportam hardlink nem `chown` (ver README da loja) — mover/renomear deve
usar `rename` no mesmo disco e copiar+apagar entre discos.

## Convivência com outros apps

- Kavita lê `Downloads/...`; Mylar3 baixa em `/downloads/comics`; BookOrbit lê `Downloads/books`.
- A biblioteca de quadrinhos atual está no HD externo (`/External/Untitled/comics`).
