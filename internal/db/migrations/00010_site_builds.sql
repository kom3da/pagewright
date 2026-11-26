-- A record of site builds, so the admin can show when the site was last
-- updated and what the build could not publish.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE site_builds (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  started_at timestamptz NOT NULL,
  finished_at timestamptz NOT NULL DEFAULT now(),
  files bigint NOT NULL DEFAULT 0,
  changed bigint NOT NULL DEFAULT 0,
  removed bigint NOT NULL DEFAULT 0,
  -- Problems that did not stop the build (texts for the admin).
  warnings jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(warnings) = 'array'),
  -- Set when the build failed.
  error text,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX site_builds_recent_idx ON site_builds (tenant_id, id DESC);

ALTER TABLE site_builds ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON site_builds
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

GRANT SELECT, INSERT, DELETE ON site_builds TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-table
DROP TABLE site_builds;
