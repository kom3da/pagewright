# アップデート

版は `ghcr.io/kom3da/pagewright:0.1.0` のように、`.env` の `PAGEWRIGHT_IMAGE` で固定しておきます（`latest` は使いません）。
こうしておくと、自分で書き換えたときにだけ新しい版になります。版の付け方と、どの版で注意が要るかは「[版の付け方と互換性](./versioning.md)」にあります。

1. [変更履歴](https://github.com/kom3da/pagewright/blob/main/CHANGELOG.md)（リリースノートと同じ内容）で、今の版から新しい版までの「注意」を読みます。設定の追加などがあれば、先に済ませます
2. バックアップを取ります（「[バックアップと復元](./backup.md)」）。**データベースの変更は、版を戻しても元に戻りません。** 戻すときはこのバックアップから復元します
3. 新しい版の設定ファイルを取得し、`.env` の `PAGEWRIGHT_IMAGE` を新しい版に書き換えます

   ```sh
   cd /opt/pagewright
   sudo git fetch --tags && sudo git checkout v0.2.0
   cd deploy
   grep PAGEWRIGHT_IMAGE .env.example   # 新しい版の値。これを .env に写す
   ```

4. 新しいイメージを取得して起動し直します。マイグレーションは起動のときに自動で行われます（版を飛ばしても、途中の分が順に行われます）

   ```sh
   docker compose pull
   docker compose up -d
   docker compose ps -a
   docker compose run --rm cli version   # 新しい版になったか確かめる
   ```

マイグレーションに失敗すると `cli` が `Exited (1)` になり、`app` は起動しません（`docker compose logs cli` で理由を確かめる）。
前の版に戻すときは、`.env` の `PAGEWRIGHT_IMAGE` と `git checkout` を前の版にし、バックアップから復元します（「[バックアップと復元](./backup.md)」）。

PostgreSQL の大きな版（18 → 19 など）を上げるときは、別の手順が必要です（いまは 18 のままにしてください）。
