-- name: CreateMedia :one
INSERT INTO media (storage_key, filename, content_type, size_bytes, width, height,
  alt_text, uploaded_by)
VALUES (@storage_key, @filename, @content_type, @size_bytes, @width, @height,
  @alt_text, @uploaded_by)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = $1;

-- name: ListMedia :many
SELECT * FROM media ORDER BY id;
