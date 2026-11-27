-- Login links (one used, one still valid) and sessions (alice on site A,
-- carol before choosing a site, and an expired one).

INSERT INTO login_tokens (token_hash, user_id, expires_at, used_at) VALUES
  (sha256('used-link'), '01900000-0000-7000-8000-0000000000a1', now() + interval '10 minutes', now()),
  (sha256('open-link'), '01900000-0000-7000-8000-0000000000b1', now() + interval '10 minutes', NULL);

INSERT INTO sessions (token_hash, user_id, tenant_id, expires_at) VALUES
  (sha256('alice-session'), '01900000-0000-7000-8000-0000000000a1', '01900000-0000-7000-8000-00000000000a', now() + interval '30 days'),
  (sha256('carol-session'), '01900000-0000-7000-8000-0000000000c1', NULL, now() + interval '30 days'),
  (sha256('old-session'), '01900000-0000-7000-8000-0000000000a1', '01900000-0000-7000-8000-00000000000a', now() - interval '1 day');
