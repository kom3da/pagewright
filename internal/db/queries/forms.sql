-- name: CreateFormSubmission :one
INSERT INTO form_submissions (name, kana, email, phone, message, page_path)
VALUES (@name, @kana, @email, @phone, @message, @page_path)
RETURNING *;

-- name: ListFormSubmissions :many
SELECT * FROM form_submissions ORDER BY created_at DESC, id DESC LIMIT 500;

-- name: DeleteFormSubmission :execrows
DELETE FROM form_submissions WHERE id = $1;

-- name: ListAdminEmails :many
-- Admins of the current site, as fallback recipients of form messages.
SELECT u.email FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.role = 'admin' ORDER BY u.email;
