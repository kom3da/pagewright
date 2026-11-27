---
layout: home

hero:
  name: Pagewright
  text: 日本語に強い、セルフホストでも速いCMS
  tagline: 中小企業・個人のWebサイト向け。管理画面は動的、公開サイトは静的に書き出します。開発中です。
  actions:
    - theme: brand
      text: 使い方を見る
      link: /guide/
    - theme: alt
      text: 導入する
      link: /install/
    - theme: alt
      text: テーマを作る
      link: /themes/
    - theme: alt
      text: 開発する
      link: /dev/

features:
  - title: 使い方
    details: 管理画面で記事やページを書いて公開する方法。ログイン、画像、お問い合わせへの対応、困ったときの対処。ふだん管理画面を使う人向け。
    link: /guide/
    linkText: 使い方へ
  - title: 導入・運用
    details: Docker Compose で PostgreSQL・Pagewright・nginx・Caddy をまとめて動かす手順と、バックアップ・アップデート・困ったときの対処。
    link: /install/
    linkText: 導入・運用へ
  - title: テーマを作る
    details: 公開サイトの見た目を決めるテーマの作り方。テーマと本体の約束（ファイル構成、渡るデータ、本文のHTML）。
    link: /themes/
    linkText: テーマを作るへ
  - title: 開発
    details: 設計ドキュメント、開発環境、管理画面の API とデザインの決まり、本文JSONのスキーマ。
    link: /dev/
    linkText: 開発へ
---

## Pagewright とは

中小企業・個人のWebサイト向けの、セルフホストでも速く、日本語に強いCMS。

- 管理画面は動的、公開サイトは静的に書き出すので、どのCDNやWebサーバーでも速く配信できる
- 日本語の全文検索とルビに対応（縦書きと和暦は v1 以降の予定）
- WordPress などからの移行機能つき

本体は AGPL-3.0、テーマとプラグインのSDKは MIT（予定）。
