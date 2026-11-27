-- Ids are numbered per tenant instead of from one sequence shared by all
-- tenants. With a shared sequence a tenant could tell from the gaps
-- between its own ids how much other tenants create (e.g. how many
-- inquiries they get). Every key already includes tenant_id (PRIMARY KEY
-- (tenant_id, id) and composite foreign keys), so ids only need to be
-- unique within a tenant. Existing ids stay as they are.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- The highest id handed out per tenant and table. name is the table name
-- (TG_TABLE_NAME of the trigger below).
CREATE TABLE tenant_id_counters (
  tenant_id uuid NOT NULL REFERENCES tenants (id),
  name text NOT NULL,
  last_id bigint NOT NULL,
  PRIMARY KEY (tenant_id, name)
);

ALTER TABLE tenant_id_counters ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_id_counters
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

-- The trigger runs as the app: it reads the new value back (SELECT),
-- creates a tenant's counter on first use (INSERT) and bumps it (UPDATE).
GRANT SELECT, INSERT, UPDATE (last_id) ON tenant_id_counters TO pagewright_app;

-- Assigns NEW.id from the (tenant, table) counter. The upsert is atomic:
-- its row lock serializes inserts into one table of one tenant until the
-- transaction ends, so concurrent inserts never get the same id, and a
-- rolled-back insert gives its id back. An explicit id (imports, tests)
-- moves the counter past it, so later ids never collide.
--
-- Runs as the caller, so RLS on the counters applies to the app. BEFORE
-- triggers run before the table's own RLS check, so a row for another
-- tenant (or with no tenant set) is refused here with the error that check
-- would give, naming the table written to, rather than an error about
-- tenant_id_counters (or a NOT NULL violation when no tenant is set).
-- +goose StatementBegin
CREATE FUNCTION assign_per_tenant_id() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF row_security_active('tenant_id_counters')
     AND NOT coalesce(NEW.tenant_id = app_current_tenant(), false) THEN
    RAISE EXCEPTION 'new row violates row-level security policy for table "%"', TG_TABLE_NAME
      USING ERRCODE = 'insufficient_privilege';
  END IF;
  IF NEW.id IS NULL THEN
    INSERT INTO tenant_id_counters AS c (tenant_id, name, last_id)
    VALUES (NEW.tenant_id, TG_TABLE_NAME, 1)
    ON CONFLICT (tenant_id, name) DO UPDATE SET last_id = c.last_id + 1
    RETURNING c.last_id INTO NEW.id;
  ELSE
    INSERT INTO tenant_id_counters AS c (tenant_id, name, last_id)
    VALUES (NEW.tenant_id, TG_TABLE_NAME, NEW.id)
    ON CONFLICT (tenant_id, name) DO UPDATE SET last_id = greatest(c.last_id, EXCLUDED.last_id);
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- Dropping the identity takes an ACCESS EXCLUSIVE lock that is held until
-- commit, so no row can be inserted between seeding a counter and creating
-- the trigger. The id columns stay NOT NULL.
--
-- ban-alter-identity warns that inserts from running clients may break.
-- They do not here: the app never gives an id (the trigger, created in
-- this same transaction, assigns one), and an explicit id, with or
-- without OVERRIDING SYSTEM VALUE, is still accepted.
-- squawk-ignore ban-alter-identity
ALTER TABLE contents ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE content_revisions ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE categories ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE tags ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE media ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE routes ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE menu_items ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE form_submissions ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE preview_links ALTER COLUMN id DROP IDENTITY;
-- squawk-ignore ban-alter-identity
ALTER TABLE site_builds ALTER COLUMN id DROP IDENTITY;

-- New ids continue after each tenant's highest existing id.
INSERT INTO tenant_id_counters (tenant_id, name, last_id)
SELECT tenant_id, 'contents', max(id) FROM contents GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'content_revisions', max(id) FROM content_revisions GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'categories', max(id) FROM categories GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'tags', max(id) FROM tags GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'media', max(id) FROM media GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'routes', max(id) FROM routes GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'menu_items', max(id) FROM menu_items GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'form_submissions', max(id) FROM form_submissions GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'preview_links', max(id) FROM preview_links GROUP BY tenant_id
UNION ALL SELECT tenant_id, 'site_builds', max(id) FROM site_builds GROUP BY tenant_id;

CREATE TRIGGER contents_assign_id BEFORE INSERT ON contents
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER content_revisions_assign_id BEFORE INSERT ON content_revisions
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER categories_assign_id BEFORE INSERT ON categories
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER tags_assign_id BEFORE INSERT ON tags
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER media_assign_id BEFORE INSERT ON media
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER routes_assign_id BEFORE INSERT ON routes
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER menu_items_assign_id BEFORE INSERT ON menu_items
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER form_submissions_assign_id BEFORE INSERT ON form_submissions
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER preview_links_assign_id BEFORE INSERT ON preview_links
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();
CREATE TRIGGER site_builds_assign_id BEFORE INSERT ON site_builds
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

DROP TRIGGER contents_assign_id ON contents;
DROP TRIGGER content_revisions_assign_id ON content_revisions;
DROP TRIGGER categories_assign_id ON categories;
DROP TRIGGER tags_assign_id ON tags;
DROP TRIGGER media_assign_id ON media;
DROP TRIGGER routes_assign_id ON routes;
DROP TRIGGER menu_items_assign_id ON menu_items;
DROP TRIGGER form_submissions_assign_id ON form_submissions;
DROP TRIGGER preview_links_assign_id ON preview_links;
DROP TRIGGER site_builds_assign_id ON site_builds;

-- One sequence per table again, starting after the highest id of any
-- tenant so it never hands out an id that some tenant already uses.
-- +goose StatementBegin
DO $$
DECLARE
  t text;
  next_id bigint;
BEGIN
  FOREACH t IN ARRAY ARRAY['contents', 'content_revisions', 'categories', 'tags',
    'media', 'routes', 'menu_items', 'form_submissions', 'preview_links', 'site_builds']
  LOOP
    EXECUTE format('SELECT coalesce(max(id), 0) + 1 FROM %I', t) INTO next_id;
    EXECUTE format('ALTER TABLE %I ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (START WITH %s)',
      t, next_id);
  END LOOP;
END
$$;
-- +goose StatementEnd

-- squawk-ignore ban-drop-function
DROP FUNCTION assign_per_tenant_id();
-- squawk-ignore ban-drop-table
DROP TABLE tenant_id_counters;
