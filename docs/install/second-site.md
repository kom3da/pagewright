# 2つ目のサイトを公開する

1つの構成で、2つ目以降のサイトも公開できます。管理画面・データベース・本体は1つ目のサイトと共通で、
公開サイトのドメインだけを足します。1つ目のサイトはそのまま（`.env` の `PAGEWRIGHT_SITE_DOMAIN` と `PAGEWRIGHT_SITE_ID`）です。

以下、2つ目のサイトのドメインを `www.example2.co.jp` とします。

1. **サイトと管理者を作る。** 表示された **ID** を控えます

   ```sh
   docker compose run --rm cli init --site "株式会社サンプル2" --email you@example2.co.jp
   ```

2. **DNS を設定する。** `www.example2.co.jp` の A レコード（AAAA レコード）を、このサーバーに向けます

3. **公開サイトのURLを設定する。** canonical・サイトマップ・RSS・プレビュー・お問い合わせ送信後の「戻る」リンクが、このURLになります

   ```sh
   docker compose run --rm cli site set-url <2つ目のサイトのID> https://www.example2.co.jp
   ```

   設定すると、そのサイトの公開サイトが書き出し直されます。管理画面の「サイトの設定」にも表示されます（管理画面からは変えられません）。
   設定を外すと、`PAGEWRIGHT_PUBLIC_BASE_URL`（標準の構成では `https://<PAGEWRIGHT_SITE_DOMAIN>`）に戻ります。

   ```sh
   docker compose run --rm cli site clear-url <2つ目のサイトのID>
   ```

4. **HTTPS の証明書を取る。** `.env` にドメインを書きます。Caddy がこのドメインの証明書も取得します

   ```sh
   PAGEWRIGHT_EXTRA_SITE_DOMAINS=www.example2.co.jp
   ```

   複数あるときは半角スペースで区切り、全体を `"` で囲みます（`,` で区切ると Caddy が起動しません）

   ```sh
   PAGEWRIGHT_EXTRA_SITE_DOMAINS="www.example2.co.jp www.example3.co.jp"
   ```

5. **nginx に配信の設定を足す。** `nginx/templates/` に、たとえば `site-example2.conf.template` という名前で次のファイルを作ります。
   `server_name` にドメインを、`/data/site/` の後ろ（2か所）にサイトのIDを書きます

   ```nginx
   server {
       listen 8080;
       server_name www.example2.co.jp;
       root /data/site/<2つ目のサイトのID>;
       index index.html;
       charset utf-8;
       server_tokens off;
       access_log /var/log/nginx/access.log pagewright_redacted;

       gzip on;
       gzip_types text/css application/javascript application/json application/xml application/rss+xml application/atom+xml image/svg+xml text/plain;

       include /data/site/<2つ目のサイトのID>/_redirects*.nginx;

       location ^~ /_redirects { return 404; }
       location ~ /\. { return 404; }

       location / {
           try_files $uri $uri/index.html =404;
       }

       location ^~ /pagefind/ { expires 1h; }
       location ^~ /media/ { expires 1d; }

       error_page 404 /404.html;
   }
   ```

   中身は `default.conf.template` の「公開サイト」の部分と同じです。アップデートのときは、リリースノートでその部分に変更がないか確認してください
   （自分で足したファイルは、`git checkout` で新しい版にしても消えません）。旧URLからのリダイレクトは、1つ目のサイトと同じく自動で読み込み直されます。

6. **起動し直す**

   ```sh
   docker compose up -d
   docker compose restart nginx
   ```

`https://www.example2.co.jp/` を開いて表示を確かめてください。まだ何も公開していなければ、2つ目のサイトの管理画面で「サイトを更新」を押します。
