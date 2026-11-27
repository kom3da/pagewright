# バックアップと復元

`backup` コマンドで、すべてのサイトのデータベースの中身と画像を1つのファイル（`pagewright-20261127T180000Z.tar.gz` のような名前。日時は UTC）にまとめます。
サービスを止めずに取れます。

| 入るもの | 入らないもの |
| --- | --- |
| 記事（古い版も）、カテゴリ・タグ、メニュー、サイトの設定、お問い合わせ、ユーザー、画像 | 公開サイトのファイル（復元のあとで作り直す）、ログインの状態（復元したら全員ログインし直す）、返事待ちの招待（復元したら招待し直す）、送信待ちのメール、`.env` |

`.env` はバックアップに入らないので、別の安全な場所に控えておきます。
記事の古い版（保存の履歴）は自動で消えます（30日より前の版は、記事ごとに新しい50個と公開中・予約中の版だけ残ります）。それより前の内容が必要になったときは、バックアップから戻します。

## 保存先を用意する（最初に1回だけ）

バックアップは `deploy/backups/` に保存されます。`cli` はユーザーID 10001 で動くので、その持ち主にしておきます。

```sh
cd /opt/pagewright/deploy
sudo install -d -m 700 -o 10001 -g 10001 backups
```

別の場所（別のディスクなど）に置くときは、そのディレクトリを同じように作り、`.env` に `PAGEWRIGHT_BACKUP_DIR=/var/backups/pagewright` のように書きます。

## バックアップを取る

```sh
docker compose run --rm cli backup --keep 14
```

`--keep 14` を付けると、新しいものから14個を残して古いバックアップを消します。付けなければ消しません。

毎日自動で取るには、root の crontab（`sudo crontab -e`）に次を足します。毎日午前3時（サーバーの時刻）に取ります。

```
0 3 * * * cd /opt/pagewright/deploy && docker compose run --rm -T cli backup --keep 14 >> /var/log/pagewright-backup.log 2>&1
```

## できているか確かめる

保存先は作った人（ユーザーID 10001）しか読めないので、`sudo` を付けて見ます。

```sh
sudo ls -lh /opt/pagewright/deploy/backups/
sudo tail /var/log/pagewright-backup.log   # cron で取ったときの記録
```

新しい日付のファイルが毎日増えていれば、うまく取れています。
cron の失敗は、だれにも知らされません。ときどき（たとえば週に1回）このコマンドで確かめるか、サーバーの監視のしくみで「新しいファイルができていない」ことを知らせるようにしておきます。
記録のファイル（`/var/log/pagewright-backup.log`）は少しずつ大きくなります。気になるときは、Linux の `logrotate` で古い記録を消すように設定します。

## 別の場所にコピーする

同じサーバーのディスクにあるバックアップは、サーバーやディスクが壊れると一緒に失われます。
別のサーバーや S3 互換ストレージなどへのコピーは、Pagewright では行いません。上の cron のあとに、`rsync` や `rclone` などでコピーする設定を各自で足してください。
**`.env` も一緒に**コピーしておきます（復元のときに要ります）。

たとえば、SSH で入れる別のサーバー（`backup.example.net`）へ `rsync` でコピーするなら、root の crontab の上の行の後ろに足します（root から鍵で SSH できるようにしておきます）。

```
30 3 * * * rsync -a /opt/pagewright/deploy/backups/ /opt/pagewright/deploy/.env backup@backup.example.net:pagewright/
```

`rclone`（S3 互換ストレージなどへコピーする道具）なら、`rclone config` で送り先（下の例では `remote`）を作っておいて、次のようにします。

```
30 3 * * * rclone copy /opt/pagewright/deploy/backups remote:pagewright-backups && rclone copy /opt/pagewright/deploy/.env remote:pagewright-backups/env
```

コピー先でも、ほかの人が読めないようにしてください（次の「扱いの注意」）。

## バックアップの扱いの注意

- お問い合わせの内容（名前、メールアドレス、電話番号、本文）など、**個人情報が入っています**。ファイルは作った人（ユーザーID 10001）しか読めない設定（`600`）で作られます。コピー先でもほかの人が読めないようにし、外に出すときは暗号化してください（`age` や `gpg` など）
- ログインの状態（セッション）とログイン用リンクは入れていません。バックアップが漏れても、そこからログインはできません
- データベースは取り始めた時点の内容で、画像はそのあとにコピーします。バックアップ中に追加された画像が、データベースには載らずに入ることがあります（害はありません）。バックアップ中に削除した画像は、データベースに載っていてもファイルが入らないことがあります

## 復元する

ふだんから、別のサーバー（試しに借りた小さな VPS など）で、下の「新しいサーバーに戻す」を一度練習しておくと安心です。いざというときに、手順やファイルが足りないことに気づけます。

復元のときは、ファイルが壊れていないか確かめ、データベースを今の版の形にして、すべての公開サイトを作り直します（`app` が動いていれば、すぐに始まります）。

- 復元先は、**まだサイトもユーザーもないデータベース**です。新しいサーバーでは、[導入](./setup.md)の手順5（`init`）を**行わずに**復元します。データがあるときは「復元先にデータがあります」と出て止まります（上書きするときは、下の `--force`）
- バックアップを取ったときより古い版の Pagewright では復元できません（「このバックアップはより新しい版の Pagewright で取られました」と出て止まります）。同じか新しい版を使ってください
- サイトのIDは、バックアップを取ったときと同じものに戻ります。控えておいた `.env` の `PAGEWRIGHT_SITE_ID` をそのまま使えます

### 新しいサーバーに戻す

1. 新しいサーバーで、[導入](./setup.md)の手順1と2を行います。`.env` は、控えておいたものを `/opt/pagewright/deploy/.env` に置きます（`chmod 600 .env`）
2. 控えておいたバックアップのファイル（例：`pagewright-20261127T180000Z.tar.gz`）を、`/opt/pagewright/deploy` に置きます（`scp` などで）
3. 次を実行します

```sh
cd /opt/pagewright/deploy
sudo install -d -m 700 -o 10001 -g 10001 backups
sudo install -m 600 -o 10001 -g 10001 pagewright-20261127T180000Z.tar.gz backups/
docker compose up -d db                       # DBだけ起動する（アプリ用ユーザーが作られる）
docker compose run --rm cli restore /backups/pagewright-20261127T180000Z.tar.gz
docker compose up -d
docker compose run --rm cli login-url --email you@example.co.jp
```

`restore` に渡すのは、`cli` の中から見たパス（`/backups/ファイル名`）です。

4. DNS の A レコード（と AAAA レコード）を、新しいサーバーの IP アドレスに向け直します（「[導入](./setup.md)」の手順3）。HTTPS の証明書はバックアップに入っていませんが、DNS が新しいサーバーを指すと、Caddy が自動で取り直します

### 同じサーバーで前の状態に戻す

今のデータを消して前の状態に戻すときは、`app` を止めてから `--force` を付けます。
**今のデータベースの中身と画像は消えます。** そのため `--force` は、消す前に今の状態を `backups/` にバックアップし、そのファイル名を表示します（保存先は `--out` で変えられます）。
バックアップのファイルの確認やデータベースの形の確認で問題が見つかったときは、何も消さずに止まります。

```sh
docker compose stop app
docker compose run --rm cli restore --force /backups/pagewright-20261127T180000Z.tar.gz
docker compose up -d
```

ログインの状態は戻らないので、全員ログインし直します（メールが届かないときは `login-url`）。

**復元するのは、自分（またはこのサーバーの担当者）が取ったバックアップだけにしてください。** 復元はデータベースの中身をそのまま入れるので、
人から受け取ったファイルや出どころのわからないファイルを復元すると、知らない管理者のアカウントやサイトが入ることがあります。

Docker を使わずに `pagewright` を動かしているときも同じコマンドです。`PAGEWRIGHT_MIGRATE_DATABASE_URL`（テーブルの持ち主）と `PAGEWRIGHT_DATA_DIR` を設定して `pagewright backup --out /var/backups/pagewright --keep 14`、`pagewright restore ファイル` を実行します。PostgreSQL の `pg_dump` は使わないので、入れる必要はありません。
