-- One inquiry marked handled.

UPDATE form_submissions
SET handled_at = '2025-07-02 09:00+09', handled_by = '01900000-0000-7000-8000-0000000000a1'
WHERE tenant_id = '01900000-0000-7000-8000-00000000000a' AND id = 1;
