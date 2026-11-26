-- Indexes for auth_cleanup(). Built concurrently so logins keep working,
-- which cannot run inside a transaction. A failed concurrent build leaves
-- an invalid index behind, so each one is dropped first: re-running the
-- migration then rebuilds it.

-- +goose NO TRANSACTION

-- +goose Up
SET lock_timeout = '5s';
SET statement_timeout = '10min';
-- squawk-ignore ban-concurrent-index-creation-in-transaction
DROP INDEX CONCURRENTLY IF EXISTS login_tokens_expires_at_idx;
-- squawk-ignore ban-concurrent-index-creation-in-transaction
CREATE INDEX CONCURRENTLY IF NOT EXISTS login_tokens_expires_at_idx ON login_tokens (expires_at);
-- squawk-ignore ban-concurrent-index-creation-in-transaction
DROP INDEX CONCURRENTLY IF EXISTS sessions_expires_at_idx;
-- squawk-ignore ban-concurrent-index-creation-in-transaction
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);
RESET lock_timeout;
RESET statement_timeout;

-- +goose Down
SET lock_timeout = '5s';
-- squawk-ignore ban-concurrent-index-creation-in-transaction
DROP INDEX CONCURRENTLY IF EXISTS sessions_expires_at_idx;
-- squawk-ignore ban-concurrent-index-creation-in-transaction
DROP INDEX CONCURRENTLY IF EXISTS login_tokens_expires_at_idx;
RESET lock_timeout;
