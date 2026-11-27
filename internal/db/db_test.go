package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
	"github.com/kom3da/pagewright/internal/db/migrations"
)

// defaultAdminURL is the dev database from deploy/compose.dev.yaml.
const defaultAdminURL = "postgres://pagewright:pagewright@localhost:5432/pagewright?sslmode=disable"

// testEnv is a fresh, migrated database with two tenants.
type testEnv struct {
	admin *pgxpool.Pool // superuser; bypasses RLS, used only for seeding and checks
	app   *pgxpool.Pool // runs as pagewright_app; subject to RLS
	db    *db.DB
	url   string // test database, as the admin user

	tenantA, tenantB  uuid.UUID
	alice, bob, carol uuid.UUID // alice: A, bob: B, carol: both
	ctxA, ctxB        context.Context
	postA, postB      dbgen.Content
	catA, catB        dbgen.Category
	tagA, tagB        dbgen.Tag
	mediaA, mediaB    dbgen.Medium
	revA, revB        dbgen.ContentRevision // published revision of postA / postB
	routeA, routeB    dbgen.Route           // canonical route of postA / postB
}

// tenantData is what seedTenant creates in one tenant.
type tenantData struct {
	post  dbgen.Content
	rev   dbgen.ContentRevision
	cat   dbgen.Category
	tag   dbgen.Tag
	media dbgen.Medium
	route dbgen.Route
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	adminURL := os.Getenv("PAGEWRIGHT_TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	server, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL (start it with `mise run dev`): %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(ctx); err != nil {
			t.Errorf("close server connection: %v", err)
		}
	})

	name := "pw_test_" + randomHex(t)
	if _, err := server.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := server.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	migrate(t, cfg.ConnConfig)

	admin, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	// The app connects with the same superuser URL on purpose: db.Open
	// must still drop to the app role.
	dbURL := withDatabase(t, adminURL, name)
	app := openDB(t, dbURL)
	e := &testEnv{admin: admin, app: app.Pool(), db: app, url: dbURL}
	e.seed(t)
	return e
}

func openDB(t *testing.T, url string) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

func withDatabase(t *testing.T, rawURL, name string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("PAGEWRIGHT_TEST_DATABASE_URL must be a URL: %v", err)
	}
	u.Path = "/" + name
	q := u.Query()
	if !q.Has("sslmode") {
		q.Set("sslmode", "disable")
	}
	u.RawQuery = q.Encode() // always non-empty, so callers can append "&..."
	return u.String()
}

func migrate(t *testing.T, cfg *pgx.ConnConfig) {
	t.Helper()
	sqlDB := stdlib.OpenDB(*cfg)
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close migration connection: %v", err)
		}
	}()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func randomHex(t *testing.T) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// seed creates tenants and users as admin (the app has no grant for that
// yet) and everything else through the app, so creation paths are covered.
func (e *testEnv) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	e.tenantA, e.tenantB = uuid.New(), uuid.New()
	e.alice, e.bob, e.carol = uuid.New(), uuid.New(), uuid.New()
	e.ctxA, e.ctxB = db.WithTenant(ctx, e.tenantA), db.WithTenant(ctx, e.tenantB)

	stmts := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO tenants (id, name) VALUES ($1, 'A'), ($2, 'B')", []any{e.tenantA, e.tenantB}},
		{"INSERT INTO users (id, email) VALUES ($1, 'alice@example.com'), ($2, 'bob@example.com'), ($3, 'carol@example.com')", []any{e.alice, e.bob, e.carol}},
		{"INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $3, 'admin'), ($2, $4, 'admin'), ($1, $5, 'editor'), ($2, $5, 'editor')", []any{e.tenantA, e.tenantB, e.alice, e.bob, e.carol}},
	}
	for _, s := range stmts {
		if _, err := e.admin.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("seed %q: %v", s.sql, err)
		}
	}

	// B's ids start after 1000, so B's ids are not also A's and "A asks
	// for B's id" really asks for a row A does not have. Overlapping ids
	// are tested in TestPerTenantIDs.
	dbtest.SkipIDs(t, e.admin, e.tenantB, 1000)
	a := e.seedTenant(t, e.ctxA, "a", e.alice)
	b := e.seedTenant(t, e.ctxB, "b", e.bob)
	e.postA, e.revA, e.catA, e.tagA, e.mediaA, e.routeA = a.post, a.rev, a.cat, a.tag, a.media, a.route
	e.postB, e.revB, e.catB, e.tagB, e.mediaB, e.routeB = b.post, b.rev, b.cat, b.tag, b.media, b.route
}

func (e *testEnv) seedTenant(t *testing.T, ctx context.Context, prefix string, author uuid.UUID) tenantData {
	t.Helper()
	var d tenantData
	err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
		var err error
		if d.post, err = q.CreateContent(ctx, newPost(author)); err != nil {
			return fmt.Errorf("content: %w", err)
		}
		if d.rev, err = q.CreateRevision(ctx, newRevision(d.post.ID, prefix+"-post", author)); err != nil {
			return fmt.Errorf("revision: %w", err)
		}
		if _, err = q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: d.post.ID, RevisionID: &d.rev.ID}); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		if d.post, err = q.GetContent(ctx, d.post.ID); err != nil {
			return fmt.Errorf("reload content: %w", err)
		}
		if d.route, err = q.CreateRoute(ctx, dbgen.CreateRouteParams{Path: "/posts/" + prefix + "/", Kind: "canonical", ContentID: &d.post.ID}); err != nil {
			return fmt.Errorf("route: %w", err)
		}
		if d.cat, err = q.CreateCategory(ctx, dbgen.CreateCategoryParams{Name: prefix, Slug: prefix + "-cat"}); err != nil {
			return fmt.Errorf("category: %w", err)
		}
		if d.tag, err = q.CreateTag(ctx, dbgen.CreateTagParams{Name: prefix, Slug: prefix + "-tag"}); err != nil {
			return fmt.Errorf("tag: %w", err)
		}
		if d.media, err = q.CreateMedia(ctx, dbgen.CreateMediaParams{
			StorageKey: prefix + "/photo.jpg", Filename: "photo.jpg", ContentType: "image/jpeg", SizeBytes: 10, UploadedBy: &author,
		}); err != nil {
			return fmt.Errorf("media: %w", err)
		}
		if err := q.AddContentCategory(ctx, dbgen.AddContentCategoryParams{ContentID: d.post.ID, CategoryID: d.cat.ID}); err != nil {
			return fmt.Errorf("content category: %w", err)
		}
		if err := q.AddContentTag(ctx, dbgen.AddContentTagParams{ContentID: d.post.ID, TagID: d.tag.ID}); err != nil {
			return fmt.Errorf("content tag: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", prefix, err)
	}
	return d
}

func newPost(author uuid.UUID) dbgen.CreateContentParams {
	return dbgen.CreateContentParams{Type: "post", AuthorID: &author}
}

func newRevision(contentID int64, title string, author uuid.UUID) dbgen.CreateRevisionParams {
	return dbgen.CreateRevisionParams{
		ContentID: contentID, Title: title, Body: []byte(`{"type":"doc","content":[]}`),
		BodySchemaVersion: 1, Fields: []byte(`{}`), CreatedBy: &author,
	}
}

// inA runs fn scoped to tenant A and fails the test on error.
func (e *testEnv) inA(t *testing.T, fn func(q *dbgen.Queries) error) {
	t.Helper()
	if err := e.db.InTenant(e.ctxA, fn); err != nil {
		t.Fatal(err)
	}
}

func TestAppRoleIsSubjectToRLS(t *testing.T) {
	e := newTestEnv(t)
	var user string
	var super, bypass bool
	err := e.app.QueryRow(context.Background(),
		"SELECT current_user, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&user, &super, &bypass)
	if err != nil {
		t.Fatal(err)
	}
	if user != "pagewright_app" || super || bypass {
		t.Fatalf("app runs as %s (super=%v, bypassrls=%v); want pagewright_app without either", user, super, bypass)
	}
	// Owners bypass RLS (it is not FORCEd), so the app must own nothing.
	var owned int
	err = e.app.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_class WHERE relowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)").Scan(&owned)
	if err != nil {
		t.Fatal(err)
	}
	if owned != 0 {
		t.Errorf("app role owns %d relations, want 0", owned)
	}
}

// SECURITY DEFINER functions run as the owner, so a caller's temporary
// objects must never shadow what they reference: pg_temp goes last.
func TestDefinerFunctionsPinSearchPath(t *testing.T) {
	e := newTestEnv(t)
	rows, err := e.admin.Query(context.Background(), `
		SELECT p.proname FROM pg_proc p
		WHERE p.prosecdef AND p.pronamespace = 'public'::regnamespace
		  AND NOT coalesce('search_path=pg_catalog, public, pg_temp' = ANY (p.proconfig), false)
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Errorf("SECURITY DEFINER functions without search_path = pg_catalog, public, pg_temp: %v", bad)
	}
}

func TestTenantCannotReadOtherTenant(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctxA

	e.inA(t, func(q *dbgen.Queries) error {
		tenant, err := q.GetCurrentTenant(ctx)
		if err != nil {
			return err
		}
		if tenant.ID != e.tenantA {
			t.Errorf("GetCurrentTenant = %v, want A", tenant.ID)
		}

		posts, err := q.ListContents(ctx, dbgen.ListContentsParams{Type: "post", MaxRows: 1000})
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]int64{e.postA.ID}, ids(posts, func(c dbgen.ListContentsRow) int64 { return c.ID })); diff != "" {
			t.Errorf("ListContents (-want +got):\n%s", diff)
		}
		if len(posts) == 1 && posts[0].Title != "a-post" {
			t.Errorf("ListContents title = %q, want %q", posts[0].Title, "a-post")
		}

		routes, err := q.ListRoutes(ctx)
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]int64{e.routeA.ID}, ids(routes, func(r dbgen.Route) int64 { return r.ID })); diff != "" {
			t.Errorf("ListRoutes (-want +got):\n%s", diff)
		}

		cats, err := q.ListCategories(ctx)
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]int64{e.catA.ID}, ids(cats, func(c dbgen.Category) int64 { return c.ID })); diff != "" {
			t.Errorf("ListCategories (-want +got):\n%s", diff)
		}

		tags, err := q.ListTags(ctx)
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]int64{e.tagA.ID}, ids(tags, func(c dbgen.Tag) int64 { return c.ID })); diff != "" {
			t.Errorf("ListTags (-want +got):\n%s", diff)
		}

		media, err := q.ListMedia(ctx)
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]int64{e.mediaA.ID}, ids(media, func(c dbgen.Medium) int64 { return c.ID })); diff != "" {
			t.Errorf("ListMedia (-want +got):\n%s", diff)
		}

		members, err := q.ListMembers(ctx)
		if err != nil {
			return err
		}
		wantMembers := []dbgen.ListMembersRow{
			{ID: e.alice, Email: "alice@example.com", Role: "admin"},
			{ID: e.carol, Email: "carol@example.com", Role: "editor"},
		}
		if diff := cmp.Diff(wantMembers, members); diff != "" {
			t.Errorf("ListMembers (-want +got):\n%s", diff)
		}

		// Both tenants have a content numbered 1; A must get its own.
		got, err := q.GetContentByNumber(ctx, 1)
		if err != nil {
			return err
		}
		if got.ID != e.postA.ID {
			t.Errorf("GetContentByNumber(1) = content %d, want %d", got.ID, e.postA.ID)
		}

		// Direct lookups by ID: A's own rows are found (so the query
		// works), B's are not.
		lookups := []struct {
			name       string
			own, other func() error
		}{
			{"GetContent",
				func() error { _, err := q.GetContent(ctx, e.postA.ID); return err },
				func() error { _, err := q.GetContent(ctx, e.postB.ID); return err }},
			{"GetMedia",
				func() error { _, err := q.GetMedia(ctx, e.mediaA.ID); return err },
				func() error { _, err := q.GetMedia(ctx, e.mediaB.ID); return err }},
			{"GetLatestRevision",
				func() error { _, err := q.GetLatestRevision(ctx, e.postA.ID); return err },
				func() error { _, err := q.GetLatestRevision(ctx, e.postB.ID); return err }},
			{"GetRouteByPath",
				func() error { _, err := q.GetRouteByPath(ctx, e.routeA.Path); return err },
				func() error { _, err := q.GetRouteByPath(ctx, e.routeB.Path); return err }},
			{"GetUser",
				func() error { _, err := q.GetUser(ctx, e.alice); return err },
				func() error { _, err := q.GetUser(ctx, e.bob); return err }},
		}
		for _, l := range lookups {
			if err := l.own(); err != nil {
				t.Errorf("%s of tenant A's row: %v", l.name, err)
			}
			if err := l.other(); !errors.Is(err, pgx.ErrNoRows) {
				t.Errorf("%s of tenant B's row: err = %v, want pgx.ErrNoRows", l.name, err)
			}
		}
		return nil
	})

	// Raw SQL, with no WHERE clause at all, still sees only A.
	for _, table := range []string{"tenants", "memberships", "contents", "content_revisions", "categories", "tags", "content_categories", "content_tags", "media", "routes"} {
		foreign := e.countInTenant(t, e.tenantA, "SELECT count(*) FROM "+table+" WHERE "+tenantColumn(table)+" <> $1", e.tenantA)
		if foreign != 0 {
			t.Errorf("%s: %d rows of other tenants visible", table, foreign)
		}
	}
}

func tenantColumn(table string) string {
	if table == "tenants" {
		return "id"
	}
	return "tenant_id"
}

// countInTenant runs a count query with app.tenant_id set to tenant.
func (e *testEnv) countInTenant(t *testing.T, tenant uuid.UUID, query string, args ...any) int {
	t.Helper()
	ctx := context.Background()
	var n int
	err := pgx.BeginFunc(ctx, e.app, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant.String()); err != nil {
			return err
		}
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	})
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestTenantCannotWriteOtherTenant(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctxA

	e.inA(t, func(q *dbgen.Queries) error {
		n, err := q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: e.postB.ID, RevisionID: nil})
		if err != nil || n != 0 {
			t.Errorf("unpublishing B's content: rows=%d err=%v, want 0 rows", n, err)
		}
		n, err = q.DeleteContent(ctx, e.postB.ID)
		if err != nil || n != 0 {
			t.Errorf("DeleteContent on B's content: rows=%d err=%v, want 0 rows", n, err)
		}
		return nil
	})

	// Each attempt runs in its own transaction because a failed statement
	// aborts the transaction.
	attempts := []struct {
		name     string
		wantCode string
		run      func(q *dbgen.Queries) error
	}{
		{"link A's content to B's category", "23503", func(q *dbgen.Queries) error {
			return q.AddContentCategory(ctx, dbgen.AddContentCategoryParams{ContentID: e.postA.ID, CategoryID: e.catB.ID})
		}},
		{"link A's content to B's tag", "23503", func(q *dbgen.Queries) error {
			return q.AddContentTag(ctx, dbgen.AddContentTagParams{ContentID: e.postA.ID, TagID: e.tagB.ID})
		}},
		{"category under B's category", "23503", func(q *dbgen.Queries) error {
			_, err := q.CreateCategory(ctx, dbgen.CreateCategoryParams{ParentID: &e.catB.ID, Name: "x", Slug: "x"})
			return err
		}},
		{"page under B's content", "23503", func(q *dbgen.Queries) error {
			p := newPost(e.alice)
			p.Type, p.ParentID = "page", &e.postB.ID
			_, err := q.CreateContent(ctx, p)
			return err
		}},
		{"content authored by a non-member", "23503", func(q *dbgen.Queries) error {
			_, err := q.CreateContent(ctx, newPost(e.bob))
			return err
		}},
		{"revision of B's content", "23503", func(q *dbgen.Queries) error {
			_, err := q.CreateRevision(ctx, newRevision(e.postB.ID, "x", e.alice))
			return err
		}},
		{"publish B's revision on A's content", "23503", func(q *dbgen.Queries) error {
			_, err := q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: e.postA.ID, RevisionID: &e.revB.ID})
			return err
		}},
		{"route to B's content", "23503", func(q *dbgen.Queries) error {
			_, err := q.CreateRoute(ctx, dbgen.CreateRouteParams{Path: "/x/", Kind: "redirect", ContentID: &e.postB.ID})
			return err
		}},
	}
	for _, a := range attempts {
		err := e.db.InTenant(ctx, a.run)
		if code := pgCode(err); code != a.wantCode {
			t.Errorf("%s: err = %v, want SQLSTATE %s", a.name, err, a.wantCode)
		}
	}

	// Writing B's tenant_id explicitly is rejected by the policy.
	rawWrites := []string{
		"INSERT INTO tags (tenant_id, name, slug) VALUES ($1, 'x', 'x')",
		"INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, '" + e.alice.String() + "', 'admin')",
		"UPDATE tags SET tenant_id = $1",
	}
	for _, q := range rawWrites {
		err := e.execInTenant(e.tenantA, q, e.tenantB)
		if code := pgCode(err); code != "42501" {
			t.Errorf("%q: err = %v, want SQLSTATE 42501 (RLS violation)", q, err)
		}
	}

	// B's content is still there and published.
	var published *int64
	if err := e.admin.QueryRow(context.Background(), "SELECT published_revision_id FROM contents WHERE tenant_id = $1 AND id = $2", e.tenantB, e.postB.ID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published == nil || *published != e.revB.ID {
		t.Errorf("B's published revision = %v, want %d", published, e.revB.ID)
	}
}

func (e *testEnv) execInTenant(tenant uuid.UUID, query string, args ...any) error {
	ctx := context.Background()
	return pgx.BeginFunc(ctx, e.app, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, query, args...)
		return err
	})
}

func TestNoTenantSeesNothing(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	if err := e.db.InTenant(ctx, func(*dbgen.Queries) error { return nil }); !errors.Is(err, db.ErrNoTenant) {
		t.Errorf("InTenant without tenant: err = %v, want ErrNoTenant", err)
	}

	for _, table := range []string{"tenants", "users", "memberships", "contents", "content_revisions", "categories", "tags", "content_categories", "content_tags", "media", "routes"} {
		var n int
		if err := e.app.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows visible without a tenant, want 0", table, n)
		}
	}

	_, err := e.app.Exec(ctx, "INSERT INTO tags (name, slug) VALUES ('x', 'x')")
	if code := pgCode(err); code != "42501" {
		t.Errorf("insert without a tenant: err = %v, want SQLSTATE 42501 (RLS violation)", err)
	}
}

func TestTenantSettingDoesNotOutliveTransaction(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// A single connection, so the query after InTenant reuses its connection.
	one := openDB(t, e.url+"&pool_max_conns=1")
	err := one.InTenant(e.ctxA, func(q *dbgen.Queries) error {
		posts, err := q.ListContents(e.ctxA, dbgen.ListContentsParams{Type: "post", MaxRows: 1000})
		if len(posts) == 0 {
			t.Error("tenant A sees no contents inside InTenant")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	var setting string
	var n int
	err = one.Pool().QueryRow(ctx,
		"SELECT coalesce(current_setting('app.tenant_id', true), ''), (SELECT count(*) FROM contents)").Scan(&setting, &n)
	if err != nil {
		t.Fatal(err)
	}
	if setting != "" || n != 0 {
		t.Errorf("after InTenant: app.tenant_id = %q and %d contents visible, want empty and 0", setting, n)
	}
}

func TestContentNumbersArePerTenant(t *testing.T) {
	e := newTestEnv(t)
	if e.postA.Number != 1 || e.postB.Number != 1 {
		t.Fatalf("first numbers: A=%d B=%d, want 1 and 1", e.postA.Number, e.postB.Number)
	}
	e.inA(t, func(q *dbgen.Queries) error {
		c, err := q.CreateContent(e.ctxA, newPost(e.alice))
		if err != nil {
			return err
		}
		if c.Number != 2 {
			t.Errorf("second content in A: number = %d, want 2", c.Number)
		}
		return nil
	})
}

func TestExplicitContentNumberAdvancesCounter(t *testing.T) {
	e := newTestEnv(t)
	// As when keeping a WordPress post ID on import.
	err := e.execInTenant(e.tenantA,
		"INSERT INTO contents (number, type) VALUES (241, 'post')")
	if err != nil {
		t.Fatal(err)
	}
	e.inA(t, func(q *dbgen.Queries) error {
		c, err := q.CreateContent(e.ctxA, newPost(e.alice))
		if err != nil {
			return err
		}
		if c.Number != 242 {
			t.Errorf("number after importing 241 = %d, want 242", c.Number)
		}
		return nil
	})
}

func TestAllDigitSlugIsRejected(t *testing.T) {
	e := newTestEnv(t)
	err := e.db.InTenant(e.ctxA, func(q *dbgen.Queries) error {
		p := newPost(e.alice)
		slug := "1"
		p.Slug = &slug
		_, err := q.CreateContent(e.ctxA, p)
		return err
	})
	if code := pgCode(err); code != "23514" {
		t.Errorf("slug %q: err = %v, want SQLSTATE 23514 (check violation)", "1", err)
	}
}

func TestRemovingAnAuthorIsRefused(t *testing.T) {
	e := newTestEnv(t)
	_, err := e.admin.Exec(context.Background(),
		"DELETE FROM memberships WHERE tenant_id = $1 AND user_id = $2", e.tenantA, e.alice)
	if code := pgCode(err); code != "23503" {
		t.Errorf("removing alice from A: err = %v, want SQLSTATE 23503", err)
	}
}

func TestRevisions(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctxA

	attempts := []struct {
		name     string
		wantCode string
		run      func(q *dbgen.Queries) error
	}{
		{"publish another content's revision", "23503", func(q *dbgen.Queries) error {
			other, err := q.CreateContent(ctx, newPost(e.alice))
			if err != nil {
				return err
			}
			_, err = q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: other.ID, RevisionID: &e.revA.ID})
			return err
		}},
		{"delete the published revision", "23503", func(q *dbgen.Queries) error {
			_, err := q.DeleteRevision(ctx, e.revA.ID)
			return err
		}},
	}
	for _, a := range attempts {
		err := e.db.InTenant(ctx, a.run)
		if code := pgCode(err); code != a.wantCode {
			t.Errorf("%s: err = %v, want SQLSTATE %s", a.name, err, a.wantCode)
		}
	}

	// Revisions are append-only, so a live revision never changes.
	err := e.execInTenant(e.tenantA, "UPDATE content_revisions SET title = 'changed'")
	if code := pgCode(err); code != "42501" {
		t.Errorf("updating a revision: err = %v, want SQLSTATE 42501", err)
	}

	// Saving a new draft leaves the published revision alone.
	e.inA(t, func(q *dbgen.Queries) error {
		draft, err := q.CreateRevision(ctx, newRevision(e.postA.ID, "draft", e.alice))
		if err != nil {
			return err
		}
		latest, err := q.GetLatestRevision(ctx, e.postA.ID)
		if err != nil {
			return err
		}
		c, err := q.GetContent(ctx, e.postA.ID)
		if err != nil {
			return err
		}
		if latest.ID != draft.ID || c.PublishedRevisionID == nil || *c.PublishedRevisionID != e.revA.ID {
			t.Errorf("latest = %d (want %d), published = %v (want %d)", latest.ID, draft.ID, c.PublishedRevisionID, e.revA.ID)
		}
		return nil
	})

	// Deleting a content takes its revisions and routes with it.
	e.inA(t, func(q *dbgen.Queries) error {
		n, err := q.DeleteContent(ctx, e.postA.ID)
		if err != nil || n != 1 {
			t.Errorf("DeleteContent: rows=%d err=%v, want 1", n, err)
		}
		return nil
	})
	if n := e.countInTenant(t, e.tenantA, "SELECT (SELECT count(*) FROM content_revisions) + (SELECT count(*) FROM routes)"); n != 0 {
		t.Errorf("%d revisions and routes left after deleting the content, want 0", n)
	}
}

func TestRoutes(t *testing.T) {
	e := newTestEnv(t)
	ctx := e.ctxA

	attempts := []struct {
		name     string
		path     string
		wantCode string
	}{
		{"same path as an existing route", e.routeA.Path, "23505"},
		{"query string", "/?p=123", "23514"},
		{"percent-encoded", "/%E3%81%8A/", "23514"},
		{"not NFC", "/\u30ab\u3099/", "23514"}, // カ + combining dakuten
		{"relative", "posts/x/", "23514"},
	}
	for _, a := range attempts {
		path := a.path
		err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
			_, err := q.CreateRoute(ctx, dbgen.CreateRouteParams{Path: path, Kind: "redirect", CategoryID: &e.catA.ID})
			return err
		})
		if code := pgCode(err); code != a.wantCode {
			t.Errorf("%s (%q): err = %v, want SQLSTATE %s", a.name, path, err, a.wantCode)
		}
	}

	// A second canonical URL for the same content is refused.
	err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
		_, err := q.CreateRoute(ctx, dbgen.CreateRouteParams{Path: "/other/", Kind: "canonical", ContentID: &e.postA.ID})
		return err
	})
	if code := pgCode(err); code != "23505" {
		t.Errorf("second canonical route: err = %v, want SQLSTATE 23505", err)
	}

	// Paths are per tenant (B already uses /posts/b/), and NFC Japanese
	// paths are accepted as redirects.
	e.inA(t, func(q *dbgen.Queries) error {
		for _, p := range []string{e.routeB.Path, "/お知らせ/"} {
			if _, err := q.CreateRoute(ctx, dbgen.CreateRouteParams{Path: p, Kind: "redirect", ContentID: &e.postA.ID}); err != nil {
				t.Errorf("route %q in A: %v", p, err)
			}
		}
		return nil
	})
}

func TestConcurrentContentNumbersAreUnique(t *testing.T) {
	e := newTestEnv(t)
	const n = 10
	nums := make(chan int64, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			err := e.db.InTenant(e.ctxA, func(q *dbgen.Queries) error {
				c, err := q.CreateContent(e.ctxA, newPost(e.alice))
				nums <- c.Number
				return err
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	close(nums)

	var got []int64
	for num := range nums {
		got = append(got, num)
	}
	slices.Sort(got)
	want := make([]int64, n)
	for i := range want {
		want[i] = int64(i + 2) // 1 is the seeded post
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("numbers (-want +got):\n%s", diff)
	}
}

func TestFindUserByEmail(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	got, err := e.db.FindUserByEmail(ctx, "Carol@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := dbgen.FindUserByEmailRow{ID: e.carol, Email: "carol@example.com"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FindUserByEmail (-want +got):\n%s", diff)
	}

	if _, err := e.db.FindUserByEmail(ctx, "nobody@example.com"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown email: err = %v, want pgx.ErrNoRows", err)
	}
}

func TestAppCannotCreateTenantsOrUsers(t *testing.T) {
	e := newTestEnv(t)
	for _, q := range []string{
		"INSERT INTO tenants (name) VALUES ('x')",
		"INSERT INTO users (email) VALUES ('x@example.com')",
		"UPDATE users SET email = 'x@example.com'",
		// A user can belong to several tenants, so no tenant may edit them.
		"UPDATE users SET display_name = 'renamed'",
		// Membership changes come with invitations; adding a member would
		// also expose that user's profile to the tenant.
		"INSERT INTO memberships (user_id, role) VALUES ('" + e.bob.String() + "', 'admin')",
		"UPDATE memberships SET role = 'admin'",
		"DELETE FROM memberships",
	} {
		err := e.execInTenant(e.tenantA, q)
		if code := pgCode(err); code != "42501" {
			t.Errorf("%q: err = %v, want SQLSTATE 42501 (permission denied)", q, err)
		}
	}
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func ids[T any](rows []T, id func(T) int64) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, id(r))
	}
	return out
}
