-- name: CreateFormSubmission :one
INSERT INTO form_submissions (name, kana, email, phone, message, page_path)
VALUES (@name, @kana, @email, @phone, @message, @page_path)
RETURNING *;

-- name: ListFormSubmissions :many
-- Newest first, a page at a time: pass the last id seen as before.
SELECT * FROM form_submissions
WHERE sqlc.narg(before)::bigint IS NULL OR id < sqlc.narg(before)::bigint
ORDER BY id DESC
LIMIT 100;

-- name: DeleteFormSubmission :execrows
DELETE FROM form_submissions WHERE id = $1;

-- name: ListAdminEmails :many
-- Admins of the current site, as fallback recipients of form messages.
SELECT u.email FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.role = 'admin' ORDER BY u.email;

-- name: GetFormSubmission :one
SELECT * FROM form_submissions WHERE id = $1;
