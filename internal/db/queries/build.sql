-- Queries for the static site build. Read-only.

-- name: ListPublishedContents :many
-- Contents with their published revision. Only the published revision is
-- ever rendered, never the latest one.
SELECT c.id, c.number, c.type, c.slug, c.parent_id, c.published_at,
  r.id AS revision_id, r.title, r.body, r.body_schema_version, r.fields,
  r.created_at AS revision_created_at
FROM contents c
JOIN content_revisions r
  ON r.content_id = c.id AND r.id = c.published_revision_id
WHERE c.published_revision_id IS NOT NULL
ORDER BY c.published_at DESC, c.number DESC;

-- name: ListContentTree :many
-- Every content's place in the URL tree, published or not, so page paths
-- can be built from their parents' slugs.
SELECT id, number, type, slug, parent_id FROM contents ORDER BY id;

-- name: ListAllContentCategories :many
SELECT content_id, category_id FROM content_categories
ORDER BY content_id, category_id;

-- name: ListAllContentTags :many
SELECT content_id, tag_id FROM content_tags ORDER BY content_id, tag_id;
