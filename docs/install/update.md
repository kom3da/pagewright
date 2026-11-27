# アップデート

版は `ghcr.io/kom3da/pagewright:0.1.0` のように、`.env` の `PAGEWRIGHT_IMAGE` で固定しておきます（`latest` は使いません）。
こうしておくと、自分で書き換えたときにだけ新しい版になります。版の付け方と、どの版で注意が要るかは「[版の付け方と互換性](./versioning.md)」にあります。

## 新しい版が出たことを知る

- GitHub の [Pagewright のリポジトリ](https://github.com/kom3da/pagewright) の「Watch」→「Custom」で「Releases」にチェックを入れると、新しい版が出たときに GitHub からメールで知らせが届きます
- 不具合やセキュリティの修正は、**最新の 0.x の版にだけ**入ります。古い版のままにせず、なるべく早く上げてください（「[版の付け方と互換性](./versioning.md)」）

## 新しい版にする

1. [変更履歴](https://github.com/kom3da/pagewright/blob/main/CHANGELOG.md)（リリースノートと同じ内容）で、今の版から新しい版までの「注意」を読みます。設定の追加などがあれば、先に済ませます
2. バックアップを取ります（「[バックアップと復元](./backup.md)」）。**データベースの変更は、版を戻しても元に戻りません。** 戻すときはこのバックアップから復元します

   ```sh
   cd /opt/pagewright/deploy
   docker compose run --rm cli backup
   ```

3. 新しい版の設定ファイルを取得し、`.env` の `PAGEWRIGHT_IMAGE` を新しい版に書き換えます（`v0.2.0` は新しい版に置き換えます）

   ```sh
   cd /opt/pagewright
   git status --short                      # 「 M」で始まる行がなければ、書き換えたファイルはない
   git fetch --tags && git checkout v0.2.0
   cd deploy
   grep PAGEWRIGHT_IMAGE .env.example      # 新しい版の値。これを .env に写す
   ```

   ` M deploy/compose.yaml` のような行が出たときは（`??` で始まる行は自分で足したファイルなので、そのままでかまいません）、次の「[手元で直したファイルがあるとき](#手元で直したファイルがあるとき)」を先に読みます

4. 新しいイメージを取得して起動し直します。マイグレーションは起動のときに自動で行われます（版を飛ばしても、途中の分が順に行われます）

   ```sh
   docker compose pull
   docker compose up -d
   docker compose ps -a
   docker compose run --rm cli version   # 新しい版になったか確かめる
   ```

マイグレーションに失敗すると `cli` が `Exited (1)` になり、`app` は起動しません（`docker compose logs cli` で理由を確かめる）。

## 手元で直したファイルがあるとき

`compose.yaml` や `nginx/templates/default.conf.template` など、Pagewright のリポジトリに入っているファイルを書き換えていると、`git checkout` が「ローカルの変更が上書きされる」と言って止まります。

- **これからは、なるべく書き換えずに済ませます。** `compose.yaml` を変えたいときは `compose.override.yaml` に書きます（「[HTTPS と構成の変更](./https.md#構成を変えるときは-compose-override-yaml-に書く)」）。nginx の設定を足すときは、`nginx/templates/` に新しいファイルを作ります（「[2つ目のサイト](./second-site.md)」）。自分で作ったファイルは、`git checkout` で消えたりぶつかったりしません
- **書き換えてしまったファイルは**、いったん脇によけてから新しい版にし、戻します

  ```sh
  cd /opt/pagewright
  git stash                                 # 書き換えた内容を脇によける
  git fetch --tags && git checkout v0.2.0
  git stash pop                             # 書き換えた内容を戻す
  ```

  `git stash pop` が `CONFLICT` と言ったときは、新しい版でも同じところが変わっています。そのファイルを開き、`<<<<<<<` から `>>>>>>>` のあいだを見て、新しい版の内容に自分の変更を入れ直し、`<<<<<<<`・`=======`・`>>>>>>>` の行を消します。終わったら `git reset -q` と `git stash drop` を実行します（よけておいた内容は、もう要らないので消します）

## 前の版に戻す

新しい版で問題が出たときは、アップデートの前に取ったバックアップから、前の版で復元します。
`v0.1.0`・`0.1.0` は前の版に、ファイル名はアップデートの前に取ったバックアップに置き換えます。

```sh
cd /opt/pagewright/deploy
docker compose run --rm cli backup      # 念のため、新しい版の今の状態も取っておく
cd /opt/pagewright
git checkout v0.1.0
cd deploy
nano .env                               # PAGEWRIGHT_IMAGE を前の版（ghcr.io/kom3da/pagewright:0.1.0）に戻す
docker compose pull
docker compose stop app
docker compose run --rm cli restore --force --no-backup /backups/pagewright-20261127T180000Z.tar.gz
docker compose up -d
docker compose run --rm cli version     # 前の版になったか確かめる
```

`--no-backup` は、上書きする前のバックアップを取らない指定です（最初の行で、新しい版のうちに取ってあるため）。
アップデートのあとに書いた記事などは、戻したバックアップには入っていないので、消えます。

## OS や Docker のアップデート

サーバーの OS や Docker 自体のアップデート（`sudo apt upgrade` など）は、Pagewright とは別に、ふだんどおり行ってかまいません。

- Docker を更新したり、サーバーを再起動したりすると、コンテナはいったん止まります。`compose.yaml` のサービスは `restart: unless-stopped` なので、Docker が起動し直すと自動で動き出します
- 終わったら `docker compose ps -a` で、`app`・`db`・`nginx`・`caddy` が `Up` になっているか確かめます（「[困ったとき](./troubleshooting.md#サーバーを再起動したあと)」）
- Docker Engine は 27 以降のままにしてください

PostgreSQL の大きな版（18 → 19 など）を上げるときは、別の手順が必要です（いまは 18 のままにしてください）。
