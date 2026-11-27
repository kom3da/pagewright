-- Whether a contact form message has been dealt with (未対応 / 対応済み),
-- so a site with several admins can tell what still needs a reply.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- When the message was marked handled, and by which member; both NULL
-- while it is unhandled. Existing messages start unhandled.
--
-- handled_by points at the membership, so it can only name a member of
-- the same site. Removing the member clears it (the message stays
-- handled, by someone no longer on the site).
--
-- NOT VALID: the column is new and NULL in every existing row, so there is
-- nothing to check and no reason to scan the table under the lock; new and
-- changed rows are checked all the same.
--
-- No index on (tenant_id, handled_by) for that ON DELETE: removing a member
-- is rare and the update it runs is limited to one site's rows by the
-- primary key. Nor on handled_at: the list and the home's count read one
-- site's messages, newest first, through the primary key.
ALTER TABLE form_submissions
  ADD COLUMN handled_at timestamptz,
  ADD COLUMN handled_by uuid,
  ADD CONSTRAINT form_submissions_handled_by_fkey FOREIGN KEY (tenant_id, handled_by)
    REFERENCES memberships (tenant_id, user_id) ON DELETE SET NULL (handled_by) NOT VALID;

-- The message itself stays unchangeable (personal data, as sent); only
-- its handled state is.
GRANT UPDATE (handled_at, handled_by) ON form_submissions TO pagewright_app;

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

REVOKE UPDATE (handled_at, handled_by) ON form_submissions FROM pagewright_app;
-- Dropping handled_by drops its foreign key.
-- squawk-ignore ban-drop-column
ALTER TABLE form_submissions DROP COLUMN handled_by, DROP COLUMN handled_at;
