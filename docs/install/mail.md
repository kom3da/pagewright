# メールの設定

Pagewright はログイン用リンクとお問い合わせの通知をメールで送ります。`.env` に次を書きます。

| 設定 | 内容 |
| --- | --- |
| `PAGEWRIGHT_SMTP_ADDR` | SMTP サーバー（例：`smtp.example.co.jp:587`） |
| `PAGEWRIGHT_SMTP_USERNAME`・`PAGEWRIGHT_SMTP_PASSWORD` | 認証が必要なサービスのとき。パスワードは TLS の上でしか送りません（サーバーが STARTTLS に対応しているか、次の implicit） |
| `PAGEWRIGHT_SMTP_TLS` | `.env.example` では `starttls`。STARTTLS に対応していないサーバーには送りません（暗号化なしでメールを送らないため。外部のメール送信サービスではこのままにします）。465 番ポートのように最初から TLS で話すサーバーなら `implicit`。同じマシンの中継サーバーのように暗号化しないサーバーへ送るときだけ空にします（サーバーが対応していれば STARTTLS を使い、対応していなければ暗号化せずに送ります） |

メール送信サービス（SendGrid、Amazon SES など）の SMTP、Google Workspace の SMTP リレー、社内の中継サーバーのどれでも使えます。

SPF・DKIM の設定がないと、迷惑メールに入りやすくなります。差出人のドメインの DNS も確認してください。

メールが届かなくても、コマンドでログイン用URLを出せます（「[困ったとき](./troubleshooting.md#ログイン用のメールが届かない)」）。
