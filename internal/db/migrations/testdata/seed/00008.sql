-- Inquiries from the contact form.

INSERT INTO form_submissions (tenant_id, id, name, kana, email, phone, message, page_path, created_at)
OVERRIDING SYSTEM VALUE VALUES
  ('01900000-0000-7000-8000-00000000000a', 1, '田中 一郎', 'たなか いちろう', 'tanaka@example.com', '03-0000-0000',
    '見積もりをお願いします。', '/company/', '2025-07-01 10:00+09'),
  ('01900000-0000-7000-8000-00000000000b', 2, '高橋 次郎', '', 'takahashi@example.com', '',
    '貸し切りはできますか。', '/menu/', '2025-07-02 10:00+09'),
  ('01900000-0000-7000-8000-00000000000a', 3, '伊藤 三郎', '', 'ito@example.com', '',
    '採用について教えてください。', '/', '2025-07-03 10:00+09');
