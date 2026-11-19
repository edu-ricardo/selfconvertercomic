-- Hash do conteúdo para achar duplicados (RF-44): SHA-256 em hexadecimal, calculado em segundo plano.
-- '' = ainda não calculado (arquivo novo ou que mudou de tamanho/data).
ALTER TABLE files ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX files_content_hash ON files (content_hash);
