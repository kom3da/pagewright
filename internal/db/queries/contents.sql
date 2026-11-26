-- name: CreateContent :one
-- The number is assigned by the contents_assign_number trigger, which locks
-- the tenant row until the transaction ends; keep such transactions short.
INSERT INTO contents (type, title, slug, parent_id, body, body_schema_version,
  status, published_at, author_id, fields)
VALUES (@type, @title, @slug, @parent_id, @body, @body_schema_version,
  @status, @published_at, @author_id, @fields)
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
