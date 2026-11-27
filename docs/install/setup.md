# 導入

## 1. ファイルを取得する

```sh
sudo git clone --branch v0.1.0 https://github.com/kom3da/pagewright.git /opt/pagewright
sudo chown -R "$USER" /opt/pagewright   # 以降は sudo なしで .env を作ったり git を使ったりできる
cd /opt/pagewright/deploy
```

`v0.1.0` は [リリース一覧](https://github.com/kom3da/pagewright/releases) にある最新の版に置き換えてください。以降のコマンドはすべて `deploy/` で実行します。

## 2. 設定ファイル（.env）を作る

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
| `PAGEWRIGHT_IMAGE` | 使う版。手順1で取得した版（`ghcr.io/kom3da/pagewright:0.1.0` など）が入っているので、そのままにする |

`PAGEWRIGHT_SITE_ID` は手順5で書くので、いまは空のままにします。
パスワードは英数字だけにしてください（記号が入ると接続できないことがあります）。

## 3. DNS を設定する

2つのドメインの A レコード（IPv6 を使うなら AAAA レコードも）を、サーバーのIPアドレスに向けます。
DNS が反映される前に起動すると証明書の取得に失敗します。その場合も、反映後しばらくすると自動でやり直されます。

## 4. 起動する

```sh
docker compose up -d
docker compose ps -a
```

起動のたびに、まず `cli` がデータベースを最新の形にして（マイグレーション）終了し、そのあと `app` が起動します。
`cli` が `Exited (0)`、ほかが `Up`（`app` と `db` は `healthy`）になっていれば成功です（`-a` を付けないと、終わった `cli` は表示されません）。

## 5. サイトと管理者を作る

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

## 6. 公開する

管理画面で記事や固定ページを作って公開すると、公開サイトが書き出されます。
最初に一度、管理画面の「サイトを更新」を押しておくと、トップページなどが作られます。
`https://<公開サイトのドメイン>/` を開いて表示を確かめてください。

2回目からは、管理画面のログイン画面でメールアドレスを入れると、ログイン用リンクがメールで届きます。
