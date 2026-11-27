# 管理画面の API（v0）

管理画面（`web/admin/`）と Go 本体（`internal/api/`）のあいだの約束。変えるときは両方を同じコミットで直す。

## 共通

- ベース：`/api/`。リクエスト・レスポンスとも JSON（メディアのアップロードだけ multipart）
- キーは camelCase。日時は RFC 3339（UTC）。ID は数値、ユーザーとテナントの ID は UUID 文字列。数値の ID はサイトごとの連番で、別のサイトにも同じ ID がある（ID が指すのは、いま選んでいるサイトのもの）
- 認証：HTTP-only の Cookie `pw_session`（SameSite=Lax、本番は Secure）
- CSRF 対策：GET 以外は、ヘッダー `X-Pagewright: 1` を必ず付ける。付いていなければ 403
- 表示中のサイト：管理画面は、いま表示しているサイトの ID（`Me.currentSiteId`）をヘッダー `X-Pagewright-Site` に付けて送る。サイトを選んで使う API（`/api/auth/*`・`/api/me` 以外）は、セッションで選んでいるサイトと違えば 409 `site_changed` で何もしない。ID はサイトごとの連番なので、別のタブでサイトを切り替えたあとに古いタブが、同じ ID の別のサイトのものを変えてしまわないため。ヘッダーがなければ確かめない
- エラー：`{"error": {"code": "invalid_slug", "message": "ページのURL（英数字）は、英小文字・数字・ハイフンで入力してください（数字だけにはできません）。", "field": "slug"}}`。`message` は管理画面にそのまま出せる日本語。`field` は入力欄に関係するエラーのときだけ付き、リクエストの項目名（`slug`、`title`、`parentId`、`name`、`altText`、`file`、`publishAt` など）が入る
  - 401 `unauthenticated`：ログインしていない
  - 403 `no_site`：サイトを選んでいない／`forbidden`：権限がない／`csrf`
  - 404 `not_found`、409 `conflict`（ほかのものと重なる）／`site_changed`（上の「表示中のサイト」）、422 入力エラー（`code` は項目ごと）
  - 409 `conflict`：URL（記事・固定ページ・カテゴリ・タグの「ページのURL（英数字）」）の重複なら `field: "slug"` が付き、メッセージもそう伝える。それ以外の重複（管理画面からはふつう起きない）は `field` なしで、URL のこととは言わない
  - どの API でも起こりうる 422：`invalid_json`（送られた JSON を読めない、知らない項目がある、4MB を超える）。500 `internal`（サーバーの不具合。メッセージは「時間をおいてもう一度」）
  - どの API でも起こりうる 422：`invalid_reference`（存在しないものを指している。DB の外部キー違反）、`invalid_value`（値が DB の制約に合わない。DB の CHECK 違反）。どちらも `field` なし。管理画面の検査をすり抜けた入力や、ほかの人が同時に消したものを指したときに出る
- テナントは画面上「サイト」と呼ぶ

## 開発時の URL

- `mise run dev`：Go 本体が http://localhost:8080 。`/api/` と、ビルド済み管理画面 `/admin/` を出す
- 公開サイトのプレビュー：http://localhost:8081 （ビルド結果を静的配信。本番の nginx の代わり）
- 管理画面の開発：`pnpm dev`（Vite）。`/api` を 8080 にプロキシする

## ログイン

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `POST /api/auth/login-link` | `{"email": "..."}` | 204。登録の有無にかかわらず同じ応答（登録のないアドレスでも同じ処理をして取り消すので、応答時間でも見分けにくい）。メールアドレスの形式が違えば 422 `invalid_email`。登録済みならメールで `/admin/login/verify?token=...` を送る（有効 15 分、1 回限り）。同じアドレスへは 15 分に 5 回、同じ IP からは 15 分に 30 回まで。超えると 429 `too_many_requests` |
| `POST /api/auth/verify` | `{"token": "..."}` | 200 `Me`。Cookie を発行。リンクを開いただけではトークンを使わず、画面のボタンでこの API を呼ぶ。無効・期限切れは 400 `invalid_token` |
| `POST /api/auth/logout` | なし | 204 |
| `GET /api/me` | なし | 200 `Me` または 401 |
| `POST /api/me/site` | `{"siteId": "uuid"}` | 200 `Me`。所属していないサイトは 403 |
| `GET /api/me/sessions` | なし | 200 `Session[]`。自分のログイン中のセッション（端末）。いまの端末が最初、あとは最後に使った順 |
| `DELETE /api/me/sessions/{id}` | | 204。その端末をログアウトさせる。ほかの人のセッション、いまの端末（`current`、ログアウトは `POST /api/auth/logout`）、ないものは 404 |
| `POST /api/me/sessions/logout-others` | なし | 200 `{"revoked": number}`。「ほかの端末からログアウト」。いまの端末のほかの、自分のセッションをすべて終わらせ、その数を返す |

```ts
type Session = {
  id: string;         // 取り消し用の名前（64 文字の16進数）。Cookie のトークンでも、サーバーが持つハッシュでもない
  device: string;     // 「Windows の Chrome」のような端末の名前。分からなければ ""
  createdAt: string;  // ログインした日時
  lastUsedAt: string; // 最後に使った日時（1 時間ごとにしか記録しないので、最大 1 時間古い）
  expiresAt: string;  // ログインの期限（ログインから 30 日。使っても延びない）
  current: boolean;   // いまの端末
};
```

セッションには、端末の名前のほか User-Agent も IP アドレスも保存しない（個人情報で、一覧には要らないため）。端末の名前は、ログインした直後と、その後 1 時間に 1 回まで、使った端末の User-Agent から作り直す。

```ts
type Me = {
  user: { id: string; email: string; displayName: string };
  sites: { id: string; name: string; role: "admin" | "editor" }[];
  currentSiteId: string | null; // 所属が 1 つなら自動で選ばれる
};
```

## コンテンツ（記事・固定ページ）

ここから下は、サイトを選んでいないと 403 `no_site`。

```ts
type ContentStatus = "draft" | "scheduled" | "published";

type ContentSummary = {
  id: number;
  number: number;            // URL 用の番号（/posts/123/）
  type: "post" | "page";
  title: string;             // 最新の版のタイトル
  slug: string | null;       // 「ページのURL（英数字）」
  status: ContentStatus;
  hasUnpublishedChanges: boolean; // 公開中の版より新しい版か、公開を待つ設定がある（一度も公開していなければ false）
  url: string | null;        // 公開中の URL（routes の canonical）
  publishedAt: string | null;
  scheduledAt: string | null; // 予約公開の日時。公開中の記事の更新を予約しているときも入る
  scheduledRevisionId: number | null; // 予約している版
  latestRevisionId: number;   // 最新の版。scheduledRevisionId と違えば、予約のあとに保存した変更がある
  parentId: number | null;    // 固定ページの親
  updatedAt: string;
};

type Revision = {
  id: number;
  title: string;
  body: object;              // schema/body-v1.md
  bodySchemaVersion: number;
  fields: Record<string, unknown>;
  createdAt: string;
};

type ContentDetail = ContentSummary & {
  categoryIds: number[];
  tagIds: number[];
  revision: Revision;        // 最新の版（編集中の内容）
  publishedRevisionId: number | null;
  pendingSettings: boolean;  // 公開を待つ設定がある
};
```

公開中のコンテンツの設定（`slug`・`parentId`・`categoryIds`・`tagIds`・`publishedAt`）は、タイトルや本文と同じく変更を公開するまで公開サイトに出ない。`ContentDetail` はこれらに保存した値（公開を待つ値を含む）を返し、`pendingSettings` が true になる。`url` は公開中の URL のまま。一覧（`ContentSummary[]`）は公開中の値を返す。

```ts
// 種類ごとの件数。draft・scheduled・published は ContentSummary.status と同じ分け方（3 つの合計が全件）
type ContentCounts = Record<"post" | "page", {
  draft: number;
  scheduled: number;           // 一度も公開していないものの公開予約（公開中の記事の更新の予約は published に入る）
  published: number;
  changed: number;             // published のうち hasUnpublishedChanges が true のもの
  nextScheduledAt: string | null; // scheduled のうち、いちばん早い scheduledAt
}>;
```

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `GET /api/contents?type=post&q=...&status=...&before=...&limit=...` | `type`：`post` / `page`（それ以外は 422 `invalid_type`）。`q`（省略可）：タイトルか「ページのURL」の部分一致、100文字まで。`status`（省略可）：`draft` / `scheduled` / `published`、または `changed`（公開中で、まだ公開していない変更があるもの＝`hasUnpublishedChanges`）。`limit`（省略可）：1〜1000、省略すると 1000。`before`（省略可）：この番号より古いものから | 200 `ContentSummary[]`（番号の降順、`limit` 件まで）。続きは、受け取った最後の `number` を `before` に入れて取る。`q` が 100 文字を超えるか制御文字を含むと 422 `invalid_q`、`status` がそれ以外なら 422 `invalid_status`、`limit` が範囲外なら 422 `invalid_limit`、`before` が正の整数でなければ 422 `invalid_before`。`sort=updated`（省略可）を付けると、番号ではなく最後に更新した順（`updatedAt` の降順）に `limit` 件まで返す（ホームの「最近編集した記事・ページ」用）。`sort=updated` は `q`・`status`・`before` と一緒には使えず、`sort` がそれ以外の値か組み合わせが違えば 422 `invalid_sort` |
| `GET /api/contents/counts` | | 200 `ContentCounts`。ホームのタイルの件数。一覧（1000 件まで）と違い、記事・固定ページが何件あっても正しく数える |
| `POST /api/contents` | `{"type": "post"}` | 201 `ContentDetail`（空の版を 1 つ作る）。`type` が `post`・`page` でなければ 422 `invalid_type` |
| `GET /api/contents/{id}` | | 200 `ContentDetail` |
| `PUT /api/contents/{id}/draft` | `{"title": "...", "body": {...}, "fields": {...}, "baseRevisionId": 12}`（`fields`・`baseRevisionId` は省略可） | 200 `ContentDetail`。保存のたびに新しい版を作る。タイトルが 200 文字を超えると 422 `invalid_title`、本文がスキーマに合わなければ 422 `invalid_body`、`fields` がオブジェクトでなければ 422 `invalid_fields`。`baseRevisionId`（編集を始めたときの `revision.id`）が最新の版と違えば、ほかの人が先に保存したので 409 `edit_conflict` |
| `PATCH /api/contents/{id}` | `{"slug"?: string \| null, "parentId"?: number \| null, "categoryIds"?: number[], "tagIds"?: number[], "publishedAt"?: string}` | 200 `ContentDetail`。公開中でなければそのまま保存する。公開中なら同じように検証したうえで、公開中の値と違う項目だけを公開待ちとして残し（前に残したものに重ねる。すべて公開中の値に戻れば公開待ちはなくなる）、公開サイトは変えず再ビルドもしない。URL の重なりは保存の時点でも確かめる（409 `conflict`）。`publishedAt` はサイトに出る公開日（今より前の日時。未公開ならその日付で公開される）。先の日時は 422 `invalid_published_at`。ないカテゴリ・タグは 422 `invalid_reference`。親ページの指定が正しくない（記事に親を付けた、ない・固定ページでない親、自分やその下のページを親にした）と 422 `invalid_parent` |
| `POST /api/contents/{id}/publish` | なし、または `{"baseRevisionId": 12}` | 200 `ContentDetail`。最新の版を公開し、公開を待つ設定も反映して（公開中に URL が変われば古い URL はリダイレクトとして残す）、サイトの再ビルドを予約する。そのあいだに消えたカテゴリ・タグは飛ばす。URL がほかで使われていれば 409 `conflict`（`field: "slug"`）で、何も変えない。公開の予約があれば取り消す。`baseRevisionId`（画面で見ていた版）が最新の版と違えば、ほかの人が先に保存したので 409 `edit_conflict` |
| `POST /api/contents/{id}/unpublish` | なし | 200 `ContentDetail`。公開を待つ設定は反映して下書きの設定にする（URL が反映できなければ URL の変更だけは捨てる。取り消しは止めない）。公開の予約があれば取り消す。再ビルドを予約する |
| `POST /api/contents/{id}/schedule` | `{"publishAt": "RFC 3339", "baseRevisionId"?: 12}`（今から 1 分以上あと） | 200 `ContentDetail`。最新の版をその日時に公開する。そのときに公開を待っている設定も反映する（URL が反映できなければ、URL の変更だけ公開待ちに残して公開は進める）。もう一度呼ぶと予約を置き換える。過去の日時は 422 `invalid_publish_at`。`baseRevisionId` は公開と同じ |
| `DELETE /api/contents/{id}/schedule` | | 200 `ContentDetail`。予約を取り消す |
| `DELETE /api/contents/{id}` | | 204。再ビルドを予約する |
| `POST /api/contents/{id}/preview-links` | `{"days": 1〜30}`（本体は必須。`days` のキーを省くと 3） | 201 `PreviewLink`（`url` 付き）。ログインなしで見られるリンク。開くたびにその時点の最新の版を描く（作った時点の内容に固定しない）。`url` はこのときしか返らない（保存しているのはハッシュだけ）。範囲外は 422 `invalid_days`（`field: "days"`） |
| `GET /api/contents/{id}/preview-links` | | 200 `PreviewLink[]`（有効なものだけ。`url` なし） |
| `DELETE /api/preview-links/{id}` | | 204。リンクを取り消す |
| `GET /api/contents/{id}/preview/` | | 200 HTML。管理画面の「プレビューを見る」が新しいタブで開くページ。最新の版を、ログインして選んでいるサイトのメンバーだけに見せる（リンクは作らない）。画像とスタイルシートは同じ場所の `media/`・`_assets/` から出す。JSON ではなくページで答え、ログインしていなければ 401、別のサイトや存在しない内容は 404。`Content-Security-Policy: sandbox allow-same-origin; script-src 'none'`（スクリプトもフォームの送信も動かない） |

```ts
type PreviewLink = { id: number; url?: string; expiresAt: string; createdAt: string };
```

プレビューのページ（`/p/{token}/`）は本体が描画する。帯で「プレビュー」と示し、検索エンジンには載せない。ページ内のリンクは公開サイトを指す。

`slug` の検査：英小文字・数字・ハイフンのみ、数字だけは不可、1〜100 文字。日本語が入っていたら 422 `invalid_slug`（「ページのURL（英数字）は、英小文字・数字・ハイフンで入力してください（数字だけにはできません）。」。カテゴリとタグでは「カテゴリのURL（英数字）は…」「タグのURL（英数字）は…」）。
いちばん上の固定ページの URL を `posts`・`media`・`search`・`pagefind`・`_assets` にする（サイトの仕組みが使うパスと重なる）と 422 `reserved_slug`（`field: "slug"`）。下に固定ページがある固定ページを `DELETE` すると 422 `has_children`（先に下のページを移すか削除する）。

URL の決まり方（MVP）：記事は `/posts/{slug または番号}/`、固定ページは `/{親の slug}/{slug または番号}/`。

## カテゴリとタグ

```ts
type Category = { id: number; name: string; slug: string; parentId: number | null; position: number };
type Tag = { id: number; name: string; slug: string };
```

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `GET /api/categories` | | 200 `Category[]` |
| `POST /api/categories` | `{"name", "slug", "parentId"?}` | 201 `Category` |
| `PATCH /api/categories/{id}` | `{"name"?, "slug"?, "parentId"?, "position"?}` | 200 `Category` |
| `DELETE /api/categories/{id}` | | 204。子カテゴリは一つ上の階層に移る |
| `GET /api/tags` | | 200 `Tag[]` |
| `POST /api/tags` | `{"name", "slug"}` | 201 `Tag` |
| `PATCH /api/tags/{id}` | `{"name"?, "slug"?}` | 200 `Tag` |
| `DELETE /api/tags/{id}` | | 204 |

カテゴリとタグの slug も同じ検査。名前が空か 100 文字を超えると 422 `invalid_name`。親カテゴリの指定が正しくない（ない、自分やその下のカテゴリ）と 422 `invalid_parent`。slug の重複は 409 `conflict`（`field: "slug"`。ほかのカテゴリ・タグと重なったときは、そう伝えるメッセージ）。slug を変えると、古い URL はリダイレクトとして残る。

## メディア（画像）

```ts
type Media = {
  id: number;
  url: string;        // 公開サイトでのパス（/media/...）。本文の image.src に入れる
  previewUrl: string; // 管理画面で表示するための URL（/api/media/{id}/file、要ログイン）
  filename: string;
  contentType: string;
  sizeBytes: number;
  width: number | null;
  height: number | null;
  altText: string;
  createdAt: string;
};
```

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `POST /api/media` | multipart：`file`、`altText` | 201 `Media`。画像（JPEG・PNG・GIF・WebP）だけ、10MB まで。`altText` が空か 500 文字を超えると 422 `alt_required`、ファイルがなければ 422 `file_required`、画像でないか読めなければ 422 `unsupported_type`、大きすぎれば 413 `too_large`（どれも `field` 付き）。サイトの画像の合計が上限（`PAGEWRIGHT_MEDIA_QUOTA_MB`。0 なら上限なし）を超えるなら 422 `quota_exceeded`（`field` 付き。`message` に上限と今の使用量、古い画像を消すか管理者に相談するよう案内が入る） |
| `GET /api/media` | | 200 `Media[]`（新しい順） |
| `GET /api/media/{id}/file` | | 画像そのもの |
| `PATCH /api/media/{id}` | `{"altText"}` | 200 `Media`。空か 500 文字を超えると 422 `alt_required` |
| `DELETE /api/media/{id}` | | 204。最新・公開中・予約中のどれかの本文で使われていれば 409 `media_in_use` |

本文に画像を入れるときは `image` ノードに `src = media.url`、`mediaId = media.id`、`alt` を入れる。管理画面のエディタでの表示には `mediaId` から `/api/media/{id}/file` を使う。

## サイト

```ts
type MenuItem = { label: string | null; contentId: number | null; url: string | null }; // contentId か url のどちらか一方
type SiteSettings = {
  name: string;
  logoMediaId: number | null;
  logo: Media | null;          // 出力だけ
  primaryColor?: string;       // "#RRGGBB"。白とのコントラスト比 4.5:1 以上。標準 #0a4f8f
  footerColor?: string;        // "#RRGGBB"。同上（白い文字が乗る）。標準 #1d2733
  font?: "gothic" | "mincho";  // 標準 gothic
  timeZone?: string;           // IANA 名。標準 "Asia/Tokyo"
  contactEmail?: string;       // お問い合わせの送り先。空ならサイトの管理者全員
  frontPageId?: number;        // トップに載せる固定ページ（どう見せるかはテーマ次第。標準テーマでは大見出しになる）
  privacyPageId?: number;      // 個人情報の取り扱いのページ。お問い合わせフォームの同意欄からリンクされる
  theme?: string;              // テーマの名前（省略時は標準の "corporate"）。入っていないテーマは 422
  themeSettings?: Record<string, Record<string, string | number>>; // テーマ名 → そのテーマが宣言した設定の値（docs/themes/settings.md）。page 型は固定ページの id
  themes: ThemeInfo[];         // 出力だけ。入っているテーマと、それぞれの設定項目の宣言
  publicBaseUrl: string;       // 出力だけ。公開サイトの URL（末尾の / なし）。サイトに設定されたもの（pagewright site set-url）、なければサーバーの PAGEWRIGHT_PUBLIC_BASE_URL。管理画面からのリンク用。PUT では送らない
  menu: MenuItem[];            // 空なら、親のない固定ページとお知らせを自動で並べる
};
type ThemeInfo = {
  name: string; title: string; description: string;
  settingsTitle: string; settingsDescription: string; // 設定項目のまとまりの見出しと説明（空なら管理画面の既定）
  settings: { key: string; label: string; description: string; type: "text" | "tel" | "url" | "page"; maxLength: number }[];
};
```

- `primaryColor` から `contactEmail` までは、省略・空文字・未設定のどれも「標準のまま」。GET では標準のときは項目が出ない
- メニュー：20 件まで。`label` は 50 文字まで（`contentId` のときは省略でき、そのページのタイトルになる）。`url` は `/` か `http://`・`https://` で始め、`label` が必須。違反と、存在しないページの指定は 422 `invalid_menu`（`field: "menu"`）
- 色・書体・タイムゾーン・送り先・テーマの違反は 422 `invalid_setting`、`field` はその項目名。テーマの設定の違反も 422 `invalid_setting` で、`field` は `themeSettings.<key>`（宣言にない項目、型に合わない値、このサイトにない固定ページ）。トップのページが固定ページでなければ 422 `invalid_front_page`、個人情報の取り扱いのページが固定ページでなければ 422 `invalid_privacy_page`。サイト名は `invalid_name`、ロゴの画像が見つからなければ `invalid_logo`
- ロゴに使っている画像は `DELETE /api/media/{id}` で 409 `media_in_use`

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `GET /api/site` | | 200 `SiteSettings` |
| `PUT /api/site` | `SiteSettings`（`logo` を除く。全体を置き換える） | 200 `SiteSettings`。管理者だけ（編集者は 403）。不正な値は 422、`field` に項目名（`primaryColor` など、メニューは `menu`）。再ビルドを予約する |
| `POST /api/site/build` | なし | 202。全体ビルドを予約する（手動で再ビルドしたいとき） |
| `GET /api/site/builds?limit=5` | `limit`：1〜50（省略時 5） | 200 `Build[]`（新しい順）。公開サイトの書き出しの記録。`limit` が整数でないか範囲外なら 422 `invalid_limit` |

```ts
type Build = {
  id: number; startedAt: string; finishedAt: string;
  files: number; changed: number; removed: number;  // 全ファイル数、変わった数、消えた数
  warnings: string[];   // 書き出せなかった記事など、利用者に見せる注意
  error: string | null; // 失敗したときだけ
};
```

## お問い合わせ

本文に `contactForm` ブロック（schema/body-v1.md）を置くと、公開サイトにお問い合わせフォームが出る。送信は公開サイトから Go 本体の `POST /f/{サイトID}/contact`（JavaScript なしの HTML フォーム。結果の画面も本体が返す）。送り先はサイト設定の `contactEmail`、なければサイトの管理者全員。送った人には自動返信メールが届く。

```ts
type Submission = {
  id: number; name: string; kana: string; email: string; phone: string;
  message: string; pagePath: string; createdAt: string;
  handledAt: string | null;  // 対応済みにした日時。未対応なら null
  handledBy: string | null;  // 対応済みにした人の名前（表示名、なければメールアドレス）。未対応か、その人がサイトのメンバーでなくなったら null
};
```

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `GET /api/forms/submissions?handled=...&before={id}` | `handled`（省略可）：`false`（未対応だけ）／`true`（対応済みだけ）。`before` は省略可 | 200 `Submission[]`（`id` の降順＝受け付けた順の新しい順に 100 件まで）。続きは、受け取った最後の `id` を `before` に入れて取る（`handled` はそのまま付ける）。`before` が正の整数でなければ 422 `invalid_before`、`handled` がそれ以外なら 422 `invalid_handled`。管理者だけ（編集者は 403） |
| `PATCH /api/forms/submissions/{id}` | `{"handled": true \| false}` | 200 `Submission`。`true` で対応済み（すでに対応済みなら、最初に対応済みにした日時と人のまま）、`false` で未対応に戻す。お問い合わせの内容は変えられない（ほかの項目を送ると 422 `invalid_json`）。`handled` がなければ 422 `invalid_handled`。管理者だけ |
| `DELETE /api/forms/submissions/{id}` | | 204。管理者だけ |
