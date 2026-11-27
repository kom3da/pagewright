-- Each site's own public URL, so that a second site's pages (canonical
-- links, sitemap, feeds, previews, form result pages) link to its own
-- domain. Set by the operator with `pagewright site set-url`, not in the
-- admin: the domain must match the web server's configuration.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- An origin: http(s)://host[:port] in lower case, without a path, query
-- or trailing slash (the CLI normalizes it). NULL uses the server's
-- PAGEWRIGHT_PUBLIC_BASE_URL. The app may read it (it has SELECT on
-- tenants) but not change it (no UPDATE grant on this column).
ALTER TABLE tenants
  ADD COLUMN public_base_url text
    CHECK (public_base_url ~ '^https?://[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:[0-9]{1,5})?$');

-- +goose Down
-- squawk-ignore ban-drop-column
ALTER TABLE tenants DROP COLUMN public_base_url;
