-- Data as an install at migration 00001 would have it: two sites sharing
-- users, pages with a parent, posts imported from WordPress (numbers above
-- 100, source body kept), a Japanese URL, a redirect, a draft, a scheduled
-- post and a published post with a newer draft. Ids come from sequences
-- shared by all tenants at this version, so they interleave.
-- Runs as the owner (outside RLS).

INSERT INTO tenants (id, name) VALUES
  ('01900000-0000-7000-8000-00000000000a', 'みどり野設備株式会社'),
  ('01900000-0000-7000-8000-00000000000b', '喫茶こもれび');

INSERT INTO users (id, email, display_name) VALUES
  ('01900000-0000-7000-8000-0000000000a1', 'alice@example.jp', '山田 花子'),
  ('01900000-0000-7000-8000-0000000000b1', 'bob@example.jp', '佐藤 太郎'),
  ('01900000-0000-7000-8000-0000000000c1', 'carol@example.jp', '制作会社 鈴木');

INSERT INTO memberships (tenant_id, user_id, role) VALUES
  ('01900000-0000-7000-8000-00000000000a', '01900000-0000-7000-8000-0000000000a1', 'admin'),
  ('01900000-0000-7000-8000-00000000000a', '01900000-0000-7000-8000-0000000000c1', 'editor'),
  ('01900000-0000-7000-8000-00000000000b', '01900000-0000-7000-8000-0000000000b1', 'admin'),
  ('01900000-0000-7000-8000-00000000000b', '01900000-0000-7000-8000-0000000000c1', 'admin');

INSERT INTO media (tenant_id, id, storage_key, filename, content_type, size_bytes, width, height, alt_text, uploaded_by)
OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, '2025/04/office.jpg', 'office.jpg', 'image/jpeg', 9, 1200, 800, '本社の外観', '01900000-0000-7000-8000-0000000000a1'),
  ('01900000-0000-7000-8000-00000000000b', 2, '2025/05/latte.jpg', 'latte.jpg', 'image/jpeg', 9, 800, 800, 'ラテアート', '01900000-0000-7000-8000-0000000000b1');

INSERT INTO categories (tenant_id, id, parent_id, name, slug, position) OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, NULL, 'お知らせ', 'news', 0),
  ('01900000-0000-7000-8000-00000000000b', 2, NULL, 'ご案内', 'info', 0),
  ('01900000-0000-7000-8000-00000000000a', 3, 1, 'イベント', 'event', 0);

INSERT INTO tags (tenant_id, id, name, slug) OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, 'プレスリリース', 'pr'),
  ('01900000-0000-7000-8000-00000000000b', 2, 'コーヒー', 'coffee');

INSERT INTO contents (tenant_id, id, number, type, slug, parent_id, author_id, source_format, source_body)
OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, 1, 'page', 'company', NULL, '01900000-0000-7000-8000-0000000000a1', NULL, NULL),
  ('01900000-0000-7000-8000-00000000000b', 2, 1, 'page', 'menu', NULL, '01900000-0000-7000-8000-0000000000b1', NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 3, 2, 'page', 'access', 1, '01900000-0000-7000-8000-0000000000a1', NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 4, 101, 'post', 'founding', NULL, '01900000-0000-7000-8000-0000000000a1',
    'wordpress', '<!-- wp:paragraph --><p>創業しました。</p><!-- /wp:paragraph -->[gallery ids="5,6"]'),
  ('01900000-0000-7000-8000-00000000000b', 5, 2, 'post', NULL, NULL, '01900000-0000-7000-8000-0000000000c1', NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 6, 102, 'post', 'renewal', NULL, '01900000-0000-7000-8000-0000000000c1', NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 7, 103, 'post', 'summer', NULL, '01900000-0000-7000-8000-0000000000a1', NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 8, 104, 'page', '沿革', NULL, '01900000-0000-7000-8000-0000000000a1', NULL, NULL);

INSERT INTO content_revisions (tenant_id, id, content_id, title, body, body_schema_version, created_by, created_at)
OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, 1, '会社概要',
    '{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"会社概要"}]},{"type":"paragraph","content":[{"type":"text","text":"緑野","marks":[{"type":"ruby","attrs":{"rt":"みどりの"}}]},{"type":"text","text":"の設備会社です。"}]}]}',
    1, '01900000-0000-7000-8000-0000000000a1', '2025-04-01 09:00+09'),
  ('01900000-0000-7000-8000-00000000000b', 2, 2, 'メニュー',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"ブレンド 500円"}]}]}',
    1, '01900000-0000-7000-8000-0000000000b1', '2025-05-01 09:00+09'),
  ('01900000-0000-7000-8000-00000000000a', 3, 3, 'アクセス',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"駅から徒歩5分です。"}]}]}',
    1, '01900000-0000-7000-8000-0000000000a1', '2025-04-01 09:10+09'),
  ('01900000-0000-7000-8000-00000000000a', 4, 4, '創業のご挨拶',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"創業しました。"}]},{"type":"image","attrs":{"src":"/media/2025/04/office.jpg","alt":"本社の外観","mediaId":1}}]}',
    1, '01900000-0000-7000-8000-0000000000a1', '2025-04-02 09:00+09'),
  ('01900000-0000-7000-8000-00000000000b', 5, 5, '営業時間の変更',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"日曜日は休みます。"}]}]}',
    1, '01900000-0000-7000-8000-0000000000c1', '2025-05-02 09:00+09'),
  ('01900000-0000-7000-8000-00000000000a', 6, 4, '創業のご挨拶（改訂中）',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"書きかけの本文"}]}]}',
    1, '01900000-0000-7000-8000-0000000000c1', '2025-06-01 09:00+09'),
  ('01900000-0000-7000-8000-00000000000a', 7, 6, 'リニューアルのお知らせ',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"下書きです。"}]}]}',
    1, '01900000-0000-7000-8000-0000000000c1', '2025-06-02 09:00+09'),
  ('01900000-0000-7000-8000-00000000000a', 8, 7, '夏季休業のお知らせ',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"8月は休みます。"}]}]}',
    1, '01900000-0000-7000-8000-0000000000a1', '2025-06-03 09:00+09'),
  ('01900000-0000-7000-8000-00000000000a', 9, 8, '沿革',
    '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"1980年 創業"}]}]}',
    1, '01900000-0000-7000-8000-0000000000a1', '2025-04-03 09:00+09');

UPDATE contents c SET published_revision_id = v.rev, published_at = v.at::timestamptz
FROM (VALUES
  ('01900000-0000-7000-8000-00000000000a'::uuid, 1, 1, '2025-04-01 10:00+09'),
  ('01900000-0000-7000-8000-00000000000b'::uuid, 2, 2, '2025-05-01 10:00+09'),
  ('01900000-0000-7000-8000-00000000000a'::uuid, 3, 3, '2025-04-01 10:00+09'),
  ('01900000-0000-7000-8000-00000000000a'::uuid, 4, 4, '2025-04-02 10:00+09'),
  ('01900000-0000-7000-8000-00000000000b'::uuid, 5, 5, '2025-05-02 10:00+09'),
  ('01900000-0000-7000-8000-00000000000a'::uuid, 8, 9, '2025-04-03 10:00+09')
) AS v (tenant, id, rev, at)
WHERE c.tenant_id = v.tenant AND c.id = v.id;

UPDATE contents SET scheduled_revision_id = 8, scheduled_at = '2099-07-01 09:00+09'
WHERE tenant_id = '01900000-0000-7000-8000-00000000000a' AND id = 7;

INSERT INTO content_categories (tenant_id, content_id, category_id) VALUES
  ('01900000-0000-7000-8000-00000000000a', 4, 1),
  ('01900000-0000-7000-8000-00000000000a', 4, 3),
  ('01900000-0000-7000-8000-00000000000b', 5, 2);

INSERT INTO content_tags (tenant_id, content_id, tag_id) VALUES
  ('01900000-0000-7000-8000-00000000000a', 4, 1),
  ('01900000-0000-7000-8000-00000000000b', 5, 2);

INSERT INTO routes (tenant_id, id, path, kind, content_id, category_id, tag_id) OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, '/company/', 'canonical', 1, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000b', 2, '/menu/', 'canonical', 2, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 3, '/company/access/', 'canonical', 3, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 4, '/posts/founding/', 'canonical', 4, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000b', 5, '/posts/2/', 'canonical', 5, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 6, '/沿革/', 'canonical', 8, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 7, '/about/', 'redirect', 1, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 8, '/2025/04/創業のご挨拶/', 'redirect', 4, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 9, '/posts/category/news/', 'canonical', NULL, 1, NULL),
  ('01900000-0000-7000-8000-00000000000b', 10, '/posts/category/info/', 'canonical', NULL, 2, NULL),
  ('01900000-0000-7000-8000-00000000000a', 11, '/posts/category/event/', 'canonical', NULL, 3, NULL),
  ('01900000-0000-7000-8000-00000000000a', 12, '/posts/tag/pr/', 'canonical', NULL, NULL, 1),
  ('01900000-0000-7000-8000-00000000000b', 13, '/posts/tag/coffee/', 'canonical', NULL, NULL, 2),
  ('01900000-0000-7000-8000-00000000000a', 14, '/posts/renewal/', 'canonical', 6, NULL, NULL),
  ('01900000-0000-7000-8000-00000000000a', 15, '/posts/summer/', 'canonical', 7, NULL, NULL);
