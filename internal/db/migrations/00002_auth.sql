-- Login links and sessions.
--
-- Both are looked up before a tenant is known, so the app gets no table
-- grants at all. It goes through the SECURITY DEFINER functions below,
-- which take a SHA-256 hash of a secret token: without a valid token the
-- app cannot read anyone's session, sites or profile.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE login_tokens (
  token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_tokens_user_id_idx ON login_tokens (user_id);

CREATE TABLE sessions (
  token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- The site chosen after login; NULL until chosen.
  tenant_id uuid,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  -- Losing the membership ends access to that site.
  FOREIGN KEY (tenant_id, user_id) REFERENCES memberships (tenant_id, user_id)
    ON DELETE SET NULL (tenant_id)
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_tenant_user_idx ON sessions (tenant_id, user_id);

-- No policies: nobody but the owner (and so the functions) can touch rows.
ALTER TABLE login_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;

-- Stores a login token for the user with this email. Returns false when no
-- such user exists (the API answers the same either way).
-- +goose StatementBegin
CREATE FUNCTION auth_request_login(p_email text, p_token_hash bytea, p_ttl interval)
RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  v_user uuid;
BEGIN
  SELECT id INTO v_user FROM public.users WHERE email = lower(p_email);
  IF v_user IS NULL THEN
    RETURN false;
  END IF;
  INSERT INTO public.login_tokens (token_hash, user_id, expires_at)
  VALUES (p_token_hash, v_user, now() + p_ttl);
  RETURN true;
END
$$;
-- +goose StatementEnd

-- Uses a login token once and opens a session. When the user belongs to
-- exactly one site, that site is selected. Returns NULL for an unknown,
-- used or expired token.
-- +goose StatementBegin
CREATE FUNCTION auth_login(p_login_token_hash bytea, p_session_token_hash bytea, p_ttl interval)
RETURNS uuid
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  v_user uuid;
  v_tenant uuid;
BEGIN
  UPDATE public.login_tokens SET used_at = now()
  WHERE token_hash = p_login_token_hash AND used_at IS NULL AND expires_at > now()
  RETURNING user_id INTO v_user;
  IF v_user IS NULL THEN
    RETURN NULL;
  END IF;
  SELECT CASE WHEN count(*) = 1 THEN min(tenant_id::text)::uuid END INTO v_tenant
  FROM public.memberships WHERE user_id = v_user;
  INSERT INTO public.sessions (token_hash, user_id, tenant_id, expires_at)
  VALUES (p_session_token_hash, v_user, v_tenant, now() + p_ttl);
  RETURN v_user;
END
$$;
-- +goose StatementEnd

-- The user and selected site of a live session; no row when the session
-- is unknown or expired. role is NULL while no site is selected.
-- +goose StatementBegin
CREATE FUNCTION auth_session(p_token_hash bytea)
RETURNS TABLE (user_id uuid, email text, display_name text, tenant_id uuid, role text)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
  SELECT u.id, u.email, u.display_name, s.tenant_id, m.role
  FROM public.sessions s
  JOIN public.users u ON u.id = s.user_id
  LEFT JOIN public.memberships m ON m.tenant_id = s.tenant_id AND m.user_id = s.user_id
  WHERE s.token_hash = p_token_hash AND s.expires_at > now()
$$;
-- +goose StatementEnd

-- The sites the session's user belongs to.
-- +goose StatementBegin
CREATE FUNCTION auth_session_sites(p_token_hash bytea)
RETURNS TABLE (tenant_id uuid, name text, role text)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
  SELECT t.id, t.name, m.role
  FROM public.sessions s
  JOIN public.memberships m ON m.user_id = s.user_id
  JOIN public.tenants t ON t.id = m.tenant_id
  WHERE s.token_hash = p_token_hash AND s.expires_at > now()
  ORDER BY t.name, t.id
$$;
-- +goose StatementEnd

-- Selects a site for the session. Returns false unless the user is a
-- member of it.
-- +goose StatementBegin
CREATE FUNCTION auth_select_site(p_token_hash bytea, p_tenant_id uuid)
RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
  UPDATE public.sessions s SET tenant_id = p_tenant_id
  WHERE s.token_hash = p_token_hash AND s.expires_at > now()
    AND EXISTS (
      SELECT FROM public.memberships m
      WHERE m.user_id = s.user_id AND m.tenant_id = p_tenant_id
    );
  RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION auth_logout(p_token_hash bytea)
RETURNS void
LANGUAGE sql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
  DELETE FROM public.sessions WHERE token_hash = p_token_hash
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION
  auth_request_login(text, bytea, interval),
  auth_login(bytea, bytea, interval),
  auth_session(bytea),
  auth_session_sites(bytea),
  auth_select_site(bytea, uuid),
  auth_logout(bytea)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
  auth_request_login(text, bytea, interval),
  auth_login(bytea, bytea, interval),
  auth_session(bytea),
  auth_session_sites(bytea),
  auth_select_site(bytea, uuid),
  auth_logout(bytea)
  TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-function
DROP FUNCTION auth_request_login(text, bytea, interval), auth_login(bytea, bytea, interval),
  auth_session(bytea), auth_session_sites(bytea), auth_select_site(bytea, uuid),
  auth_logout(bytea);
-- squawk-ignore ban-drop-table
DROP TABLE sessions, login_tokens;
