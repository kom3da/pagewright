-- Validates the logo foreign key added NOT VALID in 00006.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
ALTER TABLE tenants VALIDATE CONSTRAINT tenants_logo_fkey;

-- +goose Down
-- Nothing to undo: 00006's down drops the constraint.
SELECT 1;
