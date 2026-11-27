# HTTPS と構成の変更

::: tip 標準の構成なら、このページは読まなくてかまいません
「[導入](./setup.md)」のとおりに入れたなら、Caddy が Let's Encrypt の証明書を自動で取得・更新します。設定はいりません。
:::

## 証明書が取れているか確かめる

```sh
curl https://<管理画面のドメイン>/healthz          # ok と出れば、証明書が取れていて本体も動いている
docker compose logs caddy | grep -i certificate   # certificate obtained successfully が出ていればよい
```

ブラウザで公開サイトを開き、アドレス欄の鍵のマークからも、証明書の発行元（Let's Encrypt）と期限を確かめられます。
証明書は、期限が近づくと Caddy が自動で更新します。
取れないときは「[困ったとき](./troubleshooting.md#https-でつながらない)」を見てください。

ここから下は、標準とちがう構成にするときの説明です。

## 構成を変えるときは compose.override.yaml に書く

`compose.yaml` や `nginx/templates/default.conf.template` を直接書き換えると、アップデートで `git checkout` するときにぶつかります（「[アップデート](./update.md#手元で直したファイルがあるとき)」）。
構成を変えるときは、`deploy/` に **`compose.override.yaml`** というファイルを作り、変えたいところだけを書きます。
Docker Compose は、`compose.yaml` と同じ場所にこのファイルがあると、自動で重ねて読みます。このファイルは Pagewright のリポジトリに入っていないので、`git checkout` で消えたりぶつかったりしません。

重ねた結果は `docker compose config` で確かめられます。

::: warning 一覧は置き換わる
`compose.override.yaml` に書いた一覧（`- subnet: …` のような `-` で始まる並び）は、`compose.yaml` の同じ一覧に足されるのではなく、置き換わります。残したいものも、もう一度書いてください。
:::

## クラウドのロードバランサーで HTTPS を受ける

AWS の ALB などで HTTPS を受けるときは、Caddy を使わずに、ロードバランサーからサーバーの 80 番へ転送します。

```yaml
# compose.override.yaml
services:
  caddy:
    profiles: [disabled] # Caddy を起動しない
  nginx:
    ports: ["80:8080"]
```

- すでに Caddy が動いているときは、先に `docker compose stop caddy` で止めます
- サーバーの 80 番には、ロードバランサー以外から届かないようにしておきます
- ロードバランサーは `X-Forwarded-For` に閲覧者のIPを付け足すもの（ALB などの HTTP のロードバランサー）にしてください

## IPv6 の閲覧者

`compose.yaml` は、コンテナのネットワークで IPv6 も使うようにしています（`networks:` の `enable_ipv6: true` と、コンテナ用の IPv6 のアドレス `fd70:6167:6577::/64`）。
こうしないと、IPv6 で届いた閲覧者は Docker の中継（docker-proxy）を通って Docker のゲートウェイから来たように見え、
ログイン用リンクの送信（15分に30回）やお問い合わせ（10分に10回）の回数制限が、IPv6 の閲覧者全員でまとめてかかってしまいます。

- **Docker Engine 27 以降**を使ってください（`docker version` の Server の Version）。27 から、IPv6 の転送（ip6tables）が標準で有効になり、閲覧者の IPv6 アドレスがそのまま Caddy に届きます。
  26 以前では `/etc/docker/daemon.json` に `{"experimental": true, "ip6tables": true}` を書いて Docker を再起動する必要があります
- 閲覧者のアドレスが見えていないときは、`app` のログに「回数制限のかかった要求の多くが、プライベートなアドレスから届いています」という警告が出ます

### アドレスがほかのネットワークと重なるとき

`fd70:6167:6577::/64` がサーバーのほかのネットワークと重なるときは、別の `fd` で始まるアドレスにします。

```yaml
# compose.override.yaml
networks:
  default:
    ipam:
      config:
        - subnet: fd70:6167:6578::/64
```

### サーバーで IPv6 を切っているとき

サーバーで IPv6 を切っている（カーネルで無効にしている）と、`docker compose up` がネットワークを作れずに失敗します。
その場合は、IPv4 だけで公開します。次のように書き、DNS に AAAA レコードを置かないでください。

```yaml
# compose.override.yaml
networks:
  default:
    enable_ipv6: false
    ipam: !reset {}
```

（`!reset` は、`compose.yaml` の設定を消す書き方です。Docker Compose v2.24 以降で使えます。）

## Caddy もロードバランサーも置かずに nginx を直接公開する

標準の設定は「手前に Caddy かロードバランサーがいる」前提で、プライベートIP（`10.0.0.0/8`・`172.16.0.0/12`・`192.168.0.0/16`・`fc00::/7`）から
届いた `X-Forwarded-For` を信じます（app の `PAGEWRIGHT_TRUSTED_PROXIES` と、nginx の `set_real_ip_from`）。
nginx を直接インターネットに公開すると、閲覧者の接続が Docker のゲートウェイ（プライベートIP）から来たように見えることがあり、
閲覧者が `X-Forwarded-For` を偽ってお問い合わせフォームやログインの回数制限をすり抜けられてしまいます。

この構成では、信じる相手を nginx だけにしてください。

1. `compose.override.yaml` で nginx のIPを固定し、app の `PAGEWRIGHT_TRUSTED_PROXIES` をそのIPだけにする

   ```yaml
   # compose.override.yaml
   services:
     caddy:
       profiles: [disabled]
     app:
       environment:
         PAGEWRIGHT_TRUSTED_PROXIES: "172.30.0.10" # nginx だけ
     nginx:
       ports: ["80:8080"]
       networks:
         default:
           ipv4_address: 172.30.0.10
   networks:
     default:
       ipam:
         config: # 一覧は置き換わるので、IPv6 の分も書く
           - subnet: fd70:6167:6577::/64
           - subnet: 172.30.0.0/24
   ```

2. `nginx/templates/default.conf.template` の `set_real_ip_from` の4行と `real_ip_header` の行を消す。
   このファイルは直接書き換えるしかないので、アップデートのときは「[手元で直したファイルがあるとき](./update.md#手元で直したファイルがあるとき)」の手順で、直した内容を引き継ぎます

こうすると、Docker のゲートウェイを通る接続では閲覧者ごとの区別ができず、回数制限がまとめてかかることがあります。
閲覧者ごとに制限したいときは、標準の Caddy を使ってください。
