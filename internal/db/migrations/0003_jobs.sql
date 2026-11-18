-- Fila de conversões PDF → CBZ. Guarda tamanho/mtime do PDF para não repetir o mesmo arquivo.
CREATE TABLE jobs (
    id              INTEGER PRIMARY KEY,
    library_id      INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path        TEXT    NOT NULL,
    src_size        INTEGER NOT NULL,
    src_mtime       INTEGER NOT NULL,
    auto            INTEGER NOT NULL DEFAULT 0,
    status          TEXT    NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    progress        INTEGER NOT NULL DEFAULT 0,
    pages           INTEGER NOT NULL DEFAULT 0,
    extracted       INTEGER NOT NULL DEFAULT 0,
    rendered        INTEGER NOT NULL DEFAULT 0,
    output_rel_path TEXT    NOT NULL DEFAULT '',
    error           TEXT    NOT NULL DEFAULT '',
    attempts        INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    started_at      TEXT,
    finished_at     TEXT
);

-- No máximo um job ativo por arquivo.
CREATE UNIQUE INDEX jobs_active_file ON jobs (library_id, rel_path) WHERE status IN ('queued', 'running');
CREATE INDEX jobs_status ON jobs (status, id);
CREATE INDEX jobs_file_version ON jobs (library_id, rel_path, src_size, src_mtime);
