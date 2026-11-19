-- Metadados das fontes por edição (M10).
-- meta_status: ''       nunca pedido (edições anteriores a esta versão; "identificar" na UI)
--              new      edição nova: identifica sozinho se metadata.autoIdentify estiver ligado
--              queued   pedido pelo usuário
--              applied  metadados aplicados (meta_json); ComicInfo.xml gravado nos CBZ
--              rewrite  aplicados, mas um CBZ novo da edição ainda está sem eles
--              task     sem confiança suficiente: tarefa de confirmação aberta (meta_candidates)
--              error    a busca falhou (meta_error)
ALTER TABLE editions ADD COLUMN meta_status     TEXT    NOT NULL DEFAULT '';
ALTER TABLE editions ADD COLUMN meta_error      TEXT    NOT NULL DEFAULT '';
ALTER TABLE editions ADD COLUMN meta_score      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE editions ADD COLUMN meta_json       TEXT    NOT NULL DEFAULT ''; -- metadata.Details aplicado
ALTER TABLE editions ADD COLUMN meta_candidates TEXT    NOT NULL DEFAULT ''; -- []metadata.Candidate da última busca
ALTER TABLE editions ADD COLUMN meta_at         TEXT;
CREATE INDEX editions_meta ON editions (meta_status);
