-- Initial schema: tenants, users and memberships, contents, categories,
-- tags and media. Every tenant-owned table carries tenant_id and is
-- protected by row level security keyed on the app.tenant_id setting.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- Roles are cluster-wide, so create them only when missing and never drop
-- them in Down (other databases on the same cluster may use them).
--   pagewright_app  : the role the application runs as. Subject to RLS.
--   pagewright_auth : owns the email lookup function used at login, before
--                     a tenant is known. Cannot log in.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pagewright_app') THEN
    CREATE ROLE pagewright_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pagewright_auth') THEN
    CREATE ROLE pagewright_auth NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
END
$$;
-- +goose StatementEnd

-- The tenant of the current transaction. NULL when unset, so policies
-- comparing against it match no rows (fail closed).
-- +goose StatementBegin
CREATE FUNCTION app_current_tenant() RETURNS uuid
LANGUAGE sql STABLE
AS $$
  SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid
$$;
-- +goose StatementEnd

CREATE TABLE tenants (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  name text NOT NULL,
  -- Last number handed out to a content row (per-tenant URL numbers).
  last_content_number bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Users are shared across tenants; membership and role are per tenant.
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  email text NOT NULL UNIQUE CHECK (email = lower(email)),
  display_name text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  user_id uuid NOT NULL REFERENCES users (id),
  role text NOT NULL CHECK (role IN ('admin', 'editor')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);
-- Used by the users policy and to list a user's tenants.
CREATE INDEX memberships_user_id_idx ON memberships (user_id);

CREATE TABLE contents (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  -- Per-tenant number used in URLs such as /posts/123/.
  number bigint NOT NULL CHECK (number > 0),
  -- Kind of content ('post', 'page', ...). Free text so new kinds need no
  -- schema change; the application validates known kinds.
  type text NOT NULL CHECK (type ~ '^[a-z][a-z0-9_]*$'),
  title text NOT NULL DEFAULT '',
  -- Last path segment. NULL means the URL falls back to the number.
  -- Japanese slugs migrated from WordPress are allowed.
  slug text CHECK (slug <> '' AND slug !~ '/'),
  parent_id bigint,
  body jsonb NOT NULL,
  body_schema_version bigint NOT NULL CHECK (body_schema_version > 0),
  status text NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft', 'scheduled', 'published')),
  published_at timestamptz,
  author_id uuid,
  -- Extra fields (e.g. migrated ACF values).
  fields jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(fields) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (status = 'draft' OR published_at IS NOT NULL),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, number),
  -- Composite foreign keys keep references inside one tenant.
  FOREIGN KEY (tenant_id, parent_id) REFERENCES contents (tenant_id, id),
  FOREIGN KEY (tenant_id, author_id) REFERENCES memberships (tenant_id, user_id)
    ON DELETE SET NULL (author_id)
);
-- Slugs are unique among siblings of the same type (top level uses 0).
CREATE UNIQUE INDEX contents_slug_key
  ON contents (tenant_id, type, coalesce(parent_id, 0), slug)
  WHERE slug IS NOT NULL;
CREATE INDEX contents_parent_idx ON contents (tenant_id, parent_id);
CREATE INDEX contents_author_idx ON contents (tenant_id, author_id);

CREATE TABLE categories (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  parent_id bigint,
  name text NOT NULL,
  slug text NOT NULL CHECK (slug <> '' AND slug !~ '/'),
  position bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  -- Category URLs are flat (/posts/category/{slug}/).
  UNIQUE (tenant_id, slug),
  FOREIGN KEY (tenant_id, parent_id) REFERENCES categories (tenant_id, id)
);
CREATE INDEX categories_parent_idx ON categories (tenant_id, parent_id);

CREATE TABLE tags (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL CHECK (slug <> '' AND slug !~ '/'),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, slug)
);

CREATE TABLE content_categories (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant(),
  content_id bigint NOT NULL,
  category_id bigint NOT NULL,
  PRIMARY KEY (tenant_id, content_id, category_id),
  FOREIGN KEY (tenant_id, content_id) REFERENCES contents (tenant_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, category_id) REFERENCES categories (tenant_id, id)
    ON DELETE CASCADE
);
CREATE INDEX content_categories_category_idx
  ON content_categories (tenant_id, category_id);

CREATE TABLE content_tags (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant(),
  content_id bigint NOT NULL,
  tag_id bigint NOT NULL,
  PRIMARY KEY (tenant_id, content_id, tag_id),
  FOREIGN KEY (tenant_id, content_id) REFERENCES contents (tenant_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, tag_id) REFERENCES tags (tenant_id, id)
    ON DELETE CASCADE
);
CREATE INDEX content_tags_tag_idx ON content_tags (tenant_id, tag_id);

CREATE TABLE media (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  -- Object key in local or S3-compatible storage.
  storage_key text NOT NULL,
  filename text NOT NULL,
  content_type text NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  width bigint,
  height bigint,
  alt_text text NOT NULL DEFAULT '',
  uploaded_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, storage_key),
  FOREIGN KEY (tenant_id, uploaded_by) REFERENCES memberships (tenant_id, user_id)
    ON DELETE SET NULL (uploaded_by)
);
CREATE INDEX media_uploaded_by_idx ON media (tenant_id, uploaded_by);

-- Row level security. FORCE makes the table owner subject to it too;
-- only superusers bypass it.
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenants
  USING (id = app_current_tenant())
  WITH CHECK (id = app_current_tenant());

-- Users have no tenant_id: a user is visible only while a membership in
-- the current tenant exists.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY member_of_current_tenant ON users
  USING (EXISTS (
    SELECT FROM memberships m
    WHERE m.user_id = users.id AND m.tenant_id = app_current_tenant()
  ))
  WITH CHECK (EXISTS (
    SELECT FROM memberships m
    WHERE m.user_id = users.id AND m.tenant_id = app_current_tenant()
  ));
-- Lets find_user_by_email (owned by pagewright_auth) read any user.
CREATE POLICY auth_lookup ON users FOR SELECT TO pagewright_auth
  USING (true);

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE contents ENABLE ROW LEVEL SECURITY;
ALTER TABLE contents FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contents
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON categories
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE tags ENABLE ROW LEVEL SECURITY;
ALTER TABLE tags FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tags
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE content_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON content_categories
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE content_tags ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_tags FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON content_tags
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE media ENABLE ROW LEVEL SECURITY;
ALTER TABLE media FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON media
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

-- Login lookup: the only path that reads users without a tenant. Returns
-- at most one row and only the columns login needs.
-- +goose StatementBegin
CREATE FUNCTION find_user_by_email(p_email text)
RETURNS TABLE (id uuid, email text, display_name text)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
  SELECT u.id, u.email, u.display_name FROM public.users u
  WHERE u.email = lower(p_email)
$$;
-- +goose StatementEnd
ALTER FUNCTION find_user_by_email(text) OWNER TO pagewright_auth;
REVOKE ALL ON FUNCTION find_user_by_email(text) FROM PUBLIC;

-- Grants. Tenant creation and user creation are not granted to the app
-- yet; they come with the signup and invitation features.
GRANT USAGE ON SCHEMA public TO pagewright_app, pagewright_auth;
GRANT SELECT ON users TO pagewright_auth;
GRANT EXECUTE ON FUNCTION find_user_by_email(text) TO pagewright_app;
GRANT EXECUTE ON FUNCTION app_current_tenant() TO pagewright_app;
GRANT SELECT, UPDATE (name, last_content_number) ON tenants TO pagewright_app;
GRANT SELECT, UPDATE (display_name) ON users TO pagewright_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON
  memberships, contents, categories, tags, content_categories, content_tags, media
  TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-function
DROP FUNCTION find_user_by_email(text);
-- squawk-ignore ban-drop-table
DROP TABLE content_tags, content_categories, media, tags, categories, contents,
  memberships, users, tenants;
-- squawk-ignore ban-drop-function
DROP FUNCTION app_current_tenant();
