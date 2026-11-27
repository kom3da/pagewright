# HTTPS について

標準の構成では Caddy が Let's Encrypt の証明書を自動で取得・更新します。設定は不要です。

クラウドのロードバランサー（AWS の ALB など）で HTTPS を受ける場合は、`compose.yaml` の `caddy` サービスを消し、
`nginx` に `ports: ["80:8080"]` を足して、ロードバランサーからサーバーの 80 番へ転送してください。
サーバーの 80 番には、ロードバランサー以外から届かないようにしておきます。
ロードバランサーは `X-Forwarded-For` に閲覧者のIPを付け足すもの（ALB などの HTTP のロードバランサー）にしてください。

## Caddy もロードバランサーも置かずに nginx を直接公開する場合

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
