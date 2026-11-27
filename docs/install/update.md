# アップデート

1. 先にバックアップを取ります（「[バックアップと復元](./backup.md)」）
2. 新しい版の設定ファイルを取得し、`.env` の `PAGEWRIGHT_IMAGE` を新しい版に書き換えます

   ```sh
   cd /opt/pagewright
   sudo git fetch --tags && sudo git checkout v1.1.0
   cd deploy
   ```

3. 新しいイメージを取得して起動し直します。マイグレーションは起動のときに自動で行われます

   ```sh
   docker compose pull
   docker compose up -d
   docker compose ps
   ```

リリースノートに書かれている注意点（設定の追加など）があれば、先に確認してください。
PostgreSQL の大きな版（18 → 19 など）を上げるときは、別の手順が必要です（いまは 18 のままにしてください）。
