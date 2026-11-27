-- Preview links: one valid, one expired.

INSERT INTO preview_links (tenant_id, id, token_hash, content_id, expires_at, created_by)
OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, sha256('preview-renewal'), 6, now() + interval '7 days',
    '01900000-0000-7000-8000-0000000000c1'),
  ('01900000-0000-7000-8000-00000000000a', 2, sha256('preview-expired'), 7, now() - interval '1 day',
    '01900000-0000-7000-8000-0000000000a1');
