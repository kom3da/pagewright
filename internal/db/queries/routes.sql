-- name: CreateRoute :one
INSERT INTO routes (path, kind, content_id, category_id, tag_id)
VALUES (@path, @kind, @content_id, @category_id, @tag_id)
RETURNING *;

-- name: GetRouteByPath :one
SELECT * FROM routes WHERE path = $1;

-- name: ListRoutes :many
SELECT * FROM routes ORDER BY path;
