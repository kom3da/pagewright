-- Login-free preview links: anyone with the link sees the latest revision
-- of one content until the link expires.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE preview_links (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  id bigint GENERATED ALWAYS AS IDENTITY,
  -- SHA-256 of the secret token in the URL.
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  content_id bigint NOT NULL,
  expires_at timestamptz NOT NULL,
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id, content_id) REFERENCES contents (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, created_by) REFERENCES memberships (tenant_id, user_id)
    ON DELETE SET NULL (created_by)
);
CREATE INDEX preview_links_content_idx ON preview_links (tenant_id, content_id);
CREATE INDEX preview_links_created_by_idx ON preview_links (tenant_id, created_by);
CREATE INDEX preview_links_expires_at_idx ON preview_links (expires_at);

ALTER TABLE preview_links ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON preview_links
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

GRANT SELECT, INSERT, DELETE ON preview_links TO pagewright_app;

-- Resolves a link before the tenant is known. Only a live token yields a
-- row, and only the ids needed to render it.
-- +goose StatementBegin
CREATE FUNCTION preview_link_lookup(p_token_hash bytea)
RETURNS TABLE (tenant_id uuid, content_id bigint, expires_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
  SELECT l.tenant_id, l.content_id, l.expires_at FROM public.preview_links l
  WHERE l.token_hash = p_token_hash AND l.expires_at > now()
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION preview_link_lookup(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION preview_link_lookup(bytea) TO pagewright_app;

-- Expired preview links go with the hourly cleanup too.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION auth_cleanup()
RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  v_tokens bigint;
  v_sessions bigint;
  v_previews bigint;
BEGIN
  DELETE FROM public.login_tokens WHERE expires_at < now() - interval '1 day';
  GET DIAGNOSTICS v_tokens = ROW_COUNT;
  DELETE FROM public.sessions WHERE expires_at < now();
  GET DIAGNOSTICS v_sessions = ROW_COUNT;
  DELETE FROM public.preview_links WHERE expires_at < now();
  GET DIAGNOSTICS v_previews = ROW_COUNT;
  RETURN v_tokens + v_sessions + v_previews;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION auth_cleanup()
RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  v_tokens bigint;
  v_sessions bigint;
BEGIN
  DELETE FROM public.login_tokens WHERE expires_at < now() - interval '1 day';
  GET DIAGNOSTICS v_tokens = ROW_COUNT;
  DELETE FROM public.sessions WHERE expires_at < now();
  GET DIAGNOSTICS v_sessions = ROW_COUNT;
  RETURN v_tokens + v_sessions;
END
$$;
-- +goose StatementEnd
-- squawk-ignore ban-drop-function
DROP FUNCTION preview_link_lookup(bytea);
-- squawk-ignore ban-drop-table
DROP TABLE preview_links;
