# テンプレートに渡るデータ

すべてのページテンプレートは `*Page` を受け取ります。型の定義は `internal/build/theme.go` にあります。

## `.Site`（全ページ共通）

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
| `StyleVars` | サイト設定の色と書体（`<style>:root { {{.}} }</style>` で出す。「[サイト設定の色と書体](./settings.md#サイト設定の色と書体)」） |
| `BaseURL` | 公開サイトの URL（末尾の / なし） |

## ページごと

| 項目 | 内容 |
| --- | --- |
| `Kind` | `home` `post` `page` `posts` `category` `tag` `404` `search` |
| `Path`・`Canonical` | このページのパス、絶対 URL |
| `Title`・`Description` | `<title>` と説明。トップでは `Title` は空 |
| `NoIndex` | 検索エンジンに載せないページ（`<meta name="robots" content="noindex">` を出す） |
| `Breadcrumbs` | パンくず（`[]Link`。最後は現在のページで URL なし） |
| `Content` | 記事・固定ページ（`Title`、`Body`、`Type`、`PublishedAt`、`Categories`、`Tags`、下の親子と前後）。トップでは、トップに載せるページの本文 |
| `Front` | トップに載せるページの要約（`Summary`。なければ nil）。トップだけ。このページ自身のURLには `single` を書き出さず、トップへの転送にする（サイトマップにも載せない）。メニューやパンくずでこのページを指すリンクは `/` になる |
| `Posts` | お知らせの一覧（`[]PostItem`）。トップ（最新数件）と一覧ページ |
| `Heading`・`Pagination` | 一覧ページの見出しとページ送り |
| `SelfURL` | プレビューのときだけ、このページ自身の URL（下の「[本体が自動で付けるもの](#本体が自動で付けるもの)」のプレビュー） |

`Content` には、ほかのページへの案内に使える項目もあります。どれも公開中のものだけで、なければ nil です。

| 項目 | 内容 |
| --- | --- |
| `Parent` | 固定ページの親ページ（`*Link`） |
| `Children` | 固定ページの子ページ（`[]Link`、作った順） |
| `Siblings` | 同じ親を持つ固定ページ（`[]Link`、作った順）。このページ自身も入り、`Current` が true。親のないページでは nil |
| `Newer`・`Older` | 記事の、ひとつ新しい記事とひとつ古い記事（`*Link`） |

`Summary` は `Title`、`URL`、`Lead`（最初の段落）、`Excerpt`（短い抜粋）、`Image`（本文の最初の画像。`Src` と `Alt`）、`Rest`（最初の段落と最初の画像を除いた本文の HTML。固定ページのみ）、`More`（`Rest` に文章があるか）です。ヒーローにするか、カードにするか、本文を全部出すかはテーマが決めます。

## 関数

| 関数 | 内容 |
| --- | --- |
| `date` | `theme.json` の `dateFormat` で日付を書く |
| `isoDate` | `2006-01-02`（`<time datetime>` 用） |
| `current` | メニューのリンクに付ける `aria-current` の値（`page` / `true` / 空） |
| `initial` | 文字列の最初の 1 文字（ロゴがないときの印などに） |

## 本体が自動で付けるもの

次のものは、テーマが何もしなくても本体が付けます。テーマ側で書かないでください。

- **WordPress の旧 URL（`/?p=123`）の転送**：移行したサイトのトップページに、転送のスクリプトを差し込む
- **プレビュー**（ログイン不要のプレビュー用リンク）
  - `<head>` の直後に、公開サイトを指す `<base href>` を入れ、`<body>` の直後に「プレビューです」の帯を入れる
  - `<base>` があるので、ページ内リンク（`#main` など）は `{{.SelfURL}}#main` と書く。プレビュー以外では `SelfURL` は空なので、そのまま `#main` になる
