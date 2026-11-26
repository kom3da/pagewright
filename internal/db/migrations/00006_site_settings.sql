-- Site settings (logo, colors, font, time zone) and the site menu.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- Simple values live in one JSON object, validated by the app, so adding a
-- setting needs no migration. The logo is a real reference to media.
ALTER TABLE tenants
  ADD COLUMN settings jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(settings) = 'object'),
  ADD COLUMN logo_media_id bigint;
-- Validated in 00007, in its own transaction, to keep locks short.
ALTER TABLE tenants
  ADD CONSTRAINT tenants_logo_fkey FOREIGN KEY (id, logo_media_id)
    REFERENCES media (tenant_id, id) ON DELETE SET NULL (logo_media_id) NOT VALID;

-- Menu entries in order. Each links to a content or to a URL. With no
-- entries, the theme shows the top-level pages.
CREATE TABLE menu_items (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  position bigint NOT NULL,
  -- Shown text; NULL uses the content's title.
  label text CHECK (label <> ''),
  content_id bigint,
  -- A site path (/...) or an http(s) URL.
  url text CHECK (url ~ '^(/|https?://)'),
  CHECK (num_nonnulls(content_id, url) = 1),
  CHECK (url IS NULL OR label IS NOT NULL),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id, content_id) REFERENCES contents (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX menu_items_content_idx ON menu_items (tenant_id, content_id);

ALTER TABLE menu_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON menu_items
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

GRANT UPDATE (settings, logo_media_id) ON tenants TO pagewright_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON menu_items TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-table
DROP TABLE menu_items;
-- squawk-ignore ban-drop-constraint
ALTER TABLE tenants DROP CONSTRAINT tenants_logo_fkey;
-- squawk-ignore ban-drop-column
ALTER TABLE tenants DROP COLUMN logo_media_id, DROP COLUMN settings;
