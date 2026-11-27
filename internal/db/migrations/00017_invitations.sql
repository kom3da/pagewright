-- Invitations and members (「メンバー」): site admins invite people by
-- email, change their roles and remove them; users change their own
-- display name.
--
-- An invitation mail carries a secret token. Like a login link it proves
-- the address, so accepting it also logs the person in, creating their
-- user when the address is new. It is looked up by the token's hash before
-- a site is known, and it touches users and memberships beyond what the
-- app may, so accepting goes through a SECURITY DEFINER function, as do
-- the lookup and setting one's own display name (users are shared by all
-- sites; no site may change them).
--
-- Changing a role and removing a member of the current site need no
-- function: RLS on memberships (00001) keeps the app to that site, and the
-- app checks that the caller is an admin of it. Adding a member stays with
-- the function: a membership is what makes a user's profile visible to a
-- site.

-- +goose Up
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE invitations (
  tenant_id uuid NOT NULL DEFAULT app_current_tenant() REFERENCES tenants (id),
  -- Assigned per tenant by the trigger below (see 00013).
  id bigint NOT NULL,
  -- Lower case, like users.email. One invitation per address and site:
  -- inviting the address again replaces it.
  email text NOT NULL CHECK (email = lower(email) AND email <> '' AND char_length(email) <= 254),
  role text NOT NULL CHECK (role IN ('admin', 'editor')),
  -- SHA-256 of the secret token in the mailed link. Every send makes a new
  -- one, so the link of an earlier mail stops working.
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  expires_at timestamptz NOT NULL,
  -- Who sent it last. Cleared when they leave the site.
  invited_by uuid,
  sent_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, email),
  FOREIGN KEY (tenant_id, invited_by) REFERENCES memberships (tenant_id, user_id)
    ON DELETE SET NULL (invited_by)
);
CREATE INDEX invitations_invited_by_idx ON invitations (tenant_id, invited_by);
-- For auth_cleanup().
CREATE INDEX invitations_expires_at_idx ON invitations (expires_at);

CREATE TRIGGER invitations_assign_id BEFORE INSERT ON invitations
  FOR EACH ROW EXECUTE FUNCTION assign_per_tenant_id();

ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invitations
  USING (tenant_id = app_current_tenant())
  WITH CHECK (tenant_id = app_current_tenant());

-- Inviting again and sending again replace the token, role, expiry and
-- sender in place. Accepting deletes the row (in the function).
GRANT SELECT, INSERT, DELETE ON invitations TO pagewright_app;
GRANT UPDATE (role, token_hash, expires_at, invited_by, sent_at) ON invitations TO pagewright_app;

-- Admins change roles and remove members of the current site.
GRANT UPDATE (role) ON memberships TO pagewright_app;
GRANT DELETE ON memberships TO pagewright_app;

-- A site always keeps an admin: nobody else could invite people or change
-- members and settings. Checked here rather than only in the app, so that
-- two admins demoting each other at the same moment cannot both succeed:
-- the site's row is locked first, so such changes run one after the other
-- and the second sees the first (each statement of the function takes a
-- new snapshot in READ COMMITTED, which the app uses). The app takes the
-- same lock before changing a member, so the order of locks is always the
-- site first. Runs as the caller; RLS limits the lock and the check to the
-- current site, which is the only one the app can change.
-- +goose StatementBegin
CREATE FUNCTION memberships_keep_admin() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF OLD.role <> 'admin' OR (TG_OP = 'UPDATE' AND NEW.role = 'admin') THEN
    RETURN NULL;
  END IF;
  PERFORM FROM tenants WHERE id = OLD.tenant_id FOR NO KEY UPDATE;
  IF NOT EXISTS (SELECT FROM memberships WHERE tenant_id = OLD.tenant_id AND role = 'admin') THEN
    RAISE EXCEPTION 'site % would be left without an admin', OLD.tenant_id
      USING ERRCODE = 'check_violation', CONSTRAINT = 'memberships_keep_admin';
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER memberships_keep_admin AFTER UPDATE OF role OR DELETE ON memberships
  FOR EACH ROW EXECUTE FUNCTION memberships_keep_admin();

-- Removing a member keeps what they made: their contents, revisions and
-- images stay on the site, and only the record of who made them is
-- cleared, as 00009 and 00014 already do for preview links and handled
-- messages. Until now the removal was refused while the member had made
-- anything, and an admin could do nothing about it (nothing reassigns
-- authors, and authors are not shown anywhere).
--
-- The constraints are replaced, keeping their names. NOT VALID: every
-- existing row was checked by the constraint being replaced (same columns,
-- same reference), and the ACCESS EXCLUSIVE lock taken here is held until
-- commit, so no unchecked row can exist; validating would only scan the
-- tables under that lock. New and changed rows are checked all the same.
ALTER TABLE contents
  -- squawk-ignore ban-drop-constraint
  DROP CONSTRAINT contents_tenant_id_author_id_fkey,
  ADD CONSTRAINT contents_tenant_id_author_id_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES memberships (tenant_id, user_id) ON DELETE SET NULL (author_id) NOT VALID;
ALTER TABLE content_revisions
  -- squawk-ignore ban-drop-constraint
  DROP CONSTRAINT content_revisions_tenant_id_created_by_fkey,
  ADD CONSTRAINT content_revisions_tenant_id_created_by_fkey FOREIGN KEY (tenant_id, created_by)
    REFERENCES memberships (tenant_id, user_id) ON DELETE SET NULL (created_by) NOT VALID;
ALTER TABLE media
  -- squawk-ignore ban-drop-constraint
  DROP CONSTRAINT media_tenant_id_uploaded_by_fkey,
  ADD CONSTRAINT media_tenant_id_uploaded_by_fkey FOREIGN KEY (tenant_id, uploaded_by)
    REFERENCES memberships (tenant_id, user_id) ON DELETE SET NULL (uploaded_by) NOT VALID;

-- What the invitation page shows before 「参加する」 is pressed. No row for
-- an unknown token: never sent, replaced by a newer mail, cancelled, or
-- already used. An expired invitation is returned, so the page can say
-- so. has_account tells the page whether to ask for a display name; only
-- whoever holds the token (the invited address) learns it.
-- +goose StatementBegin
CREATE FUNCTION invitation_lookup(p_token_hash bytea)
RETURNS TABLE (site_name text, email text, role text, expires_at timestamptz, has_account boolean)
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
  SELECT t.name, i.email, i.role, i.expires_at,
    EXISTS (SELECT FROM public.users u WHERE u.email = i.email)
  FROM public.invitations i
  JOIN public.tenants t ON t.id = i.tenant_id
  WHERE i.token_hash = p_token_hash
$$;
-- +goose StatementEnd

-- Accepts an invitation: adds the invited address to the site with the
-- invitation's role and uses the invitation up. A person who is already a
-- member keeps their role. Returns
--   'joined'         a new session (p_new_session_hash) was opened
--   'joined_current' the caller's session (p_session_hash) moved to the site
--   'invalid'        no such invitation (see invitation_lookup)
--   'expired'        the invitation has expired
--   'wrong_account'  p_session_hash is a live session of another address;
--                    nothing changed
-- With a live session of the invited address, that session moves to the
-- site. Otherwise the link proves the address like a login link does: the
-- user is created when new (named p_display_name), and a new session opens
-- with the site selected.
-- +goose StatementBegin
CREATE FUNCTION auth_accept_invitation(p_token_hash bytea, p_session_hash bytea,
  p_new_session_hash bytea, p_display_name text, p_ttl interval)
RETURNS text
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
DECLARE
  v_inv public.invitations;
  v_session_user uuid;
  v_user uuid;
BEGIN
  IF char_length(p_display_name) > 100 THEN
    RAISE EXCEPTION 'display name too long' USING ERRCODE = 'check_violation';
  END IF;
  SELECT * INTO v_inv FROM public.invitations WHERE token_hash = p_token_hash FOR UPDATE;
  IF NOT FOUND THEN
    RETURN 'invalid';
  END IF;
  IF v_inv.expires_at <= now() THEN
    RETURN 'expired';
  END IF;

  SELECT s.user_id INTO v_session_user FROM public.sessions s
  WHERE s.token_hash = p_session_hash AND s.expires_at > now();
  IF v_session_user IS NOT NULL THEN
    IF NOT EXISTS (SELECT FROM public.users WHERE id = v_session_user AND email = v_inv.email) THEN
      RETURN 'wrong_account';
    END IF;
    v_user := v_session_user;
  ELSE
    SELECT id INTO v_user FROM public.users WHERE email = v_inv.email;
    IF v_user IS NULL THEN
      INSERT INTO public.users (email, display_name)
      VALUES (v_inv.email, coalesce(p_display_name, ''))
      ON CONFLICT (email) DO NOTHING
      RETURNING id INTO v_user;
      -- Made at the same moment by an invitation to another site.
      IF v_user IS NULL THEN
        SELECT id INTO v_user FROM public.users WHERE email = v_inv.email;
      END IF;
    END IF;
  END IF;

  INSERT INTO public.memberships (tenant_id, user_id, role)
  VALUES (v_inv.tenant_id, v_user, v_inv.role)
  ON CONFLICT (tenant_id, user_id) DO NOTHING;
  DELETE FROM public.invitations WHERE tenant_id = v_inv.tenant_id AND id = v_inv.id;

  IF v_session_user IS NOT NULL THEN
    UPDATE public.sessions SET tenant_id = v_inv.tenant_id WHERE token_hash = p_session_hash;
    RETURN 'joined_current';
  END IF;
  INSERT INTO public.sessions (token_hash, user_id, tenant_id, expires_at)
  VALUES (p_new_session_hash, v_user, v_inv.tenant_id, now() + p_ttl);
  RETURN 'joined';
END
$$;
-- +goose StatementEnd

-- Sets the display name of the session's user. Returns false for an
-- unknown or expired session. The app checks the name; this only keeps it
-- short.
-- +goose StatementBegin
CREATE FUNCTION auth_set_display_name(p_token_hash bytea, p_display_name text)
RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
BEGIN
  IF p_display_name IS NULL OR char_length(p_display_name) > 100 THEN
    RAISE EXCEPTION 'display name missing or too long' USING ERRCODE = 'check_violation';
  END IF;
  UPDATE public.users u SET display_name = p_display_name
  FROM public.sessions s
  WHERE s.token_hash = p_token_hash AND s.expires_at > now() AND u.id = s.user_id;
  RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- Expired invitations stay listed (as 期限切れ, to send again) for 30 days,
-- then go with the hourly cleanup.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION auth_cleanup()
RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
AS $$
DECLARE
  v_tokens bigint;
  v_sessions bigint;
  v_previews bigint;
  v_invitations bigint;
BEGIN
  DELETE FROM public.login_tokens WHERE expires_at < now() - interval '1 day';
  GET DIAGNOSTICS v_tokens = ROW_COUNT;
  DELETE FROM public.sessions WHERE expires_at < now();
  GET DIAGNOSTICS v_sessions = ROW_COUNT;
  DELETE FROM public.preview_links WHERE expires_at < now();
  GET DIAGNOSTICS v_previews = ROW_COUNT;
  DELETE FROM public.invitations WHERE expires_at < now() - interval '30 days';
  GET DIAGNOSTICS v_invitations = ROW_COUNT;
  RETURN v_tokens + v_sessions + v_previews + v_invitations;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION
  invitation_lookup(bytea),
  auth_accept_invitation(bytea, bytea, bytea, text, interval),
  auth_set_display_name(bytea, text)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
  invitation_lookup(bytea),
  auth_accept_invitation(bytea, bytea, bytea, text, interval),
  auth_set_display_name(bytea, text)
  TO pagewright_app;

-- +goose Down
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- auth_cleanup as 00009 made it (with 00012's search_path).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION auth_cleanup()
RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER
SET search_path = pg_catalog, public, pg_temp
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

-- squawk-ignore ban-drop-function
DROP FUNCTION invitation_lookup(bytea),
  auth_accept_invitation(bytea, bytea, bytea, text, interval),
  auth_set_display_name(bytea, text);

-- Removing an author is refused again. Rows whose author was cleared meanwhile
-- have NULL, which the constraints accept.
ALTER TABLE media
  -- squawk-ignore ban-drop-constraint
  DROP CONSTRAINT media_tenant_id_uploaded_by_fkey,
  ADD CONSTRAINT media_tenant_id_uploaded_by_fkey FOREIGN KEY (tenant_id, uploaded_by)
    REFERENCES memberships (tenant_id, user_id) NOT VALID;
ALTER TABLE content_revisions
  -- squawk-ignore ban-drop-constraint
  DROP CONSTRAINT content_revisions_tenant_id_created_by_fkey,
  ADD CONSTRAINT content_revisions_tenant_id_created_by_fkey FOREIGN KEY (tenant_id, created_by)
    REFERENCES memberships (tenant_id, user_id) NOT VALID;
ALTER TABLE contents
  -- squawk-ignore ban-drop-constraint
  DROP CONSTRAINT contents_tenant_id_author_id_fkey,
  ADD CONSTRAINT contents_tenant_id_author_id_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES memberships (tenant_id, user_id) NOT VALID;

DROP TRIGGER memberships_keep_admin ON memberships;
-- squawk-ignore ban-drop-function
DROP FUNCTION memberships_keep_admin();
REVOKE DELETE ON memberships FROM pagewright_app;
REVOKE UPDATE (role) ON memberships FROM pagewright_app;

-- squawk-ignore ban-drop-table
DROP TABLE invitations;
