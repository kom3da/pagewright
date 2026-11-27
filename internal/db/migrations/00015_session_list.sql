-- Lets users see where they are logged in and log out other devices
-- (「ほかの端末からログアウト」).
--
-- Like the rest of 00002, the app reaches sessions only through these
-- SECURITY DEFINER functions, keyed by the hash of the caller's own
-- session token: a user can list and end only their own sessions.
--
-- A session is named to the browser by sha256(token_hash): a hash of the
-- stored hash, so the stored hash (and the token) never leaves the
-- database, and nothing new has to be stored or backfilled.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- last_used_at: when the session was last used, to the hour. The app
-- records a use only when the recorded one is over an hour old (and the
-- function below refuses more), so requests do not each write a row. NULL
-- until the first use is recorded.
--
-- device: a short label the app makes from the User-Agent ("Windows の
-- Chrome"), so the user can tell sessions apart. The User-Agent itself and
-- the IP address are not stored: they are personal data, and the list
-- needs neither.
ALTER TABLE sessions
  ADD COLUMN last_used_at timestamptz,
  ADD COLUMN device text NOT NULL DEFAULT '' CHECK (char_length(device) <= 100);

-- auth_session also returns last_used_at, so the app knows from the lookup
-- it makes anyway whether a use is due to be recorded. Dropped and created in this transaction; the
-- new columns come last, so queries of the old ones keep working.
-- squawk-ignore ban-drop-function
DROP FUNCTION auth_session(bytea);
-- +goose StatementBegin
CREATE FUNCTION auth_session(p_token_hash bytea)
RETURNS TABLE (user_id uuid, email text, display_name text, tenant_id uuid, role text,
  last_used_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
  SELECT u.id, u.email, u.display_name, s.tenant_id, m.role, s.last_used_at
  FROM public.sessions s
  JOIN public.users u ON u.id = s.user_id
  LEFT JOIN public.memberships m ON m.tenant_id = s.tenant_id AND m.user_id = s.user_id
  WHERE s.token_hash = p_token_hash AND s.expires_at > now()
$$;
-- +goose StatementEnd

-- Records a use of a live session and the device it came from, at most
-- once an hour. The expiry stays as it is (30 days from login).
-- +goose StatementBegin
CREATE FUNCTION auth_touch_session(p_token_hash bytea, p_device text)
RETURNS void
LANGUAGE sql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
  UPDATE public.sessions SET last_used_at = now(), device = left(p_device, 100)
  WHERE token_hash = p_token_hash AND expires_at > now()
    AND (last_used_at IS NULL OR last_used_at < now() - interval '1 hour')
$$;
-- +goose StatementEnd

-- The live sessions of the caller's user, the caller's own first. id is
-- the session's name for the browser (see the top). A session whose use
-- was never recorded (made before this migration) was last used, as far as
-- is known, when it was made.
-- +goose StatementBegin
CREATE FUNCTION auth_list_sessions(p_token_hash bytea)
RETURNS TABLE (id bytea, device text, created_at timestamptz, last_used_at timestamptz,
  expires_at timestamptz, is_current boolean)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
  SELECT sha256(s.token_hash), s.device, s.created_at, coalesce(s.last_used_at, s.created_at),
    s.expires_at, s.token_hash = c.token_hash
  FROM public.sessions c
  JOIN public.sessions s ON s.user_id = c.user_id
  WHERE c.token_hash = p_token_hash AND c.expires_at > now() AND s.expires_at > now()
  ORDER BY s.token_hash = c.token_hash DESC, coalesce(s.last_used_at, s.created_at) DESC,
    s.created_at DESC
$$;
-- +goose StatementEnd

-- Ends another session of the caller's user. Returns false when there is
-- no such session (another user's, the caller's own, or none): the
-- caller's own ends with auth_logout.
-- +goose StatementBegin
CREATE FUNCTION auth_revoke_session(p_token_hash bytea, p_session_id bytea)
RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
BEGIN
  DELETE FROM public.sessions s
  USING public.sessions c
  WHERE c.token_hash = p_token_hash AND c.expires_at > now()
    AND s.user_id = c.user_id AND s.token_hash <> c.token_hash
    AND sha256(s.token_hash) = p_session_id;
  RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- Ends every other session of the caller's user and returns how many.
-- +goose StatementBegin
CREATE FUNCTION auth_revoke_other_sessions(p_token_hash bytea)
RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
DECLARE
  v_count bigint;
BEGIN
  DELETE FROM public.sessions s
  USING public.sessions c
  WHERE c.token_hash = p_token_hash AND c.expires_at > now()
    AND s.user_id = c.user_id AND s.token_hash <> c.token_hash;
  GET DIAGNOSTICS v_count = ROW_COUNT;
  RETURN v_count;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION
  auth_session(bytea),
  auth_touch_session(bytea, text),
  auth_list_sessions(bytea),
  auth_revoke_session(bytea, bytea),
  auth_revoke_other_sessions(bytea)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
  auth_session(bytea),
  auth_touch_session(bytea, text),
  auth_list_sessions(bytea),
  auth_revoke_session(bytea, bytea),
  auth_revoke_other_sessions(bytea)
  TO pagewright_app;

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- squawk-ignore ban-drop-function
DROP FUNCTION auth_touch_session(bytea, text), auth_list_sessions(bytea),
  auth_revoke_session(bytea, bytea), auth_revoke_other_sessions(bytea);

-- auth_session as 00002 made it (with 00012's search_path).
-- squawk-ignore ban-drop-function
DROP FUNCTION auth_session(bytea);
-- +goose StatementBegin
CREATE FUNCTION auth_session(p_token_hash bytea)
RETURNS TABLE (user_id uuid, email text, display_name text, tenant_id uuid, role text)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
  SELECT u.id, u.email, u.display_name, s.tenant_id, m.role
  FROM public.sessions s
  JOIN public.users u ON u.id = s.user_id
  LEFT JOIN public.memberships m ON m.tenant_id = s.tenant_id AND m.user_id = s.user_id
  WHERE s.token_hash = p_token_hash AND s.expires_at > now()
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION auth_session(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_session(bytea) TO pagewright_app;

-- squawk-ignore ban-drop-column
ALTER TABLE sessions DROP COLUMN device, DROP COLUMN last_used_at;
