-- name: FindUserByEmail :one
-- Works without a tenant; used at login.
SELECT f.id::uuid AS id, f.email::text AS email, f.display_name::text AS display_name
FROM find_user_by_email(@email::text) AS f;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: ListMembers :many
SELECT u.id, u.email, u.display_name, m.role
FROM memberships m JOIN users u ON u.id = m.user_id
ORDER BY u.email;
