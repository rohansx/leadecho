-- name: ListDocuments :many
SELECT * FROM documents
WHERE workspace_id = @workspace_id AND is_active = true
ORDER BY created_at DESC;

-- name: GetDocument :one
SELECT * FROM documents
WHERE id = @id AND workspace_id = @workspace_id AND is_active = true;

-- name: CreateDocument :one
INSERT INTO documents (
    workspace_id, title, content, content_type, source_url, file_size_bytes
) VALUES (
    @workspace_id, @title, @content, @content_type, @source_url, @file_size_bytes
) RETURNING *;

-- name: UpdateDocument :one
UPDATE documents
SET title = @title,
    content = @content,
    is_active = @is_active
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: DeleteDocument :exec
UPDATE documents
SET is_active = false
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CountDocuments :one
SELECT COUNT(*)::int as count FROM documents
WHERE workspace_id = @workspace_id AND is_active = true;

-- name: UpdateDocumentChunkCount :exec
UPDATE documents
SET chunk_count = @chunk_count
WHERE id = @id;

-- name: DeleteDocumentChunks :exec
DELETE FROM document_chunks
WHERE document_id = @document_id AND workspace_id = @workspace_id;

-- name: InsertDocumentChunk :one
INSERT INTO document_chunks (
    document_id, workspace_id, content, embedding, chunk_index, section_title
) VALUES (
    @document_id, @workspace_id, @content, @embedding, @chunk_index, @section_title
) RETURNING *;

-- name: FindSimilarDocumentChunks :many
SELECT
    dc.id,
    dc.document_id,
    dc.content,
    dc.section_title,
    (1 - (dc.embedding <=> @query_embedding::vector))::float8 AS similarity
FROM document_chunks dc
WHERE dc.workspace_id = @workspace_id
ORDER BY dc.embedding <=> @query_embedding::vector
LIMIT @lim;
