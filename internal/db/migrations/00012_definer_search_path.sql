-- SECURITY DEFINER functions put pg_temp last in their search_path. Every
-- reference in them is schema-qualified, so this changes nothing today; it
-- keeps a caller's temporary objects from ever shadowing anything if an
-- unqualified name slips in later (PostgreSQL searches pg_temp first unless
-- it is listed).
--
-- The functions are owned by the migration runner that created them, the
-- same role that runs this migration, so it may alter them.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER FUNCTION find_user_by_email(text) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_request_login(text, bytea, interval) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_login(bytea, bytea, interval) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_session(bytea) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_session_sites(bytea) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_select_site(bytea, uuid) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_logout(bytea) SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION auth_cleanup() SET search_path = pg_catalog, public, pg_temp;
ALTER FUNCTION preview_link_lookup(bytea) SET search_path = pg_catalog, public, pg_temp;

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

ALTER FUNCTION find_user_by_email(text) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_request_login(text, bytea, interval) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_login(bytea, bytea, interval) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_session(bytea) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_session_sites(bytea) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_select_site(bytea, uuid) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_logout(bytea) SET search_path = pg_catalog, public;
ALTER FUNCTION auth_cleanup() SET search_path = pg_catalog, public;
ALTER FUNCTION preview_link_lookup(bytea) SET search_path = pg_catalog, public;
