package migrations_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
	"github.com/kom3da/pagewright/internal/db/migrations"
)

// Tenants and users in testdata/seed/00001.sql.
var (
	siteA = uuid.MustParse("01900000-0000-7000-8000-00000000000a")
	siteB = uuid.MustParse("01900000-0000-7000-8000-00000000000b")
	alice = uuid.MustParse("01900000-0000-7000-8000-0000000000a1")
)

// TestMigrationNumbers keeps versions 1, 2, 3, ... without gaps. Two
// branches that both add a migration then collide on the same number and
// one must be renumbered before merging, instead of a lower number landing
// after a higher one was deployed (which goose refuses to apply; see
// TestOutOfOrderRefused).
func TestMigrationNumbers(t *testing.T) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range files {
		want := fmt.Sprintf("%05d_", i+1)
		if !strings.HasPrefix(f, want) {
			t.Fatalf("migration %d is %s, want a name starting with %s (numbers go up by one)", i+1, f, want)
		}
	}
}

// TestUpgradeWithData moves a database through every migration one at a
// time with data in it, as an install that upgrades from any earlier
// release would. At each version it runs that migration's Down and Up
// again, then loads testdata/seed/<version>.sql if there is one (data
// written by the app as of that version). At the end the app reads and
// writes the data and builds a site from it.
//
// A migration that changes the shape of existing data should come with a
// seed file for the version before it, so that the change runs on data.
func TestUpgradeWithData(t *testing.T) {
	ctx := context.Background()
	e := dbtest.NewEmpty(t)

	seeds := map[int64]string{}
	files, err := filepath.Glob("testdata/seed/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		var v int64
		if _, err := fmt.Sscanf(filepath.Base(f), "%05d.sql", &v); err != nil {
			t.Fatalf("%s: name it <migration version>.sql: %v", f, err)
		}
		seeds[v] = f
	}

	for _, src := range e.Provider.ListSources() {
		v := src.Version
		if _, err := e.Provider.UpByOne(ctx); err != nil {
			t.Fatalf("up to %d: %v", v, err)
		}
		if _, err := e.Provider.Down(ctx); err != nil {
			t.Fatalf("down from %d: %v", v, err)
		}
		if _, err := e.Provider.UpByOne(ctx); err != nil {
			t.Fatalf("up to %d again after down: %v", v, err)
		}
		if f, ok := seeds[v]; ok {
			sql, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Admin.Exec(ctx, string(sql)); err != nil {
				t.Fatalf("seed %s at version %d: %v", f, v, err)
			}
			delete(seeds, v)
		}
	}
	for v, f := range seeds {
		t.Errorf("%s: no migration %d", f, v)
	}

	d, err := db.Open(ctx, e.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	t.Run("build", func(t *testing.T) { checkBuild(t, d) })
	t.Run("new ids", func(t *testing.T) { checkNewIDs(t, d) })
	t.Run("isolation", func(t *testing.T) { checkIsolation(t, d) })
	t.Run("auth", func(t *testing.T) { checkAuth(t, d) })
}

// checkBuild builds site A and checks the published pages are there.
func checkBuild(t *testing.T, d *db.DB) {
	out, media := t.TempDir(), t.TempDir()
	for _, key := range []string{"2025/04/office.jpg"} {
		p := filepath.Join(media, siteA.String(), key)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("fake jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := build.Build(context.Background(), d, siteA, build.Options{OutDir: out, MediaRoot: media})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range res.Warnings {
		t.Errorf("build warning: %s", w)
	}
	for _, p := range []string{
		"/index.html", "/company/index.html", "/company/access/index.html", "/沿革/index.html",
		"/posts/founding/index.html", "/posts/category/news/index.html", "/posts/tag/pr/index.html",
		"/media/2025/04/office.jpg",
	} {
		if !slices.Contains(res.Changed, p) {
			t.Errorf("build did not write %s (wrote %v)", p, res.Changed)
		}
	}
	for _, p := range []string{"/posts/renewal/index.html", "/posts/summer/index.html", "/menu/index.html"} {
		if slices.Contains(res.Changed, p) {
			t.Errorf("build wrote %s, which is not published on site A", p)
		}
	}
	page, err := os.ReadFile(filepath.Join(out, siteA.String(), "posts", "founding", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "創業しました。") || strings.Contains(string(page), "書きかけ") {
		t.Errorf("/posts/founding/ does not show the published revision:\n%s", page)
	}
}

// checkNewIDs creates contents after the migrated ones: ids continue per
// tenant after the highest existing id, numbers after the highest number.
func checkNewIDs(t *testing.T, d *db.DB) {
	ctx := context.Background()
	type ids struct{ ID, Number int64 }
	create := func(tenant uuid.UUID) ids {
		t.Helper()
		var c dbgen.Content
		err := d.InTenant(db.WithTenant(ctx, tenant), func(q *dbgen.Queries) (err error) {
			c, err = q.CreateContent(ctx, dbgen.CreateContentParams{Type: "post"})
			return err
		})
		if err != nil {
			t.Fatalf("create content in %s: %v", tenant, err)
		}
		return ids{c.ID, c.Number}
	}
	if diff := cmp.Diff(ids{9, 105}, create(siteA)); diff != "" {
		t.Errorf("new content in site A (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ids{6, 3}, create(siteB)); diff != "" {
		t.Errorf("new content in site B (-want +got):\n%s", diff)
	}
}

// checkIsolation reads site A's rows from site B.
func checkIsolation(t *testing.T, d *db.DB) {
	ctx := db.WithTenant(context.Background(), siteB)
	err := d.InTenant(ctx, func(q *dbgen.Queries) error {
		// Content 4 exists only in site A.
		if _, err := q.GetContent(ctx, 4); !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("site B got site A's content 4: err = %v", err)
		}
		routes, err := q.ListRoutes(ctx)
		if err != nil {
			return err
		}
		for _, r := range routes {
			if r.TenantID != siteB {
				return fmt.Errorf("site B sees route %s of %s", r.Path, r.TenantID)
			}
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

// checkAuth looks up migrated sessions and preview links as the app does.
func checkAuth(t *testing.T, d *db.DB) {
	ctx := context.Background()
	hash := func(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

	var user, tenant uuid.UUID
	err := d.Pool().QueryRow(ctx, "SELECT user_id, tenant_id FROM auth_session($1)", hash("alice-session")).Scan(&user, &tenant)
	if err != nil {
		t.Fatalf("alice's session: %v", err)
	}
	if user != alice || tenant != siteA {
		t.Errorf("alice's session = user %s, site %s", user, tenant)
	}
	err = d.Pool().QueryRow(ctx, "SELECT user_id FROM auth_session($1)", hash("old-session")).Scan(&user)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expired session: err = %v, want no rows", err)
	}

	var content int64
	err = d.Pool().QueryRow(ctx, "SELECT tenant_id, content_id FROM preview_link_lookup($1)", hash("preview-renewal")).Scan(&tenant, &content)
	if err != nil {
		t.Fatalf("preview link: %v", err)
	}
	if tenant != siteA || content != 6 {
		t.Errorf("preview link = site %s, content %d", tenant, content)
	}

	if _, err := d.FindUserByEmail(ctx, "Carol@example.jp"); err != nil {
		t.Errorf("find carol: %v", err)
	}
}

// TestOutOfOrderRefused applies every migration but one, then all: the
// skipped one has a lower number than what the database has, and goose
// must refuse to apply it rather than run it out of order.
func TestOutOfOrderRefused(t *testing.T) {
	ctx := context.Background()
	e := dbtest.NewEmpty(t)
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Skip("needs two migrations")
	}
	skipped := files[len(files)-2]
	partial := fstest.MapFS{}
	for _, f := range files {
		if f == skipped {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		partial[f] = &fstest.MapFile{Data: b}
	}
	p, err := migrations.NewProvider(e.SQL, partial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		// The skipped migration may be needed by the newest one; then
		// this setup cannot be made and there is nothing to check.
		t.Skipf("migrations without %s do not apply: %v", skipped, err)
	}
	before, err := e.Provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.Provider.Up(ctx)
	if err == nil || !strings.Contains(err.Error(), "out-of-order") {
		t.Fatalf("Up with %s missing below the applied version: err = %v, want an out-of-order error", skipped, err)
	}
	after, err := e.Provider.GetDBVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("version moved from %d to %d after the refused Up", before, after)
	}
}
