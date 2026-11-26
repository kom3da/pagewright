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
-- Contents of a type, newest number first, with the title and id of their
-- latest revision and their canonical URL. q (an ILIKE pattern, already
-- escaped) matches the title or slug; status filters by derived status.
SELECT c.*, coalesce(lr.title, '')::text AS title, coalesce(lr.id, 0)::bigint AS latest_revision_id,
  r.path AS url
FROM contents c
LEFT JOIN LATERAL (
  SELECT id, title FROM content_revisions
  WHERE content_id = c.id ORDER BY id DESC LIMIT 1
) lr ON true
LEFT JOIN routes r ON r.content_id = c.id AND r.kind = 'canonical'
WHERE c.type = @type
  AND (sqlc.narg(q)::text IS NULL OR lr.title ILIKE sqlc.narg(q)::text ESCAPE '\' OR c.slug ILIKE sqlc.narg(q)::text ESCAPE '\')
  AND (sqlc.narg(status)::text IS NULL OR sqlc.narg(status)::text = CASE
        WHEN c.published_revision_id IS NOT NULL THEN 'published'
        WHEN c.scheduled_revision_id IS NOT NULL THEN 'scheduled'
        ELSE 'draft' END)
ORDER BY c.number DESC
LIMIT 1000;

-- name: GetContentURL :one
SELECT r.path FROM routes r WHERE r.content_id = $1 AND r.kind = 'canonical';

-- name: TouchContent :exec
UPDATE contents SET updated_at = now() WHERE id = $1;

-- name: UpdateContentSlug :exec
UPDATE contents SET slug = @slug, updated_at = now() WHERE id = @id;

-- name: UpdateContentParent :exec
UPDATE contents SET parent_id = @parent_id, updated_at = now() WHERE id = @id;

-- name: ContentAncestry :many
-- The content and its ancestors, root first.
WITH RECURSIVE chain AS (
  SELECT c.id, c.parent_id, c.slug, c.number, 0 AS depth FROM contents c WHERE c.id = $1
  UNION ALL
  SELECT p.id, p.parent_id, p.slug, p.number, chain.depth + 1
  FROM contents p JOIN chain ON p.id = chain.parent_id
  WHERE chain.depth < 32
)
SELECT chain.id, chain.slug, chain.number FROM chain ORDER BY chain.depth DESC;

-- name: ListChildContentIDs :many
SELECT id FROM contents WHERE parent_id = $1 ORDER BY id;

-- name: ListContentCategoryIDs :many
SELECT category_id FROM content_categories WHERE content_id = $1 ORDER BY category_id;

-- name: ListContentTagIDs :many
SELECT tag_id FROM content_tags WHERE content_id = $1 ORDER BY tag_id;

-- name: ClearContentCategories :exec
DELETE FROM content_categories WHERE content_id = $1;

-- name: ClearContentTags :exec
DELETE FROM content_tags WHERE content_id = $1;

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

-- name: ScheduleRevision :execrows
-- Sets (or with NULLs, clears) the revision that goes live at scheduled_at.
UPDATE contents
SET scheduled_revision_id = @revision_id, scheduled_at = @scheduled_at, updated_at = now()
WHERE id = @content_id;

-- name: PublishScheduled :one
-- Publishes the scheduled revision when it is still the given one and is
-- due. No row when it was rescheduled, cancelled or already published.
UPDATE contents
SET published_revision_id = scheduled_revision_id,
    published_at = coalesce(published_at, scheduled_at),
    scheduled_revision_id = NULL, scheduled_at = NULL, updated_at = now()
WHERE id = @content_id AND scheduled_revision_id = @revision_id AND scheduled_at <= now()
RETURNING id;

-- name: ListScheduled :many
SELECT id, scheduled_revision_id, scheduled_at FROM contents
WHERE scheduled_revision_id IS NOT NULL ORDER BY scheduled_at;

-- name: LockContent :one
-- Locks the content row until the transaction ends, so concurrent saves of
-- one content take turns.
SELECT id FROM contents WHERE id = $1 FOR UPDATE;
