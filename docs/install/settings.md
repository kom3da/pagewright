# 設定を変える

## 画像の容量の上限

はじめは上限なしです。ディスクを使いすぎないように上限を決めるときは、`.env` に次を書いて `docker compose up -d` を実行します。

| 項目 | 書くもの |
| --- | --- |
| `PAGEWRIGHT_MEDIA_QUOTA_MB` | 1つのサイトに保存できる画像の合計（MB）。`0` か空なら上限なし |

上限を超える画像を追加しようとすると、管理画面に「容量がいっぱい」と出て追加できません。使っていない画像を削除すると、その分だけまた追加できます。

## アプリ用のDBパスワードを変える

```sh
docker compose exec db psql -U pagewright -c "ALTER ROLE pagewright_app PASSWORD '新しいパスワード'"
```

`.env` の `PAGEWRIGHT_APP_DB_PASSWORD` を同じ値に変えて、`docker compose up -d` を実行します。
（`.env` を変えるだけでは、作成済みのデータベースのパスワードは変わりません。`POSTGRES_PASSWORD` も同様に `ALTER ROLE pagewright` で変えます。）
