# 本文JSON スキーマ v1

本文は ProseMirror（Tiptap）の JSON で保存する（`content_revisions.body`、`body_schema_version = 1`）。
ノード名・マーク名は Tiptap の標準拡張に合わせる。ここにないノードやマークは保存しない。

## ノード

| ノード | 属性 | 中身 | 備考 |
| --- | --- | --- | --- |
| `doc` | なし | ブロック 0 個以上 | ルート |
| `paragraph` | なし | インライン | |
| `heading` | `level`: 2〜4 | インライン | h1 はページタイトルだけが使う |
| `bulletList` | なし | `listItem` 1 個以上 | |
| `orderedList` | `start`: 1 以上（省略時 1） | `listItem` 1 個以上 | |
| `listItem` | なし | ブロック 1 個以上 | |
| `blockquote` | なし | ブロック 1 個以上 | |
| `horizontalRule` | なし | なし | |
| `image` | `src`（必須）、`alt`（必須・空不可）、`mediaId`（メディアの ID、移行で外部画像のときは null）、`title` | なし | `src` は公開サイトでのパス（`/media/...`）か外部 URL |
| `table` | なし | `tableRow` 1 個以上 | Tiptap の table 拡張 |
| `tableRow` | なし | `tableHeader` / `tableCell` | |
| `tableHeader` / `tableCell` | `colspan`、`rowspan` | ブロック 1 個以上 | |
| `embed` | `provider`: `youtube` / `googlemap`、`url` | なし | 出力時に iframe を組み立てる。任意の iframe は入れない |
| `rawHtml` | `html` | なし | 移行の受け皿。管理者だけが作成・編集でき、出力時にサニタイズする |
| `contactForm` | なし | なし | お問い合わせフォーム（固定の項目）を差し込む。送信先は Pagewright 本体 |
| `hardBreak` | なし | なし | |
| `text` | `text` | なし | `marks` を持てる |

## マーク

| マーク | 属性 | 出力 |
| --- | --- | --- |
| `bold` | なし | `<strong>` |
| `italic` | なし | `<em>` |
| `code` | なし | `<code>` |
| `link` | `href`（必須。`https://`・`http://`・`/` で始まるサイト内のパス・`#`・`mailto:`・`tel:`（数字と + ( ) - . 空白）だけ）、`target`（`_blank` のみ許可） | `<a>`。`_blank` のときは `rel="noopener noreferrer"` を付ける |
| `ruby` | `rt`（必須、ふりがな） | `<ruby>本文<rp>(</rp><rt>ふりがな</rt><rp>)</rp></ruby>` |

色・文字サイズのマークは持たない（テーマで決める）。

## 例

```json
{
  "type": "doc",
  "content": [
    { "type": "heading", "attrs": { "level": 2 }, "content": [{ "type": "text", "text": "会社概要" }] },
    {
      "type": "paragraph",
      "content": [
        { "type": "text", "text": "東京", "marks": [{ "type": "ruby", "attrs": { "rt": "とうきょう" } }] },
        { "type": "text", "text": "の会社です。" }
      ]
    }
  ]
}
```

## バージョンを上げるとき

ノードやマークの意味を変える・削るときはバージョンを上げ、古い版から新しい版への変換処理を書く（CLAUDE.md）。
ノードやマークを足すだけなら、古いデータはそのまま読めるのでバージョンは上げなくてよい。
