# 管理画面（web/admin）

React＋Vite＋TypeScript。ルーティングは TanStack Router、データ取得は TanStack Query、
部品と見た目は Mantine、フォームは React Hook Form＋Zod、本文エディタは Tiptap。
画面の作り方・見た目・言葉の決まりは [`DESIGN.md`](DESIGN.md)。
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
mise exec -- pnpm e2e        # Playwright＋axe。モック版をビルドして :5175 で動かす（PW_PORT=5197 のように変えられる）
```

- `dev:mock` は最初からログイン済み。ログインの確認は `/admin/login/verify?token=valid-token`。データはページを読み込み直すと初期状態に戻る
- 公開サイトの URL（一覧の「公開中のURL」）は `VITE_PUBLIC_SITE_ORIGIN`（既定 `http://localhost:8081`）
- E2E を初めて動かすときは `mise exec -- pnpm exec playwright install chromium`

## 決まりごと

- API の呼び出しは `src/api/client.ts` に集める。型は `src/api/types.ts`（約束と同じ形に保つ）
- 画面の言葉は、非エンジニアにわかる日本語にする（「スラッグ」ではなく「ページのURL（英数字）」）
- 部品は Mantine と `src/components/ui/` の共通部品を使う（Tailwind や自作のボタンは使わない）。詳しくは `DESIGN.md`
- 入力欄にはラベルを付け、エラーは欄の `error` にひもづける。確認には `window.confirm` ではなく `confirm()`（`src/components/ui/confirm.tsx`）を使う
- 本文に新しいノードやマークを足すときは、`src/editor/extensions.ts` と `src/editor/normalize.ts` の両方を直す
