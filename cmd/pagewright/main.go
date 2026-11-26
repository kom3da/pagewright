// Command pagewright は Pagewright 本体のエントリーポイント。
package main

import (
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		slog.Error("サブコマンドを指定してください", "usage", "pagewright serve")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		slog.Info("Pagewright を起動します（まだ中身はありません）")
	default:
		slog.Error("不明なサブコマンドです", "command", os.Args[1])
		os.Exit(2)
	}
}
