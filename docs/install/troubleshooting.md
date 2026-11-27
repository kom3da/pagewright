# 困ったとき

起きていることから探してください。コマンドは `/opt/pagewright/deploy` で実行します。

## まず状態とログを見る

```sh
docker compose ps -a
docker compose logs -f app      # 本体（Ctrl＋C で抜ける）
docker compose logs cli         # 起動のときのデータベースの更新（マイグレーション）
docker compose logs nginx caddy # 配信と HTTPS
curl -s https://<管理画面のドメイン>/healthz   # ok と出れば本体とDBは動いている
```

::: danger `docker compose down -v` は使わない
`-v` を付けると、データベース・画像・HTTPS の証明書のボリュームも消えます。止めるだけなら `docker compose stop`、作り直すなら `docker compose up -d` です。
:::

## 起動しない

### `cli` が `Exited (1)` になり、`app` が起動しない

`cli` は起動のたびにデータベースを最新の形にします（マイグレーション）。それに失敗すると、`app` は起動しません。
`docker compose logs cli` の最後の行で理由を確かめます。

- **パスワードのエラー**（`password authentication failed`）：最初に起動したあとで `.env` の `POSTGRES_PASSWORD` を変えると、こうなります。データベースのパスワードは、最初の起動のときに決まります。`.env` を元の値に戻すか、「[画像の容量・DBパスワード](./settings.md#アプリ用のdbパスワードを変える)」の手順で変えます
- **データベースにつながらない**：`docker compose ps -a` で `db` が `healthy` か確かめます。`docker compose logs db` に理由が出ます
- **アップデートのあとに失敗した**：理由を控えて、[Issue](https://github.com/kom3da/pagewright/issues) で知らせてください。前の版に戻すときは「[アップデート](./update.md#前の版に戻す)」

### 「.env に ○○ を書いてください」と出る

`.env` のその項目が空です。「[導入](./setup.md#_2-設定ファイル-env-を作る)」の表のとおりに書きます。

### ネットワークを作れずに `up` が止まる

サーバーで IPv6 を切っていると、`docker compose up` がコンテナ用のネットワークを作れずに失敗します（エラーに `network` や `IPv6` が出ます）。
IPv4 だけで公開するように `compose.override.yaml` を書きます（「[HTTPS と構成の変更](./https.md#サーバーで-ipv6-を切っているとき)」）。
IPv6 のアドレスがほかのネットワークと重なっているときも、同じページの説明で変えられます。

### `permission denied`（`docker.sock`）と出る

作業しているユーザーが `docker` グループに入っていません。

```sh
sudo usermod -aG docker "$USER"
```

実行したら、**一度ログアウトしてログインし直します**。それまでは `sudo docker …` で実行します。

## HTTPS でつながらない

- DNS がサーバーのIPアドレスを指しているか（`dig +short <ドメイン>`）。反映には時間がかかることがあります
- ポート 80 と 443 が、サーバー会社の管理画面のファイアウォールと、サーバーの中のファイアウォールの両方で開いているか（「[はじめに](./index.md#ポートを開ける)」）
- `docker compose logs caddy` に証明書のエラーが出ていないか

Caddy は、証明書が取れなくても、しばらくすると自動でやり直します。原因を直したら、少し待ってから開き直してください。

Let's Encrypt には、失敗や取り直しをくり返すと、しばらく証明書を発行してくれなくなる回数制限があります。
ログに `rateLimited` や `too many` が出ていたら、原因を直したうえで、時間をおいてください（どれだけ待てばよいかは、エラーの文に書いてあります）。
証明書は `caddy-data` のボリュームに残ります。`docker compose down -v` で消すと取り直しになるので、使わないでください。

## ログイン用のメールが届かない

次のコマンドでログイン用URLを出せます（1時間有効、1回限り）。

```sh
docker compose run --rm cli login-url --email you@example.co.jp
```

送信の失敗はログで確認できます。

```sh
docker compose logs app | grep -i mail
```

- `connect to smtp`：`PAGEWRIGHT_SMTP_ADDR` のホスト名かポートがまちがっているか、サーバー会社がそのポートをふさいでいます。587 番を使います
- `smtp auth`：ユーザー名かパスワードがまちがっています。記号が入るパスワードは `'` で囲みます（「[導入](./setup.md#_2-設定ファイル-env-を作る)」）
- `does not offer STARTTLS`：サーバーが暗号化に対応していません。465 番のサーバーなら `PAGEWRIGHT_SMTP_TLS=implicit` にします（「[メール送信サービスを用意する](./mail.md)」）

`.env` を直したら `docker compose up -d` を実行します。

## メールは送れたのに届かない

ログにエラーがないのに届かないときは、メール送信サービスから先で止まっています。

- 迷惑メールのフォルダを確かめる
- メール送信サービスの画面の送信の記録（ログ）で、相手のサーバーに断られていないか確かめる
- 差出人のドメインの SPF・DKIM を登録したか（「[メール送信サービスを用意する](./mail.md)」）
- サービスが「確認済みのアドレスにしか送れない」状態（Amazon SES のサンドボックスなど）のままになっていないか

## 公開サイトが 404 になる

- `.env` の `PAGEWRIGHT_SITE_ID` に `init` で表示されたIDを書き、`docker compose up -d` を実行したか
- 管理画面で記事を公開したか、「サイトを更新」を押したか（`docker compose logs app | grep "site built"` で書き出しの記録を確認できる）

## 公開しても公開サイトが更新されない（ログに pagefind のエラー）

サイト内検索の索引を作るプログラム（Pagefind）が動かないと、書き出し全体が失敗します。
`docker compose logs app | grep pagefind` に `Illegal instruction` などが出ている場合は、
サーバーのCPUに対応していない可能性があります。`.env` に `PAGEWRIGHT_PAGEFIND=off` を書いて
`docker compose up -d` を実行すると、検索なしで書き出せます。

## ディスクがいっぱいになった

```sh
df -h                  # ディスクの空き
docker system df       # Docker のイメージ・ボリュームが使っている量
sudo du -sh backups    # バックアップが使っている量
```

- **バックアップ**：`backup` に `--keep 14` のように付けると、古いものから消えます（「[バックアップと復元](./backup.md)」）
- **古い版のイメージ**：アップデートのあとに残った、使っていないイメージは `docker image prune -a` で消せます（あとで必要になっても、取得し直せます）
- **画像**：サイトごとに上限を決められます（「[画像の容量・DBパスワード](./settings.md)」）
- コンテナのログは、1つのサービスにつき 10MB × 5 個までしか残らない設定になっています

空きができたら `docker compose up -d` を実行し、`docker compose ps -a` で状態を確かめます。

## サーバーを再起動したあと

`compose.yaml` のサービスは `restart: unless-stopped` なので、サーバーを再起動すると Docker が自動で起動し直します（`cli` は起動の最初にだけ動くものなので、`Exited (0)` のままでかまいません）。
`docker compose stop` で止めていたものは、止まったままです。

起動していないときは、Docker 自体がサーバーの起動のときに立ち上がる設定になっているか確かめます。

```sh
sudo systemctl enable --now docker
docker compose up -d
```
