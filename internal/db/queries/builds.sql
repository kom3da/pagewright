-- name: RecordBuild :exec
INSERT INTO site_builds (started_at, files, changed, removed, warnings, error)
VALUES (@started_at, @files, @changed, @removed, @warnings, @error);

-- name: PruneBuilds :exec
-- Keeps the latest 50 records of the current site.
DELETE FROM site_builds WHERE id NOT IN (SELECT id FROM site_builds ORDER BY id DESC LIMIT 50);

-- name: ListBuilds :many
SELECT * FROM site_builds ORDER BY id DESC LIMIT $1;
