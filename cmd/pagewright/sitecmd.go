package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/jobs"
	"github.com/kom3da/pagewright/internal/site"
)

const siteUsage = `使い方:
  pagewright site list                     サイトのID・名前・公開サイトのURLを一覧にする
  pagewright site set-url サイトのID URL   公開サイトのURL（例: https://www.example.jp）を設定する
  pagewright site clear-url サイトのID     公開サイトのURLを外し、PAGEWRIGHT_PUBLIC_BASE_URL に戻す`

// runSite runs the operator's site commands. They run as the table owner:
// the app role may read its own site's public_base_url but has no grant
// to change it, since the domain must match the web server's
// configuration, which the admin cannot see.
func runSite(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return errors.New(siteUsage)
	}
	switch {
	case args[0] == "list" && len(args) == 1:
		return listSites(ctx, cfg, os.Stdout)
	case args[0] == "set-url" && len(args) == 3:
		u, err := site.ParsePublicURL(args[2])
		if err != nil {
			return err
		}
		return setSiteURL(ctx, cfg, args[1], &u)
	case args[0] == "clear-url" && len(args) == 2:
		return setSiteURL(ctx, cfg, args[1], nil)
	}
	return errors.New(siteUsage)
}

// listSites prints every site's ID, name and public URL, oldest first.
func listSites(ctx context.Context, cfg config.Config, w io.Writer) error {
	conn, err := pgx.Connect(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	rows, err := conn.Query(ctx, "SELECT id::text, name, public_base_url FROM tenants ORDER BY created_at, id")
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, name string
		var u *string
		if err := rows.Scan(&id, &name, &u); err != nil {
			return err
		}
		// The cli container has no PAGEWRIGHT_PUBLIC_BASE_URL, so the
		// default is named rather than shown.
		url := "（設定なし：PAGEWRIGHT_PUBLIC_BASE_URL）"
		if u != nil {
			url = *u
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", id, name, url); err != nil {
			return err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 0 {
		_, err = fmt.Fprintln(w, "サイトはまだありません（pagewright init で作ります）。")
	}
	return err
}

// setSiteURL stores the site's public URL (nil: none) and queues a build,
// so that the pages' links change right away.
func setSiteURL(ctx context.Context, cfg config.Config, siteID string, u *string) error {
	tenant, err := uuid.Parse(siteID)
	if err != nil {
		return errors.New("サイトのIDが正しくありません（pagewright init が表示したIDを指定してください）")
	}
	pool, err := pgxpool.New(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	// Insert-only: the server's River client builds the site.
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: riverSchema})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	var name string
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "UPDATE tenants SET public_base_url = $2 WHERE id = $1 RETURNING name", tenant, u).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ID %s のサイトはありません", tenant)
		}
		if err != nil {
			return err
		}
		_, err = rc.InsertTx(ctx, tx, jobs.BuildSiteArgs{TenantID: tenant}, nil)
		return err
	})
	if err != nil {
		return err
	}
	if u == nil {
		fmt.Printf("サイト「%s」の公開サイトのURLを外しました。PAGEWRIGHT_PUBLIC_BASE_URL のURLを使います。\n", name)
	} else {
		fmt.Printf("サイト「%s」の公開サイトのURLを %s にしました。\n", name, *u)
	}
	fmt.Println("公開サイトを作り直します（pagewright serve が動いていれば、すぐに始まります）。")
	return nil
}
