-- Bibliotecas cadastradas pelo usuário (uma pasta raiz cada).
CREATE TABLE libraries (
    id           INTEGER PRIMARY KEY,
    name         TEXT    NOT NULL,
    path         TEXT    NOT NULL UNIQUE,
    auto_convert INTEGER NOT NULL DEFAULT 1,
    available    INTEGER NOT NULL DEFAULT 1,
    last_scan_at TEXT,
    last_error   TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Arquivos de quadrinhos encontrados na varredura. rel_path usa "/" como separador.
CREATE TABLE files (
    id            INTEGER PRIMARY KEY,
    library_id    INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path      TEXT    NOT NULL,
    format        TEXT    NOT NULL CHECK (format IN ('pdf', 'cbz', 'cbr')),
    size          INTEGER NOT NULL,
    mtime         INTEGER NOT NULL, -- Unix, em nanossegundos
    first_seen_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    changed_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (library_id, rel_path)
);

CREATE INDEX files_library_format ON files (library_id, format);
