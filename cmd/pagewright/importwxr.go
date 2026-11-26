package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/jobs"
	"github.com/kom3da/pagewright/internal/migrate/wxrimport"
)


// runImportWXR implements `pagewright import-wxr [flags] <file.xml>`: it
// imports a WordPress export into one site and prints the report.
func runImportWXR(ctx context.Context, args []string) error {
	return importWXR(ctx, args, os.Stdout, os.Stderr)
}

func importWXR(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("import-wxr", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "使い方: pagewright import-wxr --site <サイトID> --author-email <メール> [オプション] <WordPressの書き出しファイル.xml>")
		fs.PrintDefaults()
	}
	defaultURL := os.Getenv("PAGEWRIGHT_DATABASE_URL")
	if defaultURL == "" {
		defaultURL = config.DevDatabaseURL
	}
	dbURL := fs.String("database-url", defaultURL, "PostgreSQL の接続先（環境変数 PAGEWRIGHT_DATABASE_URL）")
	site := fs.String("site", "", "取り込み先のサイト（テナント）の ID（UUID）")
	authorEmail := fs.String("author-email", "", "作成者にするユーザーのメールアドレス（サイトのメンバー）")
	dryRun := fs.Bool("dry-run", false, "取り込まずに、変換できないものの一覧だけを表示する")
	pwURLs := fs.Bool("pagewright-urls", false, "旧URLを使わず Pagewright のURL（/posts/…/）にし、旧URLからリダイレクトする")
	download := fs.Bool("download-media", false, "画像をダウンロードして本文の画像の場所を書き換える")
	mediaRoot := fs.String("media-root", config.FromEnv().MediaDir(), "画像の保存先（既定は PAGEWRIGHT_DATA_DIR/media）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("WordPress の書き出しファイルを1つ指定してください")
	}
	tenantID, err := uuid.Parse(*site)
	if err != nil {
		return fmt.Errorf("--site にはサイトの ID（UUID）を指定してください: %w", err)
	}
	email := strings.TrimSpace(*authorEmail)
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("--author-email に作成者のメールアドレスを指定してください")
	}

	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("ファイルを開けません: %w", err)
	}
	defer func() { _ = f.Close() }()

	d, err := db.Open(ctx, *dbURL)
	if err != nil {
		return err
	}
	defer d.Close()
	user, err := d.FindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ユーザー %s が見つかりません", email)
	}
	if err != nil {
		return err
	}

	rep, err := wxrimport.Import(ctx, d, tenantID, f, wxrimport.Options{
		AuthorID:      user.ID,
		DryRun:        *dryRun,
		KeepURLs:      !*pwURLs,
		DownloadMedia: *download,
		MediaRoot:     *mediaRoot,
	})
	if werr := rep.WriteText(stdout); werr != nil && err == nil {
		err = werr
	}
	if err != nil || *dryRun {
		return err
	}
	return scheduleImported(ctx, d, tenantID)
}

// scheduleImported adds publish jobs for scheduled posts that came in with
// the import, and a build so the imported site appears. It only inserts
// jobs; the running server works them.
func scheduleImported(ctx context.Context, d *db.DB, tenantID uuid.UUID) error {
	client, err := river.NewClient(riverpgxv5.New(d.Pool()), &river.Config{Schema: riverSchema})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}
	n, err := jobs.ScheduleExisting(ctx, d, client, tenantID)
	if err != nil {
		return fmt.Errorf("schedule imported posts: %w", err)
	}
	if _, err := client.Insert(ctx, jobs.BuildSiteArgs{TenantID: tenantID}, nil); err != nil {
		return fmt.Errorf("schedule build: %w", err)
	}
	slog.Info("予約公開とサイトのビルドを登録しました", "scheduled", n)
	return nil
}
