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
