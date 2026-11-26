-- Queries used by importers (internal/migrate/...).

-- name: CreateImportedContent :one
-- Keeps the number from the source CMS (e.g. a WordPress post ID); the
-- contents_assign_number trigger moves the tenant counter past it.
INSERT INTO contents (number, type, slug, parent_id, author_id, source_format, source_body)
VALUES (@number, @type, @slug, @parent_id, @author_id, @source_format, @source_body)
RETURNING *;

-- name: PublishImportedRevision :execrows
-- Like PublishRevision, but keeps the publication date of the source.
UPDATE contents
SET published_revision_id = @revision_id,
    published_at = @published_at,
    updated_at = now()
WHERE id = @content_id;

-- name: ScheduleImportedRevision :execrows
UPDATE contents
SET scheduled_revision_id = @revision_id,
    scheduled_at = @scheduled_at,
    updated_at = now()
WHERE id = @content_id;

-- name: ContentSlugTaken :one
-- Mirrors the contents_slug_key unique index.
SELECT EXISTS (
  SELECT FROM contents
  WHERE type = @type
    AND coalesce(parent_id, 0) = coalesce(sqlc.narg(parent_id)::bigint, 0)
    AND slug = @slug
);

-- name: GetCategoryBySlug :one
SELECT * FROM categories WHERE slug = $1;

-- name: GetTagBySlug :one
SELECT * FROM tags WHERE slug = $1;

-- name: GetMediaByStorageKey :one
SELECT * FROM media WHERE storage_key = $1;

-- name: IsMember :one
SELECT EXISTS (SELECT FROM memberships WHERE user_id = $1);

-- Savepoints let a dry run import every item in one transaction, rolling
-- back only the item that failed, and finally the whole transaction.

-- name: Savepoint :exec
SAVEPOINT import_item;

-- name: RollbackToSavepoint :exec
ROLLBACK TO SAVEPOINT import_item;

-- name: ReleaseSavepoint :exec
RELEASE SAVEPOINT import_item;
