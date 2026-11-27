-- A published post with a URL and tag change waiting to be published.

UPDATE contents SET pending_settings = '{"slug":"founding-2025","tagIds":[]}'
WHERE tenant_id = '01900000-0000-7000-8000-00000000000a' AND id = 4;
