package main

import (
	"context"
	"strings"
	"testing"

	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

func TestSiteArgs(t *testing.T) {
	// Checked before connecting: the database here does not exist.
	cfg := config.Config{MigrateDatabaseURL: "postgres://nobody@127.0.0.1:1/none"}
	const id = "0198f3c2-0000-7000-8000-000000000000"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"none", nil, "使い方"},
		{"unknown", []string{"remove"}, "使い方"},
		{"no url", []string{"set-url", id}, "使い方"},
		{"extra", []string{"clear-url", id, "https://example.jp"}, "使い方"},
		{"bad id", []string{"set-url", "abc", "https://example.jp"}, "サイトのID"},
		{"bad id clear", []string{"clear-url", "abc"}, "サイトのID"},
		{"no scheme", []string{"set-url", id, "www.example.jp"}, "https://"},
		{"path", []string{"set-url", id, "https://example.jp/blog"}, "パス"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runSite(context.Background(), cfg, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestSiteSetURL(t *testing.T) {
	env := dbtest.New(t)
	ctx := context.Background()
	cfg := config.Config{MigrateDatabaseURL: env.URL, PublicBaseURL: "https://fallback.example"}
	if err := runMigrate(ctx, cfg); err != nil { // River's tables
		t.Fatal(err)
	}
	a, b := env.CreateTenant(t, "サイトA"), env.CreateTenant(t, "サイトB")
	urlOf := func() map[string]*string {
		t.Helper()
		rows, err := env.Admin.Query(ctx, "SELECT id::text, public_base_url FROM tenants")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]*string{}
		for rows.Next() {
			var id string
			var u *string
			if err := rows.Scan(&id, &u); err != nil {
				t.Fatal(err)
			}
			out[id] = u
		}
		return out
	}
	builds := func() int {
		t.Helper()
		var n int
		if err := env.Admin.QueryRow(ctx, "SELECT count(*) FROM river.river_job WHERE kind = 'build_site' AND args->>'tenant_id' = $1", b.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if err := runSite(ctx, cfg, []string{"set-url", b.String(), " HTTPS://Site-B.Example/ "}); err != nil {
		t.Fatal(err)
	}
	got := urlOf()
	if got[b.String()] == nil || *got[b.String()] != "https://site-b.example" || got[a.String()] != nil {
		t.Errorf("after set-url: A = %v, B = %v", got[a.String()], got[b.String()])
	}
	if n := builds(); n != 1 {
		t.Errorf("build jobs for B = %d, want 1", n)
	}

	if err := runSite(ctx, cfg, []string{"set-url", "0198f3c2-0000-7000-8000-000000000000", "https://x.example"}); err == nil || !strings.Contains(err.Error(), "ありません") {
		t.Errorf("unknown site: err = %v", err)
	}

	if err := runSite(ctx, cfg, []string{"clear-url", b.String()}); err != nil {
		t.Fatal(err)
	}
	if got := urlOf(); got[b.String()] != nil {
		t.Errorf("after clear-url: B = %v, want NULL", *got[b.String()])
	}

	// The database refuses what the CLI would not store.
	for _, bad := range []string{"https://site-b.example/", "https://Site-B.example", "https://site-b.example/blog", "site-b.example", ""} {
		if _, err := env.Admin.Exec(ctx, "UPDATE tenants SET public_base_url = $1 WHERE id = $2", bad, b); err == nil {
			t.Errorf("CHECK accepted %q", bad)
		}
	}
}
