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

- `settings`（任意）は、このテーマが管理画面に出してほしい設定項目です（下の「テーマの設定項目」）
- `labels` は、本体がページに入れる言葉です。パンくず、一覧の見出し、`<title>`、RSS の説明、表を囲む領域の名前（`table`。スクリーンリーダーが読み上げます）に使います。すべて必須です。`%s` と `%d` は Go の fmt の書式です
- `dateFormat` は `date` 関数の書式です（Go の時刻レイアウト）
- 知らないキーはエラーになります（書き間違いに気づけるように）

## テンプレートに渡るデータ

すべてのページテンプレートは `*Page` を受け取ります。型の定義は `internal/build/theme.go` にあります。

### `.Site`（全ページ共通）

| 項目 | 内容 |
| --- | --- |
| `Name` | サイト名 |
| `LogoURL` | ロゴ画像（なければ空） |
| `Menu` | メニュー（`[]Link`）。管理画面で設定したもの。未設定なら、親のない固定ページ → お知らせ一覧 → お問い合わせのページ |
| `Pages` | 親のない公開中の固定ページ（`[]Summary`、番号順、トップに載せるページを除く）。カードなどに |
| `ContactURL` | お問い合わせフォームを置いたページの URL（なければ空） |
| `PrivacyURL` | サイト設定で選んだ「個人情報の取り扱い」のページの URL（なければ空） |
| `PostsURL`・`FeedURL`・`SearchURL` | お知らせ一覧、RSS、検索ページ（検索が無効なら空） |
| `Theme` | このテーマが `theme.json` で宣言した設定の値（`.Site.Theme.phone` のように使う。未入力の項目はない） |
| `StylesheetURL` | `assets/style.css` の URL |
| `StyleVars` | サイト設定の色と書体（`<style>:root { {{.}} }</style>` で出す。後述） |
| `BaseURL` | 公開サイトの URL（末尾の / なし） |

### ページごと

| 項目 | 内容 |
| --- | --- |
| `Kind` | `home` `post` `page` `posts` `category` `tag` `404` `search` |
| `Path`・`Canonical` | このページのパス、絶対 URL |
| `Title`・`Description` | `<title>` と説明。トップでは `Title` は空 |
| `NoIndex` | 検索エンジンに載せないページ（`<meta name="robots" content="noindex">` を出す） |
| `Breadcrumbs` | パンくず（`[]Link`。最後は現在のページで URL なし） |
| `Content` | 記事・固定ページ（`Title`、`Body`、`Type`、`PublishedAt`、`Categories`、`Tags`、下の親子と前後）。トップでは、トップに載せるページの本文 |
| `Front` | トップに載せるページの要約（`Summary`。なければ nil）。トップだけ |
| `Posts` | お知らせの一覧（`[]PostItem`）。トップ（最新数件）と一覧ページ |
| `Heading`・`Pagination` | 一覧ページの見出しとページ送り |
| `SelfURL` | プレビューのときだけ、このページ自身の URL（下の「プレビュー」） |

`Content` には、ほかのページへの案内に使える項目もあります。どれも公開中のものだけで、なければ nil です。

| 項目 | 内容 |
| --- | --- |
| `Parent` | 固定ページの親ページ（`*Link`） |
| `Children` | 固定ページの子ページ（`[]Link`、作った順） |
| `Siblings` | 同じ親を持つ固定ページ（`[]Link`、作った順）。このページ自身も入り、`Current` が true。親のないページでは nil |
| `Newer`・`Older` | 記事の、ひとつ新しい記事とひとつ古い記事（`*Link`） |

`Summary` は `Title`、`URL`、`Lead`（最初の段落）、`Excerpt`（短い抜粋）、`Image`（本文の最初の画像。`Src` と `Alt`）、`Rest`（最初の段落と最初の画像を除いた本文の HTML。固定ページのみ）、`More`（`Rest` に文章があるか）です。ヒーローにするか、カードにするか、本文を全部出すかはテーマが決めます。

### 関数

| 関数 | 内容 |
| --- | --- |
| `date` | `theme.json` の `dateFormat` で日付を書く |
| `isoDate` | `2006-01-02`（`<time datetime>` 用） |
| `current` | メニューのリンクに付ける `aria-current` の値（`page` / `true` / 空） |
| `initial` | 文字列の最初の 1 文字（ロゴがないときの印などに） |

## テーマの設定項目

住所や電話番号のように、テーマが見せたいだけの情報は、本体の設定には入れません。テーマが `theme.json` の `settings` で宣言すると、管理画面のサイト設定に入力欄が出て、値が `.Site.Theme.<key>` で届きます。

```json
"settings": [
  {"key": "phone", "label": "電話番号", "description": "フッターに表示します。", "type": "tel"},
  {"key": "address", "label": "住所", "type": "text", "maxLength": 200},
  {"key": "recruitPage", "label": "採用ページ", "type": "page"}
]
```

| 型 | 入力 | テンプレートでの値 |
| --- | --- | --- |
| `text` | 1行の文字（`maxLength` まで。既定 100） | 文字列 |
| `tel` | 数字・ハイフン・括弧・+ | 文字列 |
| `url` | `https://`・`http://`・`/` で始まる URL | 文字列 |
| `page` | このサイトの固定ページを選ぶ | `Link`（`Title`・`URL`）。公開中のときだけ |

- `settingsTitle` と `settingsDescription`（任意）は、管理画面でこれらの項目をまとめる見出しと説明です。利用者が探す名前にします（例：「会社の連絡先」）。省くと管理画面の既定の見出しになります
- `key` は英字で始まる英数字です。値はテーマごとに保存されるので、テーマを切り替えても消えません
- 宣言の誤り（型の間違い、キーの重複など）は、読み込むときにエラーで知らせます

## 本文の HTML

`Content.Body` は、本文の JSON（schema/body-v1.md）から本体が組み立てた安全な HTML です。テーマは CSS で見た目を付けます。付くクラスは次だけです。

| 要素 | クラス |
| --- | --- |
| 画像 | `<figure class="image"><img …></figure>`（`width`・`height` は分かるときだけ） |
| 表 | `<div class="table-wrap" role="region" tabindex="0" aria-label="（labels.table）"><table>…`。幅の広い表をキーボードでも横にスクロールできるように、囲みにフォーカスが当たります。`:focus-visible` の見た目を付けてください |
| YouTube・Google マップ | `<div class="embed embed-youtube">` / `embed-map` |
| 移行した HTML | `<div class="raw-html">` |
| ルビ | `<ruby>…<rp>(</rp><rt>…</rt><rp>)</rp></ruby>` |

見出しは `h2`〜`h4`、ほかは素の `p`、`ul`、`ol`、`blockquote`、`hr`、`code`、`a` です。

### 部品（`templates/parts/`）

本文の中のブロックのうち、見た目をテーマに任せるものは部品として呼びます。

- **`contact-form`**：お問い合わせフォーム。受け取るデータは `.Action`（送信先）、`.Page`（このページのパス）、`.PrivacyURL`（個人情報の取り扱いのページ。なければ空）です
  - 送る項目名は固定です：`name`（必須）、`kana`、`email`（必須）、`phone`、`message`（必須）、`consent=yes`（必須）、`page`
  - `website` はスパム対策の隠し欄です。人には見せず（`aria-hidden="true"`、`tabindex="-1"`）、空のまま送られるようにしてください
  - 部品がなければ、フォームのブロックは出力されません

## サイト設定の色と書体

管理画面のサイト設定で選んだ色と書体は、`Site.StyleVars` として次の CSS 変数で渡ります。値は本体で検査済みです。テーマはこれらの変数を使って色と書体を決めると、利用者の設定が反映されます。

| 変数 | 内容 |
| --- | --- |
| `--color-accent` | テーマカラー（白とのコントラスト比 4.5:1 以上が保証される） |
| `--color-accent-dark` | テーマカラーを暗くした色（ホバーなど） |
| `--color-focus` | フォーカスの枠の色（テーマカラーと同じ） |
| `--color-footer-bg` | フッターの背景（白い文字との差 4.5:1 以上） |
| `--font-sans` | 明朝体を選んだとき、明朝体の並び |

## 本体が自動で付けるもの

次のものは、テーマが何もしなくても本体が付けます。テーマ側で書かないでください。

- **WordPress の旧 URL（`/?p=123`）の転送**：移行したサイトのトップページに、転送のスクリプトを差し込む
- **プレビュー**（ログイン不要のプレビュー用リンク）
  - `<head>` の直後に、公開サイトを指す `<base href>` を入れ、`<body>` の直後に「プレビューです」の帯を入れる
  - `<base>` があるので、ページ内リンク（`#main` など）は `{{.SelfURL}}#main` と書く。プレビュー以外では `SelfURL` は空なので、そのまま `#main` になる

## アクセシビリティ

テーマは JIS X 8341-3（WCAG 2.1 AA）を満たしてください。最低限、次のとおりです。

- `lang="ja"`、ランドマーク（`header`・`nav`・`main`・`footer`）、本文へのスキップリンク
- 見出しの順序、`aria-current`
- `:focus-visible` で見えるフォーカス
- 文字のコントラスト 4.5:1

## まだないもの（予定）

- `pagewright theme check`：テーマの検査コマンド
- テーマを書き換えると、すぐ反映されるプレビュー
- ひな形の生成
- 本体に組み込まれていないテーマを読み込む仕組み

いまはテーマを `themes/` に置き、`themes/themes.go` に登録します。
