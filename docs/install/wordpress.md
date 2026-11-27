# WordPress からの移行

WordPress の「ツール → エクスポート」で書き出したファイル（WXR）を取り込めます。
まず `--dry-run` で、変換できないものの一覧を確認します。

```sh
docker compose run --rm -v "$PWD/export.xml:/import/export.xml:ro" cli \
  import-wxr --site <サイトのID> --author-email you@example.co.jp --media-root /data/media --dry-run /import/export.xml
```

問題がなければ `--dry-run` を外して実行します。画像も取り込むときは `--download-media` を付けます。
取り込みが終わると、公開サイトが書き出されます。
