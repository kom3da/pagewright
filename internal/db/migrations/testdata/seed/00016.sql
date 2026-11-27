-- The second site has its own public URL.

UPDATE tenants SET public_base_url = 'https://komorebi.example.jp'
WHERE id = '01900000-0000-7000-8000-00000000000b';
