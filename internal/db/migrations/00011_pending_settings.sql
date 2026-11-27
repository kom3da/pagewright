-- Settings of a live content saved in the admin that wait, like the title
-- and body, until its changes are published.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- Changed settings (slug, parentId, categoryIds, tagIds, publishedAt) of a
-- live content, as a JSON object validated by the app; absent keys keep the
-- live value. Applied and cleared when the content is published, scheduled
-- changes go live or it is unpublished. NULL when nothing waits, and always
-- NULL while the content is not live: then settings are saved directly.
ALTER TABLE contents
  ADD COLUMN pending_settings jsonb CHECK (jsonb_typeof(pending_settings) = 'object');

-- +goose Down
-- squawk-ignore ban-drop-column
ALTER TABLE contents DROP COLUMN pending_settings;
