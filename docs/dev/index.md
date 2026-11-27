# 開発環境で動かす

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
- コーポレートテーマのサンプル（架空の会社のページ9件・お知らせ12件・画像）を新しいサイトに入れる：`mise run sample:load`（中身は `themes/corporate/sample/`、管理APIだけを使って入れる）
- WordPress の書き出しファイル（WXR）の取り込み：`go run ./cmd/pagewright import-wxr --site <サイトID> --author-email you@example.com [--dry-run] [--download-media] ファイル.xml`
- テストと検査：`mise run test`、`mise run lint`。Go のテストは開発用の PostgreSQL を使うので、先に `mise run dev`（または `docker compose -f deploy/compose.dev.yaml up -d`）が必要です
- 管理画面だけを開発するとき：`web/admin/README.md`

## ドキュメントのサイト

`docs/` の Markdown は VitePress のサイトにもなります（検索は Pagefind）。

```sh
mise run docs:dev     # http://localhost:5173/ 。書き換えるとすぐ反映される（検索は使えない）
mise run docs:build   # docs/.vitepress/dist に書き出し、検索の索引を作る
mise run docs:preview # 書き出したものを http://localhost:4173/ で見る（検索も使える。書き出し直したら起動し直す）
```

- `schema/body-v1.md` と `web/admin/DESIGN.md` はコードの隣に置いたまま、`docs/dev/` のページが読み込んで表示します。直すときは元のファイルを直します
- リンク切れがあると `docs:build` は失敗します
