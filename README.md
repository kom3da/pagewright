# Pagewright

中小企業・個人のWebサイト向けの、セルフホストでも速く、日本語に強いCMS。

- 管理画面は動的、公開サイトは静的に書き出すので、どのCDNやWebサーバーでも速く配信できる
- 日本語の全文検索、ルビ、縦書き、和暦に対応
- WordPress などからの移行機能つき

開発中です。

## 開発環境で動かす

必要なもの：[mise](https://mise.jdx.dev/) と Docker。

```sh
mise install
mise run dev   # DB と Mailpit を起動、管理画面をビルド、マイグレーション、サーバー起動
```

初めてのときは、別のターミナルでサイトと管理者を作ります。表示されたURLを開くとログインできます。

```sh
mise exec -- go run ./cmd/pagewright init --site "サイトの名前" --email you@example.com
```

| 何を | どこで |
| --- | --- |
| 管理画面 | http://localhost:8080/admin/ |
| 公開サイトのプレビュー | http://localhost:8081/ （公開や「サイトを更新」で書き出された静的ファイル。検索は http://localhost:8081/search/ ） |
| 届いたメール（ログイン用リンク） | http://localhost:8025/ （Mailpit） |

- 2回目からは、ログイン画面でメールアドレスを入れ、Mailpit に届いたリンクを開きます。`go run ./cmd/pagewright login-url --email ...` でもログイン用URLを出せます
- WordPress の書き出しファイル（WXR）の取り込み：`go run ./cmd/pagewright import-wxr --site <サイトID> --author-email you@example.com [--dry-run] [--download-media] ファイル.xml`
- テストと検査：`mise run test`、`mise run lint`。Go のテストは開発用の PostgreSQL を使うので、先に `mise run dev`（または `docker compose -f deploy/compose.dev.yaml up -d`）が必要です
- 管理画面だけを開発するとき：`web/admin/README.md`

## ライセンス

本体は AGPL-3.0、テーマとプラグインのSDKは MIT（予定）。
