-- Site settings (colors, front page, logo) and menus.

UPDATE tenants SET
  settings = '{"primaryColor":"#1b5e20","font":"mincho","timeZone":"Asia/Tokyo","contactEmail":"info@example.jp","frontPageId":1}',
  logo_media_id = 1
WHERE id = '01900000-0000-7000-8000-00000000000a';

UPDATE tenants SET settings = '{"footerColor":"#3e2723"}'
WHERE id = '01900000-0000-7000-8000-00000000000b';

INSERT INTO menu_items (tenant_id, id, position, label, content_id, url) OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, 0, NULL, 1, NULL),
  ('01900000-0000-7000-8000-00000000000b', 2, 0, NULL, 2, NULL),
  ('01900000-0000-7000-8000-00000000000a', 3, 1, 'アクセス', 3, NULL),
  ('01900000-0000-7000-8000-00000000000a', 4, 2, '採用情報', NULL, 'https://recruit.example.jp/');
