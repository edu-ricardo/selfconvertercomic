-- Respostas dos provedores de metadados (ComicVine, GCD), para não repetir pedidos: a chave é o
-- provedor + o pedido sem a chave de API; fetched_at em segundos Unix. Expira por metadata.cacheDays.
CREATE TABLE metadata_cache (
    key        TEXT    PRIMARY KEY,
    body       BLOB    NOT NULL,
    fetched_at INTEGER NOT NULL
);
CREATE INDEX metadata_cache_fetched ON metadata_cache (fetched_at);
