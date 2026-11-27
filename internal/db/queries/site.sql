-- name: GetSite :one
SELECT t.name, t.settings, t.logo_media_id, t.public_base_url FROM tenants t WHERE t.id = app_current_tenant();

-- name: UpdateSite :exec
UPDATE tenants SET name = @name, settings = @settings, logo_media_id = @logo_media_id
WHERE id = app_current_tenant();

-- name: ListMenuItems :many
SELECT mi.id, mi.position, mi.label, mi.content_id, mi.url FROM menu_items mi ORDER BY mi.position, mi.id;

-- name: ClearMenu :exec
DELETE FROM menu_items;

-- name: AddMenuItem :exec
INSERT INTO menu_items (position, label, content_id, url) VALUES (@position, @label, @content_id, @url);
