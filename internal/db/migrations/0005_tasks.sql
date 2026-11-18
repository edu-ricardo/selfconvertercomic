-- Tarefas: o que precisa da ação do usuário. Na entrega 1, só erros de conversão; na entrega 2,
-- confirmações de metadados e planos de organização usam a mesma tabela (outros valores de kind).
CREATE TABLE tasks (
    id          INTEGER PRIMARY KEY,
    kind        TEXT    NOT NULL,               -- ex.: conversion_error
    library_id  INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path    TEXT    NOT NULL,
    ref_id      INTEGER NOT NULL DEFAULT 0,     -- ex.: id do job que falhou
    title       TEXT    NOT NULL,
    detail      TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'dismissed')),
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    resolved_at TEXT
);

-- No máximo uma tarefa aberta de cada tipo por arquivo.
CREATE UNIQUE INDEX tasks_open_file ON tasks (kind, library_id, rel_path) WHERE status = 'open';
CREATE INDEX tasks_status ON tasks (status, id);
