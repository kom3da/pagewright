# Pagewright に協力する

> **Summary (English):** Bug reports, ideas and small pull requests are welcome.
> Please read the rules below (tenant isolation, migrations, no passwords, etc.)
> before opening a PR. Discussion is mostly in Japanese, but English is fine too.

Pagewright に興味を持ってくれてありがとうございます。不具合の報告、提案、プルリクエスト（PR）を歓迎します。メンテナーは1人なので、返事に時間がかかることがあります。

- 不具合・提案：[Issue](https://github.com/kom3da/pagewright/issues/new/choose) のテンプレートから
- 脆弱性：Issue には書かず、[SECURITY.md](SECURITY.md) の方法で非公開で
- 行動規範：[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

大きな変更（新しい機能、設計に関わる変更）は、PR を作る前に Issue で相談してください。設計の決定事項と理由は `docs/design.md` にまとまっています。**設計と食い違う実装をしたくなったら、先に設計ドキュメントの変更を提案してください。**

## 開発環境

準備と起動の手順は [README.md](README.md) の「開発環境で動かす」に、管理画面だけを開発するときは [web/admin/README.md](web/admin/README.md) にあります。必要なのは [mise](https://mise.jdx.dev/) と Docker です。

よく使うコマンド：

```sh
mise run dev    # 開発用DB（PostgreSQL）と Mailpit を起動し、本体を起動
mise run test   # Go と管理画面のテスト（Go のテストは開発用DBを使う）
mise run lint   # golangci-lint、Biome、Squawk
mise run gen    # sqlc のコード生成（SQL を変えたら）
```

E2E（Playwright＋axe）は `pnpm --dir web/admin e2e` で動きます。

## 守ってほしいこと

レビューでは特に次の点を見ます。

### データとセキュリティ

- **テナント分離が最優先です。** すべてのテーブルに `tenant_id` を持たせ、RLS を有効にします。リクエストごとにDBセッションへテナントIDをセットします
- **新しいAPIの入口には、「別テナントのデータが見えない」テストを必ず書いてください**
- **マイグレーションは goose で書き、Squawk で危険な変更がないか確かめます**（`mise run lint` に含まれています）
- **RLS とマイグレーションの変更は、メンテナーのレビューなしにはマージしません。** こうした PR は時間をかけて見るので、ほかの変更と分けてください
- **DBアクセスは pgx＋sqlc です。** ORM は使いません。SQL は必ずパラメータで値を渡し、文字列の連結で組み立てないでください
- **パスワード認証は実装しません。** ログインはメールのログインリンクとパスキーだけです
- 秘密の値（APIキー、トークン、パスワード）をコミットしないでください

### 本文と公開サイト

- **本文JSONのスキーマを変えるときは、バージョンを上げて変換処理を書きます**（`schema/`）
- **公開サイトは静的です。** 公開ページに動的な処理を足したくなったら、小さなAPIに切り出します

### 管理画面

- **管理画面を使うのは非エンジニアです。** 用語は日本語でわかりやすく書きます（「スラッグ」ではなく「ページのURL（英数字）」など）
- **アクセシビリティ（JIS X 8341-3）を守ります。** キーボードだけで操作できること、ラベルや読み上げ、色のコントラストに気をつけてください。E2E では axe で確かめます
- lint とフォーマットは Biome、テストは Vitest、E2E は Playwright＋axe です
- 見た目の決まりは `web/admin/DESIGN.md` にあります

### Go

- ログは `log/slog` を使います
- テストは標準の `testing` と go-cmp を使います
- 既存のコードの書き方に合わせてください

## プルリクエスト

- main に小さな PR を積み重ねる進め方（トランクベース）です。**1つの PR で1つのことだけ**にしてください。関係のないリファクタリングや整形を混ぜないでください
- 出す前に `mise run test` と `mise run lint` が通ることを確かめてください
- 振る舞いを変えたら、テストを足すか直してください。テストのカバー範囲を減らさないでください
- 使い方や導入手順が変わるときは、ドキュメントも一緒に直してください
- 管理画面の見た目を変えたら、変更前と変更後のスクリーンショットを PR に貼ってください
- PR のテンプレートのチェックリストを埋めてください

### コミットメッセージ

英語で、命令形で書きます（"Added feature" ではなく "Add feature"）。

```
Give each site its own public URL, set with pagewright site

tenants.public_base_url holds a site's public origin; builds and
previews use it and fall back to PAGEWRIGHT_PUBLIC_BASE_URL.
```

- 1行目は72文字以内で、何をしたかを短く
- 必要なら1行空けて、なぜそうしたかを本文に書く
- コードのコメントも英語で書きます

## ライセンス

Pagewright の本体は [AGPL-3.0](LICENSE) です（テーマとプラグインのSDKは MIT にする予定です）。

送ってくれた変更は、そのファイルが属するプロジェクトのライセンスで提供されたものとして扱います。CLA（貢献者ライセンス契約）や DCO の署名（`Signed-off-by`）は今のところ求めていません。自分で書いたもの、またはそのライセンスで出してよいものだけを送ってください。
