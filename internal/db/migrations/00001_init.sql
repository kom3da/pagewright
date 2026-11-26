-- Initial schema: tenants, users and memberships, contents, categories,
-- tags and media. Every tenant-owned table carries tenant_id and is
-- protected by row level security keyed on the app.tenant_id setting.
--
-- Roles:
--   owner (whoever runs migrations) owns the tables and is NOT subject to
--     RLS, so data migrations and backfills see every tenant.
--   pagewright_app is what the application runs as (db.Open switches to it
--     on every connection). Not owner, no BYPASSRLS: always subject to RLS.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- Roles are cluster-wide: create only when missing (an administrator may
-- pre-create it when the migration runner lacks CREATEROLE) and never drop
-- in Down. The exception handler covers concurrent first runs.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pagewright_app') THEN
    CREATE ROLE pagewright_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
EXCEPTION WHEN duplicate_object OR unique_violation THEN
  NULL;
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
  -- Highest number handed out to a content row (per-tenant URL numbers).
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
  id bigint GENERATED ALWAYS AS IDENTITY,
  -- Per-tenant number used in URLs such as /posts/123/. Assigned by the
  -- contents_assign_number trigger when not given.
  number bigint NOT NULL CHECK (number > 0),
  -- Kind of content ('post', 'page', ...). Free text so new kinds need no
  -- schema change; the application validates known kinds.
  type text NOT NULL CHECK (type ~ '^[a-z][a-z0-9_]*$'),
  -- Last path segment. NULL means the URL falls back to the number, so
  -- all-digit slugs are rejected to keep the two from colliding.
  -- Japanese slugs migrated from WordPress are allowed.
  slug text CHECK (slug <> '' AND slug !~ '/' AND slug !~ '^[0-9]+$'),
  parent_id bigint,
  author_id uuid,
  -- Title, body and extra fields live in content_revisions. The latest
  -- revision is the one being edited; these point at what is live now and
  -- what goes live at scheduled_at. Draft / scheduled / published status is
  -- derived from them.
  published_revision_id bigint,
  scheduled_revision_id bigint,
  scheduled_at timestamptz,
  -- First publication date, shown on the page. Kept when unpublished.
  published_at timestamptz,
  -- Body as it was in the source CMS on import (e.g. WordPress HTML), kept
  -- verbatim so unconverted parts can be fixed later.
  source_format text CHECK (source_format ~ '^[a-z][a-z0-9_]*$'),
  source_body text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((scheduled_revision_id IS NULL) = (scheduled_at IS NULL)),
  CHECK (published_revision_id IS NULL OR published_at IS NOT NULL),
  CHECK ((source_format IS NULL) = (source_body IS NULL)),
  -- The key includes tenant_id so a key conflict never reveals that an id
  -- is used by another tenant.
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, number),
  -- Composite foreign keys keep references inside one tenant.
  FOREIGN KEY (tenant_id, parent_id) REFERENCES contents (tenant_id, id),
  -- Removing a member who still authors content is refused rather than
  -- silently clearing the author.
  FOREIGN KEY (tenant_id, author_id) REFERENCES memberships (tenant_id, user_id)
);
-- Slugs are unique among siblings of the same type (top level uses 0).
CREATE UNIQUE INDEX contents_slug_key
  ON contents (tenant_id, type, coalesce(parent_id, 0), slug)
  WHERE slug IS NOT NULL;
CREATE INDEX contents_parent_idx ON contents (tenant_id, parent_id);
CREATE INDEX contents_author_idx ON contents (tenant_id, author_id);

-- Numbers come from tenants.last_content_number. An explicit number (e.g.
-- a WordPress post ID kept on import) moves the counter past it, so later
-- automatic numbers never collide. Runs as the caller, so RLS on tenants
-- limits it to the current tenant.
-- +goose StatementBegin
CREATE FUNCTION contents_assign_number() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.number IS NULL THEN
    UPDATE tenants SET last_content_number = last_content_number + 1
    WHERE id = NEW.tenant_id
    RETURNING last_content_number INTO NEW.number;
  ELSE
    UPDATE tenants SET last_content_number = greatest(last_content_number, NEW.number)
    WHERE id = NEW.tenant_id;
  END IF;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'tenant % not found', NEW.tenant_id
      USING ERRCODE = 'foreign_key_violation';
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER contents_assign_number BEFORE INSERT ON contents
  FOR EACH ROW EXECUTE FUNCTION contents_assign_number();

-- Saved versions of a content's title, body and extra fields. Rows are
-- never updated (the app has no UPDATE grant): each save adds a revision,
-- so a published revision cannot change under the live site.
CREATE TABLE content_revisions (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant(),
  id bigint GENERATED ALWAYS AS IDENTITY,
  content_id bigint NOT NULL,
  title text NOT NULL DEFAULT '',
  -- ProseMirror (Tiptap) document JSON.
  body jsonb NOT NULL CHECK (jsonb_typeof(body) = 'object'),
  body_schema_version bigint NOT NULL CHECK (body_schema_version > 0),
  -- Extra fields (e.g. migrated ACF values).
  fields jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(fields) = 'object'),
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  -- Lets contents point at a revision of that same content; also serves
  -- "latest revision of a content" lookups.
  UNIQUE (tenant_id, content_id, id),
  FOREIGN KEY (tenant_id, content_id) REFERENCES contents (tenant_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, created_by) REFERENCES memberships (tenant_id, user_id)
);
CREATE INDEX content_revisions_created_by_idx
  ON content_revisions (tenant_id, created_by);

-- Added after content_revisions exists. A published or scheduled revision
-- must belong to the same content, and cannot be deleted while in use.
-- Both tables are new in this migration, so validating is instant.
-- squawk-ignore adding-foreign-key-constraint
ALTER TABLE contents
  ADD CONSTRAINT contents_published_revision_fkey
    FOREIGN KEY (tenant_id, id, published_revision_id)
    REFERENCES content_revisions (tenant_id, content_id, id),
  ADD CONSTRAINT contents_scheduled_revision_fkey
    FOREIGN KEY (tenant_id, id, scheduled_revision_id)
    REFERENCES content_revisions (tenant_id, content_id, id);

CREATE TABLE categories (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  parent_id bigint,
  name text NOT NULL,
  slug text NOT NULL CHECK (slug <> '' AND slug !~ '/'),
  position bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  -- Category URLs are flat (/posts/category/{slug}/).
  UNIQUE (tenant_id, slug),
  FOREIGN KEY (tenant_id, parent_id) REFERENCES categories (tenant_id, id)
);
CREATE INDEX categories_parent_idx ON categories (tenant_id, parent_id);

CREATE TABLE tags (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  name text NOT NULL,
  slug text NOT NULL CHECK (slug <> '' AND slug !~ '/'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
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
  id bigint GENERATED ALWAYS AS IDENTITY,
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
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, storage_key),
  FOREIGN KEY (tenant_id, uploaded_by) REFERENCES memberships (tenant_id, user_id)
);
CREATE INDEX media_uploaded_by_idx ON media (tenant_id, uploaded_by);

-- Every public path of a tenant: the canonical URL of each content,
-- category and tag, plus old URLs kept as redirects. One table so that no
-- two things can claim the same path. Redirects point at the target, not
-- at a path, so they follow later URL changes.
CREATE TABLE routes (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  -- Decoded (no %XX) and NFC-normalized, so Japanese paths compare
  -- reliably. No query or fragment: static files cannot serve /?p=123.
  path text NOT NULL CHECK (
    path ~ '^/' AND path !~ '//' AND path !~ '[?#]'
    AND path !~ '%[0-9A-Fa-f]{2}' AND path IS NFC NORMALIZED
  ),
  kind text NOT NULL CHECK (kind IN ('canonical', 'redirect')),
  content_id bigint,
  category_id bigint,
  tag_id bigint,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (num_nonnulls(content_id, category_id, tag_id) = 1),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, path),
  FOREIGN KEY (tenant_id, content_id) REFERENCES contents (tenant_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, category_id) REFERENCES categories (tenant_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, tag_id) REFERENCES tags (tenant_id, id)
    ON DELETE CASCADE
);
CREATE INDEX routes_content_idx ON routes (tenant_id, content_id);
CREATE INDEX routes_category_idx ON routes (tenant_id, category_id);
CREATE INDEX routes_tag_idx ON routes (tenant_id, tag_id);
-- At most one canonical URL per target.
CREATE UNIQUE INDEX routes_canonical_content_key ON routes (tenant_id, content_id)
  WHERE kind = 'canonical' AND content_id IS NOT NULL;
CREATE UNIQUE INDEX routes_canonical_category_key ON routes (tenant_id, category_id)
  WHERE kind = 'canonical' AND category_id IS NOT NULL;
CREATE UNIQUE INDEX routes_canonical_tag_key ON routes (tenant_id, tag_id)
  WHERE kind = 'canonical' AND tag_id IS NOT NULL;

-- Row level security. Not FORCEd: the owner (migration runner) bypasses
-- it on purpose; the app never runs as the owner.
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenants
  USING (id = app_current_tenant())
  WITH CHECK (id = app_current_tenant());

-- Users have no tenant_id: a user is visible only while a membership in
-- the current tenant exists. Read-only for the app; users may belong to
-- several tenants, so no tenant may change them.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY member_of_current_tenant ON users FOR SELECT
  USING (EXISTS (
    SELECT FROM memberships m
    WHERE m.user_id = users.id AND m.tenant_id = app_current_tenant()
  ));

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE contents ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contents
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE content_revisions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON content_revisions
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON categories
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE tags ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tags
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE content_categories ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON content_categories
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE content_tags ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON content_tags
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE media ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON media
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

ALTER TABLE routes ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON routes
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

-- Login lookup: the only path that reads users without a tenant. Runs as
-- the owner, which is not subject to RLS. Returns at most one row and only
-- the columns login needs.
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
REVOKE ALL ON FUNCTION find_user_by_email(text) FROM PUBLIC;

-- Grants. Creating tenants and users, and adding, changing or removing
-- members, are not granted yet; they come with signup and invitations.
GRANT USAGE ON SCHEMA public TO pagewright_app;
GRANT EXECUTE ON FUNCTION find_user_by_email(text) TO pagewright_app;
GRANT SELECT, UPDATE (name) ON tenants TO pagewright_app;
-- The numbering trigger runs as the app and bumps the counter.
GRANT UPDATE (last_content_number) ON tenants TO pagewright_app;
GRANT SELECT ON users, memberships TO pagewright_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON
  contents, categories, tags, content_categories, content_tags, media, routes
  TO pagewright_app;
-- Revisions are append-only.
GRANT SELECT, INSERT, DELETE ON content_revisions TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-function
DROP FUNCTION find_user_by_email(text);
-- squawk-ignore ban-drop-table
DROP TABLE routes, content_tags, content_categories, media, tags, categories,
  content_revisions, contents, memberships, users, tenants;
-- squawk-ignore ban-drop-function
DROP FUNCTION contents_assign_number(), app_current_tenant();
