# 本文の HTML

`Content.Body` は、本文の JSON（[本文JSONのスキーマ](../dev/body-schema.md)。元は `schema/body-v1.md`）から本体が組み立てた安全な HTML です。テーマは CSS で見た目を付けます。付くクラスは次だけです。

| 要素 | クラス |
| --- | --- |
| 画像 | `<figure class="image"><img …></figure>`（`width`・`height` は分かるときだけ） |
| 表 | `<div class="table-wrap" role="region" tabindex="0" aria-label="（labels.table）"><table>…`。幅の広い表をキーボードでも横にスクロールできるように、囲みにフォーカスが当たります。`:focus-visible` の見た目を付けてください |
| YouTube・Google マップ | `<div class="embed embed-youtube">` / `embed-map` |
| 移行した HTML | `<div class="raw-html">` |
| ルビ | `<ruby>…<rp>(</rp><rt>…</rt><rp>)</rp></ruby>` |

見出しは `h2`〜`h4`、ほかは素の `p`、`ul`、`ol`、`blockquote`、`hr`、`code`、`a` です。

## 部品（`templates/parts/`）

本文の中のブロックのうち、見た目をテーマに任せるものは部品として呼びます。

- **`contact-form`**：お問い合わせフォーム。受け取るデータは `.Action`（送信先）、`.Page`（このページのパス）、`.PrivacyURL`（個人情報の取り扱いのページ。なければ空）です
  - 送る項目名は固定です：`name`（必須）、`kana`、`email`（必須）、`phone`、`message`（必須）、`consent=yes`（必須）、`page`
  - `website` はスパム対策の隠し欄です。人には見せず（`aria-hidden="true"`、`tabindex="-1"`）、空のまま送られるようにしてください
  - 部品がなければ、フォームのブロックは出力されません
  - 送信して入力に問題があったときは、結果のページ（`form.html`）でこの部品をもう一度呼びます。そのときは `.Values`（入力された内容：`Name`、`Kana`、`Email`、`Phone`、`Message`、`Consent`）と `.Errors`（項目名ごとのメッセージ。`{{with index .Errors "email"}}…{{end}}` のように使う）が入っています。入力欄に値を戻し、問題のある欄には `aria-invalid="true"` とメッセージ（`aria-describedby` で結びつける）を付けてください。ふだんのページでは、どちらも空です
