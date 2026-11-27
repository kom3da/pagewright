-- An invitation waiting to be accepted.

INSERT INTO invitations (tenant_id, id, email, role, token_hash, expires_at, invited_by) VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, 'dan@example.jp', 'editor', sha256('invite-dan'),
    now() + interval '7 days', '01900000-0000-7000-8000-0000000000a1');
