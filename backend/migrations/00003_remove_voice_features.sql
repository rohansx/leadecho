-- +goose Up

-- Remove exemplar/effectiveness from document_chunks
DROP INDEX IF EXISTS idx_chunks_exemplars;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS is_exemplar;
ALTER TABLE document_chunks DROP COLUMN IF EXISTS effectiveness_score;

-- Remove variant from replies, replace with simple tone text
ALTER TABLE replies DROP COLUMN IF EXISTS variant;
DROP TYPE IF EXISTS reply_variant;

-- +goose Down

-- Re-add reply_variant
CREATE TYPE reply_variant AS ENUM ('value_only', 'technical', 'soft_sell');
ALTER TABLE replies ADD COLUMN variant reply_variant NOT NULL DEFAULT 'value_only';

-- Re-add exemplar fields
ALTER TABLE document_chunks ADD COLUMN is_exemplar BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE document_chunks ADD COLUMN effectiveness_score REAL DEFAULT 0.0;
CREATE INDEX idx_chunks_exemplars ON document_chunks(workspace_id) WHERE is_exemplar = true;
