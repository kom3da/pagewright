# 導入

サーバーに Pagewright を入れて、公開サイトと管理画面を動かすまでの手順です。
先に「[はじめに（必要なもの）](./index.md)」の準備と、「[メール送信サービスを用意する](./mail.md)」を済ませておきます。

::: tip 例の値は置き換えて読む
この手順では、次の例の値を使います。自分の値に置き換えて読んでください。

| 例の値 | 意味 |
| --- | --- |
| `www.example.co.jp` | 公開サイトのドメイン |
| `admin.example.co.jp` | 管理画面のドメイン |
| `you@example.co.jp` | 最初の管理者（自分）のメールアドレス |
| `株式会社サンプル` | サイトの名前 |
| `v0.1.0` | 使う版 |
:::

途中でうまくいかないときは「[困ったとき](./troubleshooting.md)」を見てください。

## 1. ファイルを取得する

`v0.1.0` は、[リリース一覧](https://github.com/kom3da/pagewright/releases) にある最新の版に置き換えてから実行します。

```sh
sudo git clone --branch v0.1.0 https://github.com/kom3da/pagewright.git /opt/pagewright
sudo chown -R "$USER" /opt/pagewright   # 以降は sudo なしで .env を作ったり git を使ったりできる
cd /opt/pagewright/deploy
```

以降のコマンドはすべて `/opt/pagewright/deploy` で実行します。使う設定ファイルは、すべてここにあります。

| ファイル | 中身 |
| --- | --- |
| `compose.yaml` | 全体の構成 |
| `.env.example` | 設定の見本（`.env` にコピーして使う） |
| `nginx/` | 公開サイトの配信と、管理画面への中継の設定 |
| `caddy/Caddyfile` | HTTPS の設定 |
| `postgres/init/` | 最初の起動時に、アプリ用のDBユーザーを作る |

## 2. 設定ファイル（.env）を作る

```sh
cp .env.example .env
chmod 600 .env
openssl rand -hex 32   # DBのパスワードを作る。2回実行して、それぞれ下の2か所に貼る
nano .env              # エディターで開く（vi など、使い慣れたものでかまいません）
```

`nano` では、書き換えたら Ctrl＋O、Enter で保存し、Ctrl＋X で閉じます。

次の項目を書き換えます。

| 項目 | 書くもの |
| --- | --- |
| `PAGEWRIGHT_SITE_DOMAIN` | 公開サイトのドメイン（例：`www.example.co.jp`） |
| `PAGEWRIGHT_ADMIN_DOMAIN` | 管理画面のドメイン（例：`admin.example.co.jp`） |
| `POSTGRES_PASSWORD` | DBの管理用パスワード（上で作ったもの） |
| `PAGEWRIGHT_APP_DB_PASSWORD` | アプリがDBに接続するときのパスワード（もう1つ作ったもの） |
| `PAGEWRIGHT_SMTP_ADDR` | メール送信サービスの SMTP サーバー（`ホスト名:ポート`） |
| `PAGEWRIGHT_SMTP_USERNAME` | メール送信サービスのユーザー名 |
| `PAGEWRIGHT_SMTP_PASSWORD` | メール送信サービスのパスワード（API キーなど） |
| `PAGEWRIGHT_MAIL_FROM` | メールの差出人（例：`ウェブサイト管理 <no-reply@example.co.jp>`） |

メールの項目に何を書くかは「[メール送信サービスを用意する](./mail.md)」にあります。
`PAGEWRIGHT_SITE_ID` は手順5で書くので、いまは空のままにします。

::: info `PAGEWRIGHT_IMAGE` はそのままにする
`PAGEWRIGHT_IMAGE` は使う版です。手順1で取得した版（`ghcr.io/kom3da/pagewright:0.1.0` など）がもう入っているので、書き換えません。
:::

`.env` に書くときの注意：

- `=` の前後に空白を入れません（`PAGEWRIGHT_SMTP_ADDR=smtp.example.net:587`）
- DB のパスワード（`POSTGRES_PASSWORD`・`PAGEWRIGHT_APP_DB_PASSWORD`）は英数字だけにします（記号が入ると接続できないことがあります）。上の `openssl rand -hex 32` で作れば英数字だけです
- メール送信サービスのパスワードなどに `#`・`$`・空白が入るときは、値の全体を `'` で囲みます（例：`PAGEWRIGHT_SMTP_PASSWORD='ab#c$d'`）。囲まないと、`#` から後ろがコメントとして無視されたり、`$` が別の意味に読まれたりします

## 3. DNS を設定する

2つのドメイン（`www.example.co.jp` と `admin.example.co.jp`）の A レコードを、サーバーの IP アドレスに向けます。IPv6 でも公開するなら、AAAA レコードも足します。

設定が反映されたかは、サーバーで次のように確かめます。どちらもサーバーの IP アドレスが出れば反映されています。

```sh
dig +short www.example.co.jp
dig +short admin.example.co.jp
```

（`dig` がないときは、Ubuntu なら `sudo apt install bind9-dnsutils`。）
反映には、数分から、長いと数時間かかります。反映される前に起動すると、証明書の取得に失敗します。その場合も、反映されたあとしばらくすると自動でやり直されます。

## 4. 起動する

```sh
docker compose up -d
docker compose ps -a
```

起動のたびに、まず `cli` がデータベースを最新の形にして（マイグレーション。「[用語集](./glossary.md)」）終了し、そのあと `app` が起動します。
`cli` が `Exited (0)`、ほかが `Up`（`app` と `db` は `healthy`）になっていれば成功です（`-a` を付けないと、終わった `cli` は表示されません）。
`cli` が `Exited (1)` のときや、`up` がネットワークを作れずに止まるときは「[困ったとき](./troubleshooting.md#起動しない)」を見てください。サーバーで IPv6 を切っていると、`up` がネットワークを作れずに失敗します。

### 動いているか確かめる

1分ほど待ってから、次を実行します。`ok` と出れば、HTTPS の証明書が取れていて、本体とデータベースが動いています。

```sh
curl https://admin.example.co.jp/healthz
```

証明書のエラーが出るときは、Caddy のログを見ます。`certificate obtained successfully` が2つのドメインの分あれば、証明書は取れています。

```sh
docker compose logs caddy | grep -i certificate
```

## 5. サイトと管理者を作る

`株式会社サンプル` をサイトの名前に、`you@example.co.jp` を自分のメールアドレスに置き換えて実行します。

```sh
docker compose run --rm cli init --site "株式会社サンプル" --email you@example.co.jp
```

次のように表示されます。

```
サイト「株式会社サンプル」を作りました（ID: 0193xxxx-xxxx-7xxx-xxxx-xxxxxxxxxxxx）
you@example.co.jp のログイン用URL（1時間有効、1回限り）:
https://admin.example.co.jp/admin/login/verify?token=...
```

1. 表示された **ID** を `.env` の `PAGEWRIGHT_SITE_ID=` に書き、`docker compose up -d` をもう一度実行します（公開サイトの配信先が決まります）。ID はあとで「[WordPress からの移行](./wordpress.md)」などでも使います
2. 表示された **URL** をブラウザで開き、「ログイン」を押します

このコマンドで出したURLは **1時間** 有効で、1回だけ使えます（ログイン画面から送るメールのリンクは15分です）。
期限が切れたときは、次のコマンドで新しいURLを出せます。

```sh
docker compose run --rm cli login-url --email you@example.co.jp
```

## 6. 公開する

1. 管理画面の「ホーム」の「公開サイトの更新」の枠にある「**サイトを更新**」を押します。トップページなどが作られます
2. `https://www.example.co.jp/` を開いて表示を確かめます

最初の更新が終わるまでは、公開サイトは 404（ページが見つかりません）になります。これはふつうのことです。
そのあとは、管理画面で記事や固定ページを公開するたびに、公開サイトが自動で書き出されます。

最後に、メールが届くか確かめます。

1. 管理画面のメニューのいちばん下の自分の名前を押し、「ログアウト」を押します
2. ログイン画面で自分のメールアドレスを入れて、「ログイン用のリンクを送る」を押します
3. メールが届き、リンクからログインできれば成功です

届かないときは、`docker compose logs app | grep -i mail` で送信のエラーを確かめます（「[困ったとき](./troubleshooting.md#ログイン用のメールが届かない)」）。

ほかに管理画面を使う人は、管理画面の「メンバー」から招待します（`init` で作るのは最初の管理者だけです）。

## 7. バックアップを設定する

バックアップの保存先を用意して、毎日自動で取るようにします（「[バックアップと復元](./backup.md)」の「保存先を用意する」と、cron の設定）。

## 次は

- 管理画面を使う人に、アドレスなどを伝えます（「[使う人に渡すもの](./handover.md)」）
- 管理画面の使い方は「[使い方](../guide/)」にあります。サイトの名前・ロゴ・メニューなどは「[サイトの設定](../guide/settings.md)」で決めます
- WordPress から移るときは「[WordPress からの移行](./wordpress.md)」
