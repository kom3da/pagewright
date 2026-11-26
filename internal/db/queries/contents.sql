-- name: CreateContent :one
-- The number is assigned by the contents_assign_number trigger, which locks
-- the tenant row until the transaction ends; keep such transactions short.
INSERT INTO contents (type, slug, parent_id, author_id)
VALUES (@type, @slug, @parent_id, @author_id)
RETURNING *;

-- name: GetContent :one
SELECT * FROM contents WHERE id = $1;

-- name: GetContentByNumber :one
SELECT * FROM contents WHERE number = $1;

-- name: ListContents :many
-- Contents of a type with the title of their latest revision.
SELECT c.*, coalesce(r.title, '')::text AS title
FROM contents c
LEFT JOIN LATERAL (
  SELECT title FROM content_revisions
  WHERE content_id = c.id ORDER BY id DESC LIMIT 1
) r ON true
WHERE c.type = $1
ORDER BY c.number;

-- name: DeleteContent :execrows
DELETE FROM contents WHERE id = $1;

-- name: CreateRevision :one
INSERT INTO content_revisions (content_id, title, body, body_schema_version, fields, created_by)
VALUES (@content_id, @title, @body, @body_schema_version, @fields, @created_by)
RETURNING *;

-- name: GetLatestRevision :one
SELECT * FROM content_revisions WHERE content_id = $1 ORDER BY id DESC LIMIT 1;

-- name: PublishRevision :execrows
-- Makes a revision of the content live. Keeps the first publication date.
UPDATE contents
SET published_revision_id = @revision_id,
    published_at = coalesce(published_at, now()),
    updated_at = now()
WHERE id = @content_id;

-- name: AddContentCategory :exec
INSERT INTO content_categories (content_id, category_id) VALUES ($1, $2);

-- name: AddContentTag :exec
INSERT INTO content_tags (content_id, tag_id) VALUES ($1, $2);

-- name: DeleteRevision :execrows
-- Fails with a foreign key error while the revision is published or scheduled.
DELETE FROM content_revisions WHERE id = $1;
