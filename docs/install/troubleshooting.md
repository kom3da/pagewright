# 困ったとき

## ログイン用のメールが届かない

次のコマンドでログイン用URLを出せます（1時間有効、1回限り）。

```sh
docker compose run --rm cli login-url --email you@example.co.jp
```

送信の失敗はログで確認できます：`docker compose logs app | grep -i mail`

## 状態とログを見る

```sh
docker compose ps -a
docker compose logs -f app      # 本体
docker compose logs nginx caddy # 配信と HTTPS
curl -s https://<管理画面のドメイン>/healthz   # ok と出れば本体とDBは動いている
```

## 公開サイトが 404 になる

- `.env` の `PAGEWRIGHT_SITE_ID` に `init` で表示されたIDを書き、`docker compose up -d` を実行したか
- 管理画面で記事を公開したか、「サイトを更新」を押したか（`docker compose logs app | grep "site built"` で書き出しの記録を確認できる）

## 公開しても公開サイトが更新されない（ログに pagefind のエラー）

サイト内検索の索引を作るプログラム（Pagefind）が動かないと、書き出し全体が失敗します。
`docker compose logs app | grep pagefind` に `Illegal instruction` などが出ている場合は、
サーバーのCPUに対応していない可能性があります。`.env` に `PAGEWRIGHT_PAGEFIND=off` を書いて
`docker compose up -d` を実行すると、検索なしで書き出せます。

## HTTPS でつながらない

- DNS がサーバーのIPアドレスを指しているか（`dig +short <ドメイン>`）
- ポート 80 と 443 がファイアウォールで開いているか
- `docker compose logs caddy` に証明書のエラーが出ていないか
