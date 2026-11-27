# Pagewright

中小企業・個人のWebサイト向けの、セルフホストでも速く、日本語に強いCMS。
管理画面は動的、公開サイトは静的ファイルとして書き出す。

設計ドキュメント（決定事項と理由はすべてここ）： `docs/design.md`

設計と食い違う実装をしたくなったら、先に設計ドキュメントの変更を提案すること。

## 構成

- `cmd/pagewright/` … Go本体のエントリーポイント
- `internal/` … API、ジョブ（River）、ビルド（静的出力）、移行、プラグイン基盤（wazero）
- `web/admin/` … 管理画面（React＋Vite＋TanStack Router、Tiptap）。ビルド結果は `go:embed` でバイナリに埋め込む
- `themes/` … 標準テーマ（Goのテンプレート）
- `schema/` … 本文JSONのスキーマ定義（バージョン番号つき）
- `deploy/` … Docker Compose、導入テンプレート
- `docs/` … ドキュメントのサイト（VitePress。導入・運用、テーマの作り方、開発。`mise run docs:dev`）

## 開発の始め方

```sh
mise install        # Go、Node.js、pnpm、sqlc、goose などを入れる
mise run dev        # 開発用DB（PostgreSQL）とMailpitを起動し、本体を起動
mise run test
mise run lint
mise run gen        # sqlc のコード生成
```

Mailpit（開発用メール）は http://localhost:8025

## 守ること

- **テナント分離が最優先。** すべてのテーブルに `tenant_id` を持たせ、RLSを有効にする。リクエストごとにDBセッションへテナントIDをセットする。新しいAPIの入口には「別テナントのデータが見えない」テストを必ず書く
- **RLSとマイグレーションの変更は、人のレビューなしにマージしない。** マイグレーションは goose で書き、Squawk で危険な変更を確認する
- **DBアクセスは pgx＋sqlc。** ORMは使わない
- **パスワード認証は実装しない。** ログインはメールのログインリンクとパスキーだけ
- **本文JSONのスキーマを変えるときは、バージョンを上げて変換処理を書く**
- **公開サイトは静的。** 公開ページに動的な処理を足したくなったら、小さなAPIに切り出す
- **管理画面は非エンジニアが使う。** 用語は日本語でわかりやすく（「スラッグ」ではなく「ページのURL（英数字）」など）。アクセシビリティ（JIS X 8341-3）を守る
- ログは `log/slog`、Goのテストは標準の `testing`＋go-cmp
- 管理画面の lint とフォーマットは Biome、テストは Vitest、E2E は Playwright＋axe

## 進め方

- トランクベースで、main に小さなPRを積み重ねる
- 変更は小さく、1つのPRで1つのことだけ
