package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kom3da/pagewright/internal/backup"
	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/jobs"
)

// archiveRe matches the archives runBackup writes, oldest first by name.
var archiveRe = regexp.MustCompile(`^pagewright-\d{8}T\d{6}Z\.tar\.gz$`)

// runBackup writes one archive of the database and the media to --out.
// It runs as the table owner, beside the running server.
func runBackup(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	out := fs.String("out", "backups", "バックアップを保存するディレクトリ")
	keep := fs.Int("keep", 0, "新しいものからこの数だけ残し、古いバックアップを消す（0 で消さない）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *keep < 0 {
		return errors.New("使い方: pagewright backup [--out ディレクトリ] [--keep 残す数]")
	}
	if err := writeBackup(ctx, cfg, *out); err != nil {
		return err
	}
	if *keep > 0 {
		return pruneBackups(*out, *keep)
	}
	return nil
}

// writeBackup writes one archive to out and prints its name.
func writeBackup(ctx context.Context, cfg config.Config, out string) error {
	if err := os.MkdirAll(out, 0o700); err != nil {
		return fmt.Errorf("%s を作れません（書き込める場所か確認してください）: %w", out, err)
	}
	removeStaleTemp(out, time.Now().Add(-staleTempAge))

	name := "pagewright-" + time.Now().UTC().Format("20060102T150405Z") + ".tar.gz"
	final := filepath.Join(out, name)
	// Personal data (inquiries): readable by the owner only.
	f, err := os.CreateTemp(out, ".pagewright-*.partial")
	if err != nil {
		return fmt.Errorf("%s に書き込めません（Docker では、ホストのディレクトリの持ち主をユーザーID 10001 にしてください）: %w", out, err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // after a failure; gone once renamed
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	m, err := backup.Write(ctx, backup.Options{
		DatabaseURL: cfg.MigrateDatabaseURL, MediaDir: cfg.MediaDir(), TempDir: out, Version: version,
	}, f)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("%s はもうあります", final)
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	var rows int64
	for _, t := range m.Tables {
		rows += t.Rows
	}
	fmt.Printf("バックアップを作りました: %s（データ %d 行、ファイル %d 個）\n", final, rows, len(m.Files)-len(m.Tables))
	return nil
}

// staleTempAge is how long a backup's temporary file goes unwritten before
// it counts as left behind by a crash (a running backup writes all along).
const staleTempAge = 12 * time.Hour

// tempRe matches the temporary files of writeBackup and backup.Write.
var tempRe = regexp.MustCompile(`^\.pagewright-(table-)?[0-9]+(\.partial)?$`)

// removeStaleTemp removes temporary files in dir last written before
// cutoff: a killed backup (OOM, reboot) cannot remove its own.
func removeStaleTemp(dir string, cutoff time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || !tempRe.MatchString(e.Name()) {
			continue
		}
		if info, err := e.Info(); err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			fmt.Printf("途中で止まったバックアップの一時ファイルを消しました: %s\n", e.Name())
		}
	}
}

// pruneBackups removes all but the newest keep archives in dir.
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && archiveRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return err
		}
		fmt.Printf("古いバックアップを消しました: %s\n", names[0])
		names = names[1:]
	}
	return nil
}

// runRestore restores an archive, migrates up and queues what River must
// run again: a build of every site and the scheduled publishing.
func runRestore(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	force := fs.Bool("force", false, "データベースと画像に今あるものを消して上書きする（先に今の状態を --out にバックアップする）")
	out := fs.String("out", "backups", "--force のとき、上書きする前の状態のバックアップを保存するディレクトリ")
	noBackup := fs.Bool("no-backup", false, "--force のとき、上書きする前のバックアップを取らない")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("使い方: pagewright restore [--force [--out ディレクトリ] [--no-backup]] バックアップのファイル")
	}
	opts := backup.RestoreOptions{DatabaseURL: cfg.MigrateDatabaseURL, MediaDir: cfg.MediaDir(), Force: *force}
	if !*noBackup {
		opts.BeforeOverwrite = func(ctx context.Context) error {
			fmt.Println("上書きする前に、今の状態をバックアップします。")
			if err := writeBackup(ctx, cfg, *out); err != nil {
				return fmt.Errorf("上書きする前のバックアップを取れませんでした（取らずに進めるなら --no-backup）: %w", err)
			}
			return nil
		}
	}
	m, err := backup.Restore(ctx, opts, fs.Arg(0))
	if errors.Is(err, backup.ErrNotEmpty) {
		return fmt.Errorf("%w。消して上書きするなら --force を付けてください", err)
	}
	if err != nil {
		return err
	}
	if err := runMigrate(ctx, cfg); err != nil {
		return err
	}
	n, err := queueAfterRestore(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Printf("%s に取ったバックアップ（Pagewright %s）を復元しました。サイト %d 個を作り直します（pagewright serve が動いていれば、すぐに始まります）。\n",
		m.CreatedAt.Local().Format("2006-01-02 15:04"), m.PagewrightVersion, n)
	fmt.Println("ログインの状態は復元しません。全員ログインし直してください（メールが届かないときは pagewright login-url）。")
	return nil
}

// queueAfterRestore inserts the jobs the archive does not carry: a build
// and the revision pruning of every site, and the publishing of every
// scheduled content. It returns the number of sites.
func queueAfterRestore(ctx context.Context, cfg config.Config) (int, error) {
	pool, err := pgxpool.New(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: riverSchema}) // insert-only
	if err != nil {
		return 0, fmt.Errorf("river client: %w", err)
	}
	rows, err := pool.Query(ctx, "SELECT id FROM tenants ORDER BY id")
	if err != nil {
		return 0, err
	}
	tenants, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	var params []river.InsertManyParams
	for _, t := range tenants {
		params = append(params,
			river.InsertManyParams{Args: jobs.BuildSiteArgs{TenantID: t}},
			river.InsertManyParams{Args: jobs.NextPruneRevisions(t, time.Now())})
	}
	rows, err = pool.Query(ctx, `SELECT tenant_id, id, scheduled_revision_id, scheduled_at
		FROM contents WHERE scheduled_revision_id IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	scheduled, err := pgx.CollectRows(rows, pgx.RowToStructByPos[jobs.PublishScheduledArgs])
	if err != nil {
		return 0, err
	}
	for _, a := range scheduled {
		params = append(params, river.InsertManyParams{Args: a})
	}
	if len(params) == 0 {
		return 0, nil
	}
	if _, err := rc.InsertMany(ctx, params); err != nil {
		return 0, err
	}
	return len(tenants), nil
}
