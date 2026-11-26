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

	e.postA, e.catA, e.tagA, e.mediaA = e.seedTenant(t, e.ctxA, "a", e.alice)
	e.postB, e.catB, e.tagB, e.mediaB = e.seedTenant(t, e.ctxB, "b", e.bob)
}

func (e *testEnv) seedTenant(t *testing.T, ctx context.Context, prefix string, author uuid.UUID) (post dbgen.Content, cat dbgen.Category, tag dbgen.Tag, media dbgen.Medium) {
	t.Helper()
	err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
		var err error
		if post, err = q.CreateContent(ctx, newPost(prefix+"-post", author)); err != nil {
			return fmt.Errorf("content: %w", err)
		}
		if cat, err = q.CreateCategory(ctx, dbgen.CreateCategoryParams{Name: prefix, Slug: prefix + "-cat"}); err != nil {
			return fmt.Errorf("category: %w", err)
		}
		if tag, err = q.CreateTag(ctx, dbgen.CreateTagParams{Name: prefix, Slug: prefix + "-tag"}); err != nil {
			return fmt.Errorf("tag: %w", err)
		}
		if media, err = q.CreateMedia(ctx, dbgen.CreateMediaParams{
			StorageKey: prefix + "/photo.jpg", Filename: "photo.jpg", ContentType: "image/jpeg", SizeBytes: 10, UploadedBy: &author,
		}); err != nil {
			return fmt.Errorf("media: %w", err)
		}
		if err := q.AddContentCategory(ctx, dbgen.AddContentCategoryParams{ContentID: post.ID, CategoryID: cat.ID}); err != nil {
			return fmt.Errorf("content category: %w", err)
		}
		if err := q.AddContentTag(ctx, dbgen.AddContentTagParams{ContentID: post.ID, TagID: tag.ID}); err != nil {
			return fmt.Errorf("content tag: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", prefix, err)
	}
	return post, cat, tag, media
}

func newPost(title string, author uuid.UUID) dbgen.CreateContentParams {
	return dbgen.CreateContentParams{
		Type: "post", Title: title, Body: []byte(`{}`), BodySchemaVersion: 1,
		Status: "draft", AuthorID: &author, Fields: []byte(`{}`),
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

		posts, err := q.ListContents(ctx, "post")
		if err != nil {
			return err
		}
		if diff := cmp.Diff([]int64{e.postA.ID}, ids(posts, func(c dbgen.Content) int64 { return c.ID })); diff != "" {
			t.Errorf("ListContents (-want +got):\n%s", diff)
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
	for _, table := range []string{"tenants", "memberships", "contents", "categories", "tags", "content_categories", "content_tags", "media"} {
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
		n, err := q.UpdateContentTitle(ctx, dbgen.UpdateContentTitleParams{ID: e.postB.ID, Title: "hijacked"})
		if err != nil || n != 0 {
			t.Errorf("UpdateContentTitle on B's content: rows=%d err=%v, want 0 rows", n, err)
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
			p := newPost("x", e.alice)
			p.Type, p.ParentID = "page", &e.postB.ID
			_, err := q.CreateContent(ctx, p)
			return err
		}},
		{"content authored by a non-member", "23503", func(q *dbgen.Queries) error {
			_, err := q.CreateContent(ctx, newPost("x", e.bob))
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

	// B's data is untouched.
	var title string
	if err := e.admin.QueryRow(context.Background(), "SELECT title FROM contents WHERE id = $1", e.postB.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != e.postB.Title {
		t.Errorf("B's content title = %q, want %q", title, e.postB.Title)
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

	for _, table := range []string{"tenants", "users", "memberships", "contents", "categories", "tags", "content_categories", "content_tags", "media"} {
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
		posts, err := q.ListContents(e.ctxA, "post")
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
		c, err := q.CreateContent(e.ctxA, newPost("second", e.alice))
		if err != nil {
			return err
		}
		if c.Number != 2 {
			t.Errorf("second content in A: number = %d, want 2", c.Number)
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
				c, err := q.CreateContent(e.ctxA, newPost("concurrent", e.alice))
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
