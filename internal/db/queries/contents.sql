-- name: CreateContent :one
-- Takes the next per-tenant number from tenants.last_content_number.
WITH n AS (
  UPDATE tenants SET last_content_number = last_content_number + 1
  WHERE id = app_current_tenant()
  RETURNING last_content_number
)
INSERT INTO contents (number, type, title, slug, parent_id, body, body_schema_version,
  status, published_at, author_id, fields)
SELECT n.last_content_number, @type, @title, @slug, @parent_id, @body, @body_schema_version,
  @status, @published_at, @author_id, @fields
FROM n
RETURNING *;

-- name: GetContent :one
SELECT * FROM contents WHERE id = $1;

-- name: GetContentByNumber :one
SELECT * FROM contents WHERE number = $1;

-- name: ListContents :many
SELECT * FROM contents WHERE type = $1 ORDER BY number;

-- name: UpdateContentTitle :execrows
UPDATE contents SET title = @title, updated_at = now() WHERE id = @id;

-- name: DeleteContent :execrows
DELETE FROM contents WHERE id = $1;

-- name: AddContentCategory :exec
INSERT INTO content_categories (content_id, category_id) VALUES ($1, $2);

-- name: AddContentTag :exec
INSERT INTO content_tags (content_id, tag_id) VALUES ($1, $2);
