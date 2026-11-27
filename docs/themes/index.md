# テーマの作り方（テーマと本体の約束）

Pagewright のテーマは、公開サイトの見た目を決める部品一式です。本体（Go）は「何を載せるか」を中立なデータで渡し、「どう見せるか」はすべてテーマが決めます。本体はテーマ固有の文言・クラス名・マークアップを持ちません（テストで確かめています：`internal/build/theme_test.go`）。

いちばん小さな見本は `internal/build/testdata/minimal-theme/` です。標準テーマは `themes/corporate/` です。

## ファイルの構成

```
mytheme/
  theme.json              名前・文言・日付の書き方（必須）
  templates/
    base.html             ページの枠。"base" を定義する（必須）
    home.html             トップ（必須）
    single.html           記事・固定ページ（必須）
    list.html             お知らせ一覧・カテゴリ・タグ（必須）
    404.html              見つからないページ（必須）
    search.html           サイト内検索（必須。検索が無効なら使われない）
    parts/                本文の中に差し込む部品（任意）
      contact-form.html   お問い合わせフォーム（任意。ないとフォームのブロックは出ない）
  assets/
    style.css             必須。/_assets/style.css?v=<中身のハッシュ> で配られる
    …                     ほかのファイルも /_assets/ にそのまま置かれる
```

テンプレートは Go の [html/template](https://pkg.go.dev/html/template) です。各ページのテンプレートは `"main"` を定義し、`base.html` の `"base"` がそれを `{{template "main" .}}` で読み込みます。`base.html` で `"menu"` のような共通部品を定義して、ページ側から呼ぶこともできます。

読み込むときに足りないものがあれば、どのファイルの何が足りないかをエラーで知らせます。

## theme.json

```json
{
  "name": "corporate",
  "title": "コーポレート",
  "description": "管理画面でテーマを選ぶときに出す説明",
  "labels": {
    "home": "ホーム",
    "posts": "お知らせ",
    "untitled": "（無題）",
    "categoryHeading": "カテゴリ：%s",
    "tagHeading": "タグ：%s",
    "pageHeading": "%s（%dページ目）",
    "search": "サイト内検索",
    "notFound": "ページが見つかりません",
    "feedDescription": "%sのお知らせ",
    "table": "表"
  },
  "dateFormat": "2006年1月2日"
}
```

- `settings`（任意）は、このテーマが管理画面に出してほしい設定項目です（「[テーマの設定項目](./settings.md#テーマの設定項目)」）
- `labels` は、本体がページに入れる言葉です。パンくず、一覧の見出し、`<title>`、RSS の説明、表を囲む領域の名前（`table`。スクリーンリーダーが読み上げます）に使います。すべて必須です。`%s` と `%d` は Go の fmt の書式です
- `dateFormat` は `date` 関数の書式です（Go の時刻レイアウト）
- 知らないキーはエラーになります（書き間違いに気づけるように）

## まだないもの（予定）

- `pagewright theme check`：テーマの検査コマンド
- テーマを書き換えると、すぐ反映されるプレビュー
- ひな形の生成
- 本体に組み込まれていないテーマを読み込む仕組み

いまはテーマを `themes/` に置き、`themes/themes.go` に登録します。
