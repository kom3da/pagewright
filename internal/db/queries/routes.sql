-- name: CreateRoute :one
INSERT INTO routes (path, kind, content_id, category_id, tag_id)
VALUES (@path, @kind, @content_id, @category_id, @tag_id)
RETURNING *;

-- name: GetRouteByPath :one
SELECT * FROM routes WHERE path = $1;

-- name: ListRoutes :many
SELECT * FROM routes ORDER BY path;

-- name: GetCanonicalRouteForContent :one
SELECT * FROM routes WHERE content_id = $1 AND kind = 'canonical';

-- name: GetCanonicalRouteForCategory :one
SELECT * FROM routes WHERE category_id = $1 AND kind = 'canonical';

-- name: GetCanonicalRouteForTag :one
SELECT * FROM routes WHERE tag_id = $1 AND kind = 'canonical';

-- name: DemoteRouteToRedirect :exec
UPDATE routes SET kind = 'redirect' WHERE id = $1;

-- name: DeleteRoute :exec
DELETE FROM routes WHERE id = $1;
