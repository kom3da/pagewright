# Pagewright の導入手順（サーバー1台）

会社のWebサイトを1つ、自社で用意したサーバーで動かすための手順です。
Docker Compose で、データベース（PostgreSQL）ごとまとめて動かします。

```
閲覧者 ─HTTPS→ caddy ─→ nginx ─┬→ 公開サイト（書き出した静的ファイル）
                                └→ 管理画面（app） ─→ db（PostgreSQL）
```

- **公開サイト**（例：`www.example.co.jp`）は、管理画面で公開したときに書き出される静的ファイルです。管理画面が止まっていても表示され続けます。
- **管理画面**（例：`admin.example.co.jp`）は、記事を書いたり公開したりする画面です。
- HTTPS の証明書は Caddy が自動で取得・更新します。

使う設定ファイルはすべて `deploy/` にあります。

| ファイル | 中身 |
| --- | --- |
| `deploy/compose.yaml` | 全体の構成 |
| `deploy/.env.example` | 設定の見本（`.env` にコピーして使う） |
| `deploy/nginx/` | 公開サイトの配信と、管理画面への中継の設定 |
| `deploy/caddy/Caddyfile` | HTTPS の設定 |
| `deploy/postgres/init/` | 最初の起動時に、アプリ用のDBユーザーを作る |

## 必要なもの

- **Linux のサーバー1台**（VPS やクラウドのサーバー）。x86_64 か arm64。メモリ 2GB 以上、ディスク 20GB 以上を目安に（画像が多ければ増やす）
- **Docker Engine と Docker Compose v2**（`docker compose version` で確認）
- **ドメイン2つ**：公開サイト用と管理画面用。DNS を設定できること
- **ポート 80 と 443** が外から届くこと（証明書の取得に使う）
- **メールを送れる SMTP サーバー**（ログイン用リンクを送る。後述の「メールの設定」）

## 導入

### 1. ファイルを取得する

```sh
sudo git clone --branch v1.0.0 https://github.com/kom3da/pagewright.git /opt/pagewright
cd /opt/pagewright/deploy
```

`v1.0.0` は [リリース一覧](https://github.com/kom3da/pagewright/releases) にある最新の版に置き換えてください。以降のコマンドはすべて `deploy/` で実行します。

### 2. 設定ファイル（.env）を作る

```sh
cp .env.example .env
chmod 600 .env
openssl rand -hex 32   # パスワードを作る。2回実行して、それぞれ下の2か所に貼る
```

`.env` を開いて、次を書き換えます。

| 項目 | 書くもの |
| --- | --- |
| `PAGEWRIGHT_SITE_DOMAIN` | 公開サイトのドメイン |
| `PAGEWRIGHT_ADMIN_DOMAIN` | 管理画面のドメイン |
| `POSTGRES_PASSWORD` | DBの管理用パスワード（上で作ったもの） |
| `PAGEWRIGHT_APP_DB_PASSWORD` | アプリがDBに接続するときのパスワード（もう1つ作ったもの） |
| `PAGEWRIGHT_SMTP_ADDR` | メールを送る SMTP サーバー（`ホスト名:ポート`） |
| `PAGEWRIGHT_MAIL_FROM` | メールの差出人 |
| `PAGEWRIGHT_IMAGE` | 使う版。`ghcr.io/kom3da/pagewright:1.0.0` のように、手順1と同じ版に固定する |

`PAGEWRIGHT_SITE_ID` は手順5で書くので、いまは空のままにします。
パスワードは英数字だけにしてください（記号が入ると接続できないことがあります）。

### 3. DNS を設定する

2つのドメインの A レコード（IPv6 を使うなら AAAA レコードも）を、サーバーのIPアドレスに向けます。
DNS が反映される前に起動すると証明書の取得に失敗します。その場合も、反映後しばらくすると自動でやり直されます。

### 4. 起動する

```sh
docker compose up -d
docker compose ps
```

起動のたびに、まず `cli` がデータベースを最新の形にして（マイグレーション）終了し、そのあと `app` が起動します。
`cli` が `exited (0)`、ほかが `running`（`app` と `db` は `healthy`）になっていれば成功です。

### 5. サイトと管理者を作る

```sh
docker compose run --rm cli init --site "株式会社サンプル" --email you@example.co.jp
```

次のように表示されます。

```
サイト「株式会社サンプル」を作りました（ID: 0193xxxx-xxxx-7xxx-xxxx-xxxxxxxxxxxx）
you@example.co.jp のログイン用URL（1時間有効、1回限り）:
https://admin.example.co.jp/admin/login/verify?token=...
```

1. 表示された **ID** を `.env` の `PAGEWRIGHT_SITE_ID=` に書き、`docker compose up -d` をもう一度実行します（公開サイトの配信先が決まります）
2. 表示された **URL** をブラウザで開き、「ログイン」を押します

### 6. 公開する

管理画面で記事や固定ページを作って公開すると、公開サイトが書き出されます。
最初に一度、管理画面の「サイトを更新」を押しておくと、トップページなどが作られます。
`https://<公開サイトのドメイン>/` を開いて表示を確かめてください。

2回目からは、管理画面のログイン画面でメールアドレスを入れると、ログイン用リンクがメールで届きます。

## HTTPS について

標準の構成では Caddy が Let's Encrypt の証明書を自動で取得・更新します。設定は不要です。

クラウドのロードバランサー（AWS の ALB など）で HTTPS を受ける場合は、`compose.yaml` の `caddy` サービスを消し、
`nginx` に `ports: ["80:8080"]` を足して、ロードバランサーからサーバーの 80 番へ転送してください。
サーバーの 80 番には、ロードバランサー以外から届かないようにしておきます。
ロードバランサーは `X-Forwarded-For` に閲覧者のIPを付け足すもの（ALB などの HTTP のロードバランサー）にしてください。

### Caddy もロードバランサーも置かずに nginx を直接公開する場合

標準の設定は「手前に Caddy かロードバランサーがいる」前提で、プライベートIP（`10.0.0.0/8`・`172.16.0.0/12`・`192.168.0.0/16`）から
届いた `X-Forwarded-For` を信じます（app の `PAGEWRIGHT_TRUSTED_PROXIES` と、nginx の `set_real_ip_from`）。
nginx を直接インターネットに公開すると、閲覧者の接続が Docker のゲートウェイ（プライベートIP）から来たように見えることがあり、
閲覧者が `X-Forwarded-For` を偽ってお問い合わせフォームやログインの回数制限をすり抜けられてしまいます。

この構成では、信じる相手を nginx だけにしてください。

1. `compose.yaml` で nginx のIPを固定し、app の `PAGEWRIGHT_TRUSTED_PROXIES` をそのIPだけにする

   ```yaml
   services:
     app:
       environment:
         PAGEWRIGHT_TRUSTED_PROXIES: "172.30.0.10" # nginx だけ
     nginx:
       networks:
         default:
           ipv4_address: 172.30.0.10
   networks:
     default:
       ipam:
         config:
           - subnet: 172.30.0.0/24
   ```

2. `nginx/templates/default.conf.template` の `set_real_ip_from` の3行と `real_ip_header` の行を消す

こうすると、Docker のゲートウェイを通る接続では閲覧者ごとの区別ができず、回数制限がまとめてかかることがあります。
閲覧者ごとに制限したいときは、標準の Caddy を使ってください。

## メールの設定

Pagewright はログイン用リンクとお問い合わせの通知をメールで送ります。`.env` に次を書きます。

| 設定 | 内容 |
| --- | --- |
| `PAGEWRIGHT_SMTP_ADDR` | SMTP サーバー（例：`smtp.example.co.jp:587`） |
| `PAGEWRIGHT_SMTP_USERNAME`・`PAGEWRIGHT_SMTP_PASSWORD` | 認証が必要なサービスのとき。パスワードは TLS の上でしか送りません（サーバーが STARTTLS に対応しているか、次の implicit） |
| `PAGEWRIGHT_SMTP_TLS` | 465 番ポートのように最初から TLS で話すサーバーなら `implicit`。`starttls` にすると、STARTTLS に対応していないサーバーには送りません（暗号化なしでメールを送らないため。外部のメール送信サービスを使うときにおすすめ）。空のときは、サーバーが対応していれば STARTTLS を使い、対応していなければ暗号化せずに送ります（同じマシンの中継サーバーや Mailpit 向け） |

メール送信サービス（SendGrid、Amazon SES など）の SMTP、Google Workspace の SMTP リレー、社内の中継サーバーのどれでも使えます。

SPF・DKIM の設定がないと、迷惑メールに入りやすくなります。差出人のドメインの DNS も確認してください。

メールが届かなくても、次のコマンドでログイン用URLを出せます（後述の「困ったとき」）。

## 画像の容量の上限

はじめは上限なしです。ディスクを使いすぎないように上限を決めるときは、`.env` に次を書いて `docker compose up -d` を実行します。

| 項目 | 書くもの |
| --- | --- |
| `PAGEWRIGHT_MEDIA_QUOTA_MB` | 1つのサイトに保存できる画像の合計（MB）。`0` か空なら上限なし |

上限を超える画像を追加しようとすると、管理画面に「容量がいっぱい」と出て追加できません。使っていない画像を削除すると、その分だけまた追加できます。

## バックアップ

本体にバックアップ機能が入るまでは、次の2つを毎日保存してください。

- **データベース**（記事、設定、ユーザーなど）
- **画像**（`/data/media`）

公開サイトのファイル（`/data/site`）は、データベースと画像から作り直せるので保存しなくてかまいません。
記事の古い版（保存の履歴）は自動で消えます（30日より前の版は、記事ごとに新しい50個と公開中・予約中の版だけ残ります）。それより前の内容が必要になったときは、バックアップから戻します。
`.env` も別の安全な場所に控えておきます。

```sh
mkdir -p /var/backups/pagewright
cd /opt/pagewright/deploy
docker compose exec -T db pg_dump -U pagewright -Fc pagewright > /var/backups/pagewright/db-$(date +%F).dump
docker compose exec -T app tar -czf - -C /data media > /var/backups/pagewright/media-$(date +%F).tar.gz
```

毎日自動で行うには、root の crontab（`sudo crontab -e`）に次を足します（`%` は `\%` と書く）。
古いファイルの削除や、別のサーバー・ストレージへのコピーも合わせて設定してください。

```
0 3 * * * cd /opt/pagewright/deploy && docker compose exec -T db pg_dump -U pagewright -Fc pagewright > /var/backups/pagewright/db-$(date +\%F).dump && docker compose exec -T app tar -czf - -C /data media > /var/backups/pagewright/media-$(date +\%F).tar.gz
```

### 復元する

新しいサーバーに戻すときは、導入の手順1〜3（`.env` は控えておいたもの）を行ってから:

```sh
docker compose up -d db                       # DBだけ起動する（アプリ用ユーザーが作られる）
docker compose exec -T db pg_restore -U pagewright -d pagewright --clean --if-exists < db-2026-01-01.dump
docker compose run --rm --no-deps -T --entrypoint tar cli -xzf - -C /data < media-2026-01-01.tar.gz
docker compose up -d
docker compose run --rm cli login-url --email you@example.co.jp
```

ログインして「サイトを更新」を押すと、公開サイトが書き出されます。

## アップデート

1. 先にバックアップを取ります（上の「バックアップ」）
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

## WordPress からの移行

WordPress の「ツール → エクスポート」で書き出したファイル（WXR）を取り込めます。
まず `--dry-run` で、変換できないものの一覧を確認します。

```sh
docker compose run --rm -v "$PWD/export.xml:/import/export.xml:ro" cli \
  import-wxr --site <サイトのID> --author-email you@example.co.jp --media-root /data/media --dry-run /import/export.xml
```

問題がなければ `--dry-run` を外して実行します。画像も取り込むときは `--download-media` を付けます。
取り込みが終わると、公開サイトが書き出されます。

## アプリ用のDBパスワードを変える

```sh
docker compose exec db psql -U pagewright -c "ALTER ROLE pagewright_app PASSWORD '新しいパスワード'"
```

`.env` の `PAGEWRIGHT_APP_DB_PASSWORD` を同じ値に変えて、`docker compose up -d` を実行します。
（`.env` を変えるだけでは、作成済みのデータベースのパスワードは変わりません。`POSTGRES_PASSWORD` も同様に `ALTER ROLE pagewright` で変えます。）

## 困ったとき

### ログイン用のメールが届かない

次のコマンドでログイン用URLを出せます（1時間有効、1回限り）。

```sh
docker compose run --rm cli login-url --email you@example.co.jp
```

送信の失敗はログで確認できます：`docker compose logs app | grep -i mail`

### 状態とログを見る

```sh
docker compose ps
docker compose logs -f app      # 本体
docker compose logs nginx caddy # 配信と HTTPS
curl -s https://<管理画面のドメイン>/healthz   # ok と出れば本体とDBは動いている
```

### 公開サイトが 404 になる

- `.env` の `PAGEWRIGHT_SITE_ID` に `init` で表示されたIDを書き、`docker compose up -d` を実行したか
- 管理画面で記事を公開したか、「サイトを更新」を押したか（`docker compose logs app | grep "site built"` で書き出しの記録を確認できる）

### 公開しても公開サイトが更新されない（ログに pagefind のエラー）

サイト内検索の索引を作るプログラム（Pagefind）が動かないと、書き出し全体が失敗します。
`docker compose logs app | grep pagefind` に `Illegal instruction` などが出ている場合は、
サーバーのCPUに対応していない可能性があります。`.env` に `PAGEWRIGHT_PAGEFIND=off` を書いて
`docker compose up -d` を実行すると、検索なしで書き出せます。

### HTTPS でつながらない

- DNS がサーバーのIPアドレスを指しているか（`dig +short <ドメイン>`）
- ポート 80 と 443 がファイアウォールで開いているか
- `docker compose logs caddy` に証明書のエラーが出ていないか

## この構成でできないこと

- 1つの構成で複数のサイトを公開すること（管理画面では複数のサイトを扱えますが、配信の設定は1サイト分です）。
  配信だけの問題ではありません。公開サイトのURL（`PAGEWRIGHT_PUBLIC_BASE_URL`）は全サイトで1つなので、
  2つ目以降のサイトを書き出すと、canonical・サイトマップ・RSS・プレビューのリンク先・お問い合わせ送信後の「戻る」リンクが、
  すべて1つ目のサイトのドメインを指します。今のところ、公開するサイトは1つの構成につき1つにしてください
- 管理画面を社内ネットワークの中だけに置くこと（Caddy の設定で IP 制限をかけるなどの工夫が必要です）
