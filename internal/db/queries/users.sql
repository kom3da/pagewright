-- name: FindUserByEmail :one
-- Works without a tenant; used at login.
SELECT f.id::uuid AS id, f.email::text AS email, f.display_name::text AS display_name
FROM find_user_by_email(@email::text) AS f;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: ListMembers :many
SELECT u.id, u.email, u.display_name, m.role, m.created_at
FROM memberships m JOIN users u ON u.id = m.user_id
ORDER BY u.email;

-- name: GetMember :one
SELECT u.id, u.email, u.display_name, m.role, m.created_at
FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.user_id = $1;

-- name: IsMemberEmail :one
SELECT EXISTS (
  SELECT FROM memberships m JOIN users u ON u.id = m.user_id WHERE u.email = lower(@email::text)
);

-- name: LockMembers :exec
-- Serializes changes to the site's members (see memberships_keep_admin in
-- migrations/00017_invitations.sql). Taken before changing a member.
SELECT FROM tenants WHERE id = app_current_tenant() FOR NO KEY UPDATE;

-- name: SetMemberRole :execrows
UPDATE memberships SET role = @role WHERE user_id = @user_id;

-- name: RemoveMember :execrows
DELETE FROM memberships WHERE user_id = $1;
