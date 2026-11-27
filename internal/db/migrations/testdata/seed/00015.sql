-- The device and last use of a session.

UPDATE sessions SET device = 'Windows の Chrome', last_used_at = now() - interval '2 hours'
WHERE token_hash = sha256('alice-session');
