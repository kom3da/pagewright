-- Schema for River's job tables. River creates its tables with its own
-- migrator (pagewright migrate runs it right after goose). Its tables have
-- no tenant_id: the documented exception. Workers set the tenant from the
-- job arguments before touching tenant data.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE SCHEMA IF NOT EXISTS river;
GRANT USAGE ON SCHEMA river TO pagewright_app;
-- Applies to the tables and sequences River's migrator creates later, as
-- long as it runs as the same role as goose (pagewright migrate does).
ALTER DEFAULT PRIVILEGES IN SCHEMA river
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pagewright_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA river
  GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO pagewright_app;

-- +goose Down
ALTER DEFAULT PRIVILEGES IN SCHEMA river
  REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM pagewright_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA river
  REVOKE USAGE, SELECT, UPDATE ON SEQUENCES FROM pagewright_app;
-- squawk-ignore ban-drop-schema
DROP SCHEMA river CASCADE;
