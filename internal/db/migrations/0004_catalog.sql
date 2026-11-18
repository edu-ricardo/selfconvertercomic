-- Campos do catálogo. Na entrega 1 vêm das pastas (editora = 1ª pasta, série = pasta do arquivo);
-- na entrega 2 os metadados passam a preenchê-los. title vazio = linha ainda não derivada (a próxima
-- varredura preenche).
ALTER TABLE files ADD COLUMN publisher TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN series    TEXT NOT NULL DEFAULT ''; -- pasta relativa do arquivo ("" = raiz)
ALTER TABLE files ADD COLUMN title     TEXT NOT NULL DEFAULT '';

CREATE INDEX files_series ON files (library_id, series, rel_path);
CREATE INDEX files_publisher ON files (publisher);

-- Busca textual sem acentos ("capitulo" acha "Capítulo").
CREATE VIRTUAL TABLE files_fts USING fts5 (
    title, series, publisher,
    content = 'files', content_rowid = 'id',
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER files_fts_ai AFTER INSERT ON files BEGIN
    INSERT INTO files_fts (rowid, title, series, publisher) VALUES (new.id, new.title, new.series, new.publisher);
END;
CREATE TRIGGER files_fts_ad AFTER DELETE ON files BEGIN
    INSERT INTO files_fts (files_fts, rowid, title, series, publisher) VALUES ('delete', old.id, old.title, old.series, old.publisher);
END;
CREATE TRIGGER files_fts_au AFTER UPDATE OF title, series, publisher ON files BEGIN
    INSERT INTO files_fts (files_fts, rowid, title, series, publisher) VALUES ('delete', old.id, old.title, old.series, old.publisher);
    INSERT INTO files_fts (rowid, title, series, publisher) VALUES (new.id, new.title, new.series, new.publisher);
END;
