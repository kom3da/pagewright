-- Removes expired login links and sessions. The app has no grants on the
-- auth tables, so it runs this SECURITY DEFINER function from a periodic job.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- +goose StatementBegin
CREATE FUNCTION auth_cleanup()
RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
  v_tokens bigint;
  v_sessions bigint;
BEGIN
  -- Keep used and expired links for a day, for troubleshooting.
  DELETE FROM public.login_tokens WHERE expires_at < now() - interval '1 day';
  GET DIAGNOSTICS v_tokens = ROW_COUNT;
  DELETE FROM public.sessions WHERE expires_at < now();
  GET DIAGNOSTICS v_sessions = ROW_COUNT;
  RETURN v_tokens + v_sessions;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION auth_cleanup() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_cleanup() TO pagewright_app;

-- +goose Down
-- squawk-ignore ban-drop-function
DROP FUNCTION auth_cleanup();
