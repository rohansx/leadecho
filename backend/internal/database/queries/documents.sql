-- name: ListDocuments :many
SELECT * FROM documents
WHERE workspace_id = @workspace_id AND is_active = true
ORDER BY created_at DESC;

-- name: GetDocument :one
SELECT * FROM documents
WHERE id = @id AND workspace_id = @workspace_id;

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
