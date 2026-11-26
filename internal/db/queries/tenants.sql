-- name: GetCurrentTenant :one
SELECT * FROM tenants WHERE id = app_current_tenant();
