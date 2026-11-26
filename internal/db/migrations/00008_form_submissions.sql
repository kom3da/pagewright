-- Messages sent with the contact form on the public site.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE form_submissions (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  name text NOT NULL CHECK (name <> ''),
  kana text NOT NULL DEFAULT '',
  email text NOT NULL CHECK (email <> ''),
  phone text NOT NULL DEFAULT '',
  message text NOT NULL CHECK (message <> ''),
  -- The page the form was on.
  page_path text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX form_submissions_created_idx ON form_submissions (tenant_id, created_at DESC);

ALTER TABLE form_submissions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON form_submissions
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

-- Personal data: no UPDATE.
GRANT SELECT, INSERT, DELETE ON form_submissions TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-table
DROP TABLE form_submissions;
