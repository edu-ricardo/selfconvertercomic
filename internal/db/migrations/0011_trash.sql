-- Lixeira da app (rodada 9): arquivos excluídos pela tela de Duplicados vão para uma pasta oculta da
-- própria biblioteca (.quadrinhoteca-lixeira/<quando>/<caminho original>), ignorada pela varredura.
-- Restaurar devolve ao caminho original; esvaziar apaga de vez.
CREATE TABLE trash (
    id          INTEGER PRIMARY KEY,
    library_id  INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    rel_path    TEXT    NOT NULL, -- onde estava
    trash_rel   TEXT    NOT NULL, -- onde está (relativo à biblioteca)
    size        INTEGER NOT NULL,
    format      TEXT    NOT NULL,
    reason      TEXT    NOT NULL DEFAULT '',
    deleted_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX trash_library ON trash (library_id, id);
