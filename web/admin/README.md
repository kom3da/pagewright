# 管理画面（web/admin）

React＋Vite＋TypeScript。ルーティングは TanStack Router、データ取得は TanStack Query、
スタイルは Tailwind CSS（ダイアログなどの部品は Radix）、フォームは React Hook Form＋Zod、本文エディタは Tiptap。
API の約束は [`docs/dev/admin-api.md`](../../docs/dev/admin-api.md)、本文 JSON は [`schema/body-v1.md`](../../schema/body-v1.md)。

## 開発

Node.js と pnpm は mise で入る（リポジトリ直下で `mise install`）。

```sh
cd web/admin
mise exec -- pnpm install

mise exec -- pnpm dev        # http://localhost:5173/admin/ 。/api は Go 本体（:8080）へプロキシ
mise exec -- pnpm dev:mock   # Go 本体なしで、モックの API（MSW）で動かす
mise exec -- pnpm test       # Vitest（API は MSW でモック。axe による検査も含む）
mise exec -- pnpm lint       # Biome（`pnpm format` で自動修正）
mise exec -- pnpm build      # dist/ に出力。Go 本体が go:embed で /admin/ に埋め込む
mise exec -- pnpm e2e        # Playwright＋axe。モック版をビルドして :5175 で動かす
```

- `dev:mock` は最初からログイン済み。ログインの確認は `/admin/login/verify?token=valid-token`。データはページを読み込み直すと初期状態に戻る
- 公開サイトの URL（一覧の「公開中のURL」）は `VITE_PUBLIC_SITE_ORIGIN`（既定 `http://localhost:8081`）
- E2E を初めて動かすときは `mise exec -- pnpm exec playwright install chromium`

## 決まりごと

- API の呼び出しは `src/api/client.ts` に集める。型は `src/api/types.ts`（約束と同じ形に保つ）
- 画面の言葉は、非エンジニアにわかる日本語にする（「スラッグ」ではなく「ページのURL（英数字）」）
- 入力欄にはラベルを付け、エラーは `Field` で欄にひもづける。確認には `window.confirm` ではなく `ConfirmDialog` を使う
- 本文に新しいノードやマークを足すときは、`src/editor/extensions.ts` と `src/editor/normalize.ts` の両方を直す
