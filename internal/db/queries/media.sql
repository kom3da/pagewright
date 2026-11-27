-- name: CreateMedia :one
INSERT INTO media (storage_key, filename, content_type, size_bytes, width, height,
  alt_text, uploaded_by)
VALUES (@storage_key, @filename, @content_type, @size_bytes, @width, @height,
  @alt_text, @uploaded_by)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = $1;

-- name: ListMedia :many
SELECT * FROM media ORDER BY id DESC;

-- name: UpdateMediaAlt :one
UPDATE media SET alt_text = @alt_text WHERE id = @id RETURNING *;

-- name: MediaInUse :one
-- Whether the site logo is the image, or the latest, published or
-- scheduled revision of any content shows it.
SELECT (EXISTS (
  SELECT FROM contents c
  JOIN content_revisions r ON r.content_id = c.id
  WHERE (r.id = c.published_revision_id OR r.id = c.scheduled_revision_id
         OR r.id = (SELECT max(l.id) FROM content_revisions l WHERE l.content_id = c.id))
    AND jsonb_path_exists(r.body, '$.** ? (@.type == "image" && @.attrs.mediaId == $id)',
                          jsonb_build_object('id', @media_id::bigint))
) OR EXISTS (
  SELECT FROM tenants t WHERE t.logo_media_id = @media_id::bigint
))::boolean AS in_use;

-- name: DeleteMedia :one
DELETE FROM media WHERE id = $1 RETURNING storage_key;

-- name: LockMediaQuota :exec
-- Makes uploads of the current tenant take turns until the transaction
-- ends, so two uploads cannot both pass the quota check. Two-key advisory
-- lock in the namespace 'pwmq' (see build.buildLockNamespace).
SELECT pg_advisory_xact_lock(1886874993, hashtext(app_current_tenant()::text));

-- name: MediaTotalBytes :one
SELECT coalesce(sum(size_bytes), 0)::bigint AS total FROM media;
