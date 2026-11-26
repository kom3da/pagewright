-- These run without a tenant; see migrations/00002_auth.sql.

-- name: AuthRequestLogin :one
SELECT auth_request_login(@email::text, @token_hash::bytea, (@ttl_seconds::bigint || ' seconds')::interval)::boolean AS found;

-- name: AuthLogin :one
SELECT auth_login(@login_token_hash::bytea, @session_token_hash::bytea, (@ttl_seconds::bigint || ' seconds')::interval)::uuid AS user_id;

-- name: AuthSession :one
SELECT s.user_id::uuid AS user_id, s.email::text AS email, s.display_name::text AS display_name,
  s.tenant_id::uuid AS tenant_id, coalesce(s.role, '')::text AS role
FROM auth_session(@token_hash::bytea) AS s;

-- name: AuthSessionSites :many
SELECT s.tenant_id::uuid AS tenant_id, s.name::text AS name, s.role::text AS role
FROM auth_session_sites(@token_hash::bytea) AS s;

-- name: AuthSelectSite :one
SELECT auth_select_site(@token_hash::bytea, @tenant_id::uuid)::boolean AS ok;

-- name: AuthLogout :exec
SELECT auth_logout(@token_hash::bytea);

-- name: AuthCleanup :one
SELECT auth_cleanup()::bigint AS removed;
