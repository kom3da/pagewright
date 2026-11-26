-- name: CreatePreviewLink :one
INSERT INTO preview_links (token_hash, content_id, expires_at, created_by)
VALUES (@token_hash, @content_id, @expires_at, @created_by)
RETURNING id, content_id, expires_at, created_at;

-- name: ListPreviewLinks :many
SELECT id, content_id, expires_at, created_at FROM preview_links
WHERE content_id = $1 AND expires_at > now() ORDER BY id DESC;

-- name: DeletePreviewLink :execrows
DELETE FROM preview_links WHERE id = $1;

-- name: LookupPreviewLink :one
-- Runs without a tenant.
SELECT l.tenant_id::uuid AS tenant_id, l.content_id::bigint AS content_id, l.expires_at::timestamptz AS expires_at
FROM preview_link_lookup(@token_hash::bytea) AS l;
