-- Edições (entrega 2): arquivos da mesma pasta com o mesmo nome-base ("Capítulo 01.pdf" e
-- "Capítulo 01.cbz") são a mesma edição e compartilham os metadados. Na M8 os campos vêm do nome e das
-- pastas (source = 'name'); os provedores e a confirmação manual os substituem depois.
CREATE TABLE editions (
    id              INTEGER PRIMARY KEY,
    library_id      INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    dir             TEXT    NOT NULL, -- pasta relativa ("" = raiz)
    stem            TEXT    NOT NULL, -- nome do arquivo sem extensão
    series          TEXT    NOT NULL DEFAULT '', -- nome para exibição
    series_key      TEXT    NOT NULL DEFAULT '', -- agrupa a série (metadata.Fold)
    volume          TEXT    NOT NULL DEFAULT '',
    number          TEXT    NOT NULL DEFAULT '', -- "" = desconhecido
    number_inferred INTEGER NOT NULL DEFAULT 0,  -- número deduzido pela ordem de leitura
    count           INTEGER NOT NULL DEFAULT 0,
    year            INTEGER NOT NULL DEFAULT 0,
    grp             TEXT    NOT NULL DEFAULT '', -- grupo de tradução
    read_order      INTEGER NOT NULL DEFAULT 0,
    source          TEXT    NOT NULL DEFAULT 'name',
    parser_version  INTEGER NOT NULL DEFAULT 0, -- edições 'name' de versão antiga são reanalisadas
    UNIQUE (library_id, dir, stem)
);
CREATE INDEX editions_series ON editions (library_id, series_key);

ALTER TABLE files ADD COLUMN edition_id INTEGER REFERENCES editions (id) ON DELETE SET NULL;
CREATE INDEX files_edition ON files (edition_id);

CREATE VIRTUAL TABLE editions_fts USING fts5 (
    series,
    content = 'editions', content_rowid = 'id',
    tokenize = 'unicode61 remove_diacritics 2'
);
CREATE TRIGGER editions_fts_ai AFTER INSERT ON editions BEGIN
    INSERT INTO editions_fts (rowid, series) VALUES (new.id, new.series);
END;
CREATE TRIGGER editions_fts_ad AFTER DELETE ON editions BEGIN
    INSERT INTO editions_fts (editions_fts, rowid, series) VALUES ('delete', old.id, old.series);
END;
CREATE TRIGGER editions_fts_au AFTER UPDATE OF series ON editions BEGIN
    INSERT INTO editions_fts (editions_fts, rowid, series) VALUES ('delete', old.id, old.series);
    INSERT INTO editions_fts (rowid, series) VALUES (new.id, new.series);
END;
