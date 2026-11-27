# HTTPS について

標準の構成では Caddy が Let's Encrypt の証明書を自動で取得・更新します。設定は不要です。

クラウドのロードバランサー（AWS の ALB など）で HTTPS を受ける場合は、`compose.yaml` の `caddy` サービスを消し、
`nginx` に `ports: ["80:8080"]` を足して、ロードバランサーからサーバーの 80 番へ転送してください。
サーバーの 80 番には、ロードバランサー以外から届かないようにしておきます。
ロードバランサーは `X-Forwarded-For` に閲覧者のIPを付け足すもの（ALB などの HTTP のロードバランサー）にしてください。

## IPv6 の閲覧者

`compose.yaml` は、コンテナのネットワークで IPv6 も使うようにしています（`networks:` の `enable_ipv6: true` と、コンテナ用の IPv6 のアドレス `fd70:6167:6577::/64`）。
こうしないと、IPv6 で届いた閲覧者は Docker の中継（docker-proxy）を通って Docker のゲートウェイから来たように見え、
ログイン用リンクの送信（15分に30回）やお問い合わせ（10分に10回）の回数制限が、IPv6 の閲覧者全員でまとめてかかってしまいます。

- **Docker Engine 27 以降**を使ってください（`docker version` の Server の Version）。27 から、IPv6 の転送（ip6tables）が標準で有効になり、閲覧者の IPv6 アドレスがそのまま Caddy に届きます。
  26 以前では `/etc/docker/daemon.json` に `{"experimental": true, "ip6tables": true}` を書いて Docker を再起動する必要があります
- `fd70:6167:6577::/64` がサーバーのほかのネットワークと重なるときは、`compose.yaml` の `subnet` を別の `fd` で始まるアドレスに変えます
- サーバーで IPv6 を切っている（カーネルで無効にしている）と、`docker compose up` がネットワークを作れずに失敗します。その場合は `compose.yaml` の `networks:` の部分を消し、DNS に AAAA レコードを置かないでください（IPv4 だけで公開します）
- 閲覧者のアドレスが見えていないときは、`app` のログに「回数制限のかかった要求の多くが、プライベートなアドレスから届いています」という警告が出ます

## Caddy もロードバランサーも置かずに nginx を直接公開する場合

標準の設定は「手前に Caddy かロードバランサーがいる」前提で、プライベートIP（`10.0.0.0/8`・`172.16.0.0/12`・`192.168.0.0/16`・`fc00::/7`）から
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
       enable_ipv6: true
       ipam:
         config:
           - subnet: fd70:6167:6577::/64
           - subnet: 172.30.0.0/24
   ```

2. `nginx/templates/default.conf.template` の `set_real_ip_from` の4行と `real_ip_header` の行を消す

こうすると、Docker のゲートウェイを通る接続では閲覧者ごとの区別ができず、回数制限がまとめてかかることがあります。
閲覧者ごとに制限したいときは、標準の Caddy を使ってください。
