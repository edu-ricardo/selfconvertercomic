-- Histórico da organização (RF-42): cada "aplicar" é uma operação com os movimentos feitos, para
-- desfazer. status: done | partial (parou num erro; o que foi feito está em organize_moves) | undone.
CREATE TABLE organize_ops (
    id         INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    template   TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    error      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    undone_at  TEXT
);
CREATE INDEX organize_ops_library ON organize_ops (library_id, id);

CREATE TABLE organize_moves (
    id         INTEGER PRIMARY KEY,
    op_id      INTEGER NOT NULL REFERENCES organize_ops (id) ON DELETE CASCADE,
    edition_id INTEGER NOT NULL,
    file_id    INTEGER NOT NULL,
    from_rel   TEXT    NOT NULL,
    to_rel     TEXT    NOT NULL
);
CREATE INDEX organize_moves_op ON organize_moves (op_id, id);
