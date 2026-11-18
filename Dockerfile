# syntax=docker/dockerfile:1

# 1) Frontend estático
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 2) Binário Go (sem CGO)
FROM golang:1.27-alpine AS go
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/build ./web/build
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/quadrinhoteca ./cmd/quadrinhoteca

# 3) Imagem final: binário + poppler-utils (pdftoppm, fallback de renderização)
FROM alpine:3.22
RUN apk add --no-cache poppler-utils ca-certificates tzdata
COPY --from=go /out/quadrinhoteca /usr/local/bin/quadrinhoteca
ENV QT_ADDR=:8080
USER 1000:1000
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/quadrinhoteca"]
