-- name: CreateCategory :one
INSERT INTO categories (parent_id, name, slug, position)
VALUES (@parent_id, @name, @slug, @position)
RETURNING *;

-- name: ListCategories :many
SELECT * FROM categories ORDER BY position, id;

-- name: ListContentCategories :many
SELECT c.* FROM categories c
JOIN content_categories cc ON cc.category_id = c.id
WHERE cc.content_id = $1
ORDER BY c.position, c.id;

-- name: CreateTag :one
INSERT INTO tags (name, slug) VALUES (@name, @slug) RETURNING *;

-- name: ListTags :many
SELECT * FROM tags ORDER BY slug;

-- name: ListContentTags :many
SELECT t.* FROM tags t
JOIN content_tags ct ON ct.tag_id = t.id
WHERE ct.content_id = $1
ORDER BY t.slug;

-- name: GetCategory :one
SELECT * FROM categories WHERE id = $1;

-- name: UpdateCategory :one
UPDATE categories SET
  name = coalesce(sqlc.narg(name), name),
  slug = coalesce(sqlc.narg(slug), slug),
  parent_id = CASE WHEN @set_parent::boolean THEN sqlc.narg(parent_id) ELSE parent_id END,
  position = coalesce(sqlc.narg(position), position)
WHERE id = @id
RETURNING *;

-- name: DeleteCategory :execrows
DELETE FROM categories WHERE id = $1;

-- name: CategoryAncestorIDs :many
-- Ids of the category and its ancestors (for cycle checks).
WITH RECURSIVE chain AS (
  SELECT c.id, c.parent_id, 0 AS depth FROM categories c WHERE c.id = $1
  UNION ALL
  SELECT p.id, p.parent_id, chain.depth + 1
  FROM categories p JOIN chain ON p.id = chain.parent_id
  WHERE chain.depth < 32
)
SELECT chain.id FROM chain;

-- name: GetTag :one
SELECT * FROM tags WHERE id = $1;

-- name: DeleteTag :execrows
DELETE FROM tags WHERE id = $1;

-- name: UpdateTag :one
UPDATE tags SET
  name = coalesce(sqlc.narg(name), name),
  slug = coalesce(sqlc.narg(slug), slug)
WHERE id = @id
RETURNING *;

-- name: ReparentCategoryChildren :exec
-- Moves the children of a category up to its parent, before deleting it.
UPDATE categories
SET parent_id = (SELECT p.parent_id FROM categories p WHERE p.id = @id)
WHERE parent_id = @id;
