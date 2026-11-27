// Command pagewright は Pagewright 本体のエントリーポイント。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kom3da/pagewright/internal/config"
)

const usage = `使い方: pagewright <サブコマンド>

  serve                          管理画面・APIと公開サイトのプレビューを起動する
  migrate                        データベースを最新の形にする
  init --site 名前 --email メール  サイトと管理者を作り、ログイン用URLを表示する
  login-url --email メール        ログイン用URLを発行する（メールが届かないとき）
  site set-url サイトのID URL      サイトの公開URLを設定する（2つ目以降のサイト用）
  site clear-url サイトのID        サイトの公開URLを外す（PAGEWRIGHT_PUBLIC_BASE_URL を使う）
  backup [--out ディレクトリ] [--keep 残す数]
                                 データベースと画像を1つのファイルにバックアップする
  restore [--force [--out ディレクトリ] [--no-backup]] ファイル
                                 バックアップから復元する（空のデータベースへ。--force は
                                 今の状態を --out にバックアップしてから上書きする）
  version                        版とコミットを表示する
  import-wxr --site ID --author-email メール [--dry-run] ファイル
                                 WordPressの書き出しファイル（WXR）を取り込む

設定は環境変数 PAGEWRIGHT_* で行う（internal/config を参照）。
`

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "version", "--version", "-v":
		printVersion()
		return
	}
	cfg := config.FromEnv()
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(ctx, cfg)
	case "migrate":
		err = runMigrate(ctx, cfg)
	case "init":
		err = runInit(ctx, cfg, args)
	case "login-url":
		err = runLoginURL(ctx, cfg, args)
	case "site":
		err = runSite(ctx, cfg, args)
	case "backup":
		err = runBackup(ctx, cfg, args)
	case "restore":
		err = runRestore(ctx, cfg, args)
	case "import-wxr":
		err = runImportWXR(ctx, args)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		slog.Error("失敗しました", "command", os.Args[1], "err", err)
		os.Exit(1)
	}
}
