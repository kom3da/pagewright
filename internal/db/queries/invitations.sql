-- name: ListInvitations :many
-- inviter_name is the sender's display name, or their email without one;
-- empty once they have left the site.
SELECT i.id, i.email, i.role, i.expires_at, i.sent_at,
  coalesce(nullif(u.display_name, ''), u.email, '')::text AS inviter_name
FROM invitations i LEFT JOIN users u ON u.id = i.invited_by
ORDER BY i.sent_at DESC, i.id DESC;

-- name: GetInvitation :one
SELECT * FROM invitations WHERE id = $1;

-- name: UpsertInvitation :one
-- Inviting an address that already has an invitation replaces it: the old
-- link stops working.
INSERT INTO invitations (email, role, token_hash, expires_at, invited_by)
VALUES (lower(@email::text), @role, @token_hash, @expires_at, @invited_by)
ON CONFLICT (tenant_id, email) DO UPDATE
SET role = EXCLUDED.role, token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at,
  invited_by = EXCLUDED.invited_by, sent_at = now()
RETURNING *;

-- name: RenewInvitation :one
UPDATE invitations
SET token_hash = @token_hash, expires_at = @expires_at, invited_by = @invited_by, sent_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteInvitation :execrows
DELETE FROM invitations WHERE id = $1;
