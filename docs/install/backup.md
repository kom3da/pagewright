# バックアップと復元

本体にバックアップ機能が入るまでは、次の2つを毎日保存してください。

- **データベース**（記事、設定、ユーザーなど）
- **画像**（`/data/media`）

公開サイトのファイル（`/data/site`）は、データベースと画像から作り直せるので保存しなくてかまいません。
記事の古い版（保存の履歴）は自動で消えます（30日より前の版は、記事ごとに新しい50個と公開中・予約中の版だけ残ります）。それより前の内容が必要になったときは、バックアップから戻します。
`.env` も別の安全な場所に控えておきます。

```sh
mkdir -p /var/backups/pagewright
cd /opt/pagewright/deploy
docker compose exec -T db pg_dump -U pagewright -Fc pagewright > /var/backups/pagewright/db-$(date +%F).dump
docker compose exec -T app tar -czf - -C /data media > /var/backups/pagewright/media-$(date +%F).tar.gz
```

毎日自動で行うには、root の crontab（`sudo crontab -e`）に次を足します（`%` は `\%` と書く）。
古いファイルの削除や、別のサーバー・ストレージへのコピーも合わせて設定してください。

```
0 3 * * * cd /opt/pagewright/deploy && docker compose exec -T db pg_dump -U pagewright -Fc pagewright > /var/backups/pagewright/db-$(date +\%F).dump && docker compose exec -T app tar -czf - -C /data media > /var/backups/pagewright/media-$(date +\%F).tar.gz
```

## 復元する

新しいサーバーに戻すときは、[導入](./setup.md)の手順1〜3（`.env` は控えておいたもの）を行ってから:

```sh
docker compose up -d db                       # DBだけ起動する（アプリ用ユーザーが作られる）
docker compose exec -T db pg_restore -U pagewright -d pagewright --clean --if-exists < db-2026-01-01.dump
docker compose run --rm --no-deps -T --entrypoint tar cli -xzf - -C /data < media-2026-01-01.tar.gz
docker compose up -d
docker compose run --rm cli login-url --email you@example.co.jp
```

ログインして「サイトを更新」を押すと、公開サイトが書き出されます。
