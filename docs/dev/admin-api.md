# 管理画面の API（v0）

管理画面（`web/admin/`）と Go 本体（`internal/api/`）のあいだの約束。変えるときは両方を同じコミットで直す。

## 共通

- ベース：`/api/`。リクエスト・レスポンスとも JSON（メディアのアップロードだけ multipart）
- キーは camelCase。日時は RFC 3339（UTC）。ID は数値、ユーザーとテナントの ID は UUID 文字列
- 認証：HTTP-only の Cookie `pw_session`（SameSite=Lax、本番は Secure）
- CSRF 対策：GET 以外は、ヘッダー `X-Pagewright: 1` を必ず付ける。付いていなければ 403
- エラー：`{"error": {"code": "invalid_slug", "message": "ページのURLは英小文字・数字・ハイフンで入力してください", "field": "slug"}}`。`message` は管理画面にそのまま出せる日本語。`field` は入力欄に関係するエラーのときだけ付き、リクエストの項目名（`slug`、`title`、`parentId`、`name`、`altText`、`file`、`publishAt` など）が入る
  - 401 `unauthenticated`：ログインしていない
  - 403 `no_site`：サイトを選んでいない／`forbidden`：権限がない／`csrf`
  - 404 `not_found`、409 `conflict`（URL の重複など）、422 入力エラー（`code` は項目ごと）
- テナントは画面上「サイト」と呼ぶ

## 開発時の URL

- `mise run dev`：Go 本体が http://localhost:8080 。`/api/` と、ビルド済み管理画面 `/admin/` を出す
- 公開サイトのプレビュー：http://localhost:8081 （ビルド結果を静的配信。本番の nginx の代わり）
- 管理画面の開発：`pnpm dev`（Vite）。`/api` を 8080 にプロキシする

## ログイン

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `POST /api/auth/login-link` | `{"email": "..."}` | 204。登録の有無にかかわらず同じ応答。登録済みならメールで `/admin/login/verify?token=...` を送る（有効 15 分、1 回限り） |
| `POST /api/auth/verify` | `{"token": "..."}` | 200 `Me`。Cookie を発行。リンクを開いただけではトークンを使わず、画面のボタンでこの API を呼ぶ。無効・期限切れは 400 `invalid_token` |
| `POST /api/auth/logout` | なし | 204 |
| `GET /api/me` | なし | 200 `Me` または 401 |
| `POST /api/me/site` | `{"siteId": "uuid"}` | 200 `Me`。所属していないサイトは 403 |

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
  hasUnpublishedChanges: boolean; // 公開中の版より新しい版がある（一度も公開していなければ false）
  url: string | null;        // 公開中の URL（routes の canonical）
  publishedAt: string | null;
  scheduledAt: string | null; // 予約公開の日時。公開中の記事の更新を予約しているときも入る
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
  scheduledRevisionId: number | null;
};
```

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `GET /api/contents?type=post` | `type`：`post` / `page` | 200 `ContentSummary[]`（番号の降順） |
| `POST /api/contents` | `{"type": "post"}` | 201 `ContentDetail`（空の版を 1 つ作る） |
| `GET /api/contents/{id}` | | 200 `ContentDetail` |
| `PUT /api/contents/{id}/draft` | `{"title": "...", "body": {...}, "fields": {...}, "baseRevisionId": 12}`（`fields`・`baseRevisionId` は省略可） | 200 `ContentDetail`。保存のたびに新しい版を作る。`baseRevisionId`（編集を始めたときの `revision.id`）が最新の版と違えば、ほかの人が先に保存したので 409 `edit_conflict` |
| `PATCH /api/contents/{id}` | `{"slug"?: string \| null, "parentId"?: number \| null, "categoryIds"?: number[], "tagIds"?: number[]}` | 200 `ContentDetail`。公開中に URL が変わったら、古い URL はリダイレクトとして残す |
| `POST /api/contents/{id}/publish` | なし | 200 `ContentDetail`。最新の版を公開し、サイトの再ビルドを予約する |
| `POST /api/contents/{id}/unpublish` | なし | 200 `ContentDetail`。再ビルドを予約する |
| `POST /api/contents/{id}/schedule` | `{"publishAt": "RFC 3339"}`（今から 1 分以上あと） | 200 `ContentDetail`。最新の版をその日時に公開する。もう一度呼ぶと予約を置き換える。過去の日時は 422 `invalid_publish_at` |
| `DELETE /api/contents/{id}/schedule` | | 200 `ContentDetail`。予約を取り消す |
| `DELETE /api/contents/{id}` | | 204。再ビルドを予約する |

`slug` の検査：英小文字・数字・ハイフンのみ、数字だけは不可、1〜100 文字。日本語が入っていたら 422 `invalid_slug`（「ページのURLは英小文字・数字・ハイフンで入力してください」）。

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

カテゴリとタグの slug も同じ検査。重複は 409 `conflict`。slug を変えると、古い URL はリダイレクトとして残る。

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
| `POST /api/media` | multipart：`file`、`altText` | 201 `Media`。画像（JPEG・PNG・GIF・WebP）だけ、10MB まで |
| `GET /api/media` | | 200 `Media[]`（新しい順） |
| `GET /api/media/{id}/file` | | 画像そのもの |
| `PATCH /api/media/{id}` | `{"altText"}` | 200 `Media` |
| `DELETE /api/media/{id}` | | 204。最新・公開中・予約中のどれかの本文で使われていれば 409 `media_in_use` |

本文に画像を入れるときは `image` ノードに `src = media.url`、`mediaId = media.id`、`alt` を入れる。管理画面のエディタでの表示には `mediaId` から `/api/media/{id}/file` を使う。

## サイト

| メソッドとパス | 入力 | 出力 |
| --- | --- | --- |
| `POST /api/site/build` | なし | 202。全体ビルドを予約する（手動で再ビルドしたいとき） |
