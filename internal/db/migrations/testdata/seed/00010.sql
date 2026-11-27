-- Build history: a success with warnings and a failure.

INSERT INTO site_builds (tenant_id, id, started_at, finished_at, files, changed, removed, warnings, error)
OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, '2025-08-01 10:00:00+09', '2025-08-01 10:00:03+09', 42, 42, 0,
    '["menu item 9 has no target"]', NULL),
  ('01900000-0000-7000-8000-00000000000b', 2, '2025-08-01 11:00:00+09', '2025-08-01 11:00:01+09', 0, 0, 0,
    '[]', 'pagefind: exit status 1');
