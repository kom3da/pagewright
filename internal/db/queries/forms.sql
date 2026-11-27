-- name: CreateFormSubmission :one
INSERT INTO form_submissions (name, kana, email, phone, message, page_path)
VALUES (@name, @kana, @email, @phone, @message, @page_path)
RETURNING *;

-- name: ListFormSubmissions :many
-- Newest first, a page at a time: pass the last id seen as before.
-- handled (optional) keeps only handled or only unhandled messages.
-- handled_by_name is the member who marked it handled ('' when unknown).
SELECT sqlc.embed(f), coalesce(nullif(u.display_name, ''), u.email, '')::text AS handled_by_name
FROM form_submissions f LEFT JOIN users u ON u.id = f.handled_by
WHERE (sqlc.narg(before)::bigint IS NULL OR f.id < sqlc.narg(before)::bigint)
  AND (sqlc.narg(handled)::boolean IS NULL OR (f.handled_at IS NOT NULL) = sqlc.narg(handled)::boolean)
ORDER BY f.id DESC
LIMIT 100;

-- name: GetFormSubmissionWithHandler :one
SELECT sqlc.embed(f), coalesce(nullif(u.display_name, ''), u.email, '')::text AS handled_by_name
FROM form_submissions f LEFT JOIN users u ON u.id = f.handled_by
WHERE f.id = $1;

-- name: SetFormSubmissionHandled :execrows
-- Marking a handled message handled again keeps who did it first, and when.
UPDATE form_submissions SET
  handled_at = CASE WHEN @handled::boolean THEN coalesce(handled_at, now()) END,
  handled_by = CASE WHEN @handled::boolean THEN
    CASE WHEN handled_at IS NULL THEN @user_id::uuid ELSE handled_by END END
WHERE id = @id;

-- name: DeleteFormSubmission :execrows
DELETE FROM form_submissions WHERE id = $1;

-- name: ListAdminEmails :many
-- Admins of the current site, as fallback recipients of form messages.
SELECT u.email FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.role = 'admin' ORDER BY u.email;

-- name: GetFormSubmission :one
SELECT * FROM form_submissions WHERE id = $1;
