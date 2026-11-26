package wxrimport_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/migrations"
	"github.com/kom3da/pagewright/internal/migrate/wxrimport"
)

const defaultAdminURL = "postgres://pagewright:pagewright@localhost:5432/pagewright?sslmode=disable"

const fixture = "../wxr/testdata/sample.wxr"

// env is a fresh migrated database with tenants A (import target) and B.
type env struct {
	admin            *pgxpool.Pool // superuser, bypasses RLS: seeding and checks only
	db               *db.DB        // runs as pagewright_app
	tenantA, tenantB uuid.UUID
	alice, bob       uuid.UUID // alice: member of A, bob: member of B
	mediaRoot        string
	client           *http.Client
	downloads        *atomic.Int64
}

func newEnv(t *testing.T) *env {
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
	t.Cleanup(func() { _ = server.Close(ctx) })

	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "pw_test_" + hex.EncodeToString(b)
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
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = sqlDB.Close()

	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	d, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)

	e := &env{admin: admin, db: d, tenantA: uuid.New(), tenantB: uuid.New(), alice: uuid.New(), bob: uuid.New(), mediaRoot: t.TempDir()}
	e.exec(t, "INSERT INTO tenants (id, name) VALUES ($1, 'A'), ($2, 'B')", e.tenantA, e.tenantB)
	e.exec(t, "INSERT INTO users (id, email) VALUES ($1, 'alice@example.com'), ($2, 'bob@example.com')", e.alice, e.bob)
	e.exec(t, "INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $3, 'admin'), ($2, $4, 'admin')", e.tenantA, e.tenantB, e.alice, e.bob)
	// Tenant B has a post with the same number and paths the import uses:
	// numbers and paths are per tenant, so neither may get in the way.
	e.exec(t, "INSERT INTO contents (tenant_id, number, type, slug, author_id) VALUES ($1, 101, 'post', 'company', $2)", e.tenantB, e.bob)
	e.exec(t, "INSERT INTO routes (tenant_id, path, kind, content_id) SELECT $1, '/company/', 'canonical', id FROM contents WHERE tenant_id = $1", e.tenantB)
	e.exec(t, "INSERT INTO categories (tenant_id, name, slug) VALUES ($1, 'B news', 'news')", e.tenantB)
	// Tenant A already uses the old URL of post 102 for something else.
	e.exec(t, "INSERT INTO contents (tenant_id, number, type, slug, author_id) VALUES ($1, 1, 'page', 'sale', $2)", e.tenantA, e.alice)
	e.exec(t, "INSERT INTO routes (tenant_id, path, kind, content_id) SELECT $1, '/2024/07/summer-sale/', 'canonical', id FROM contents WHERE tenant_id = $1", e.tenantA)

	e.downloads = &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wp-content/uploads/2024/05/shop.jpg" {
			http.NotFound(w, r)
			return
		}
		e.downloads.Add(1)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3)), nil); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	e.client = &http.Client{Timeout: 5 * time.Second, Transport: rewriteHost{target}}
	return e
}

// rewriteHost sends every request to the test server, whatever the host
// in the fixture's URLs.
type rewriteHost struct{ target *url.URL }

func (rt rewriteHost) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = rt.target.Scheme, rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *env) options(keepURLs, dryRun bool) wxrimport.Options {
	return wxrimport.Options{
		AuthorID:      e.alice,
		DryRun:        dryRun,
		KeepURLs:      keepURLs,
		DownloadMedia: true,
		MediaRoot:     e.mediaRoot,
		HTTPClient:    e.client,
	}
}

func (e *env) importFixture(t *testing.T, opts wxrimport.Options) wxrimport.Report {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rep, err := wxrimport.Import(context.Background(), e.db, e.tenantA, f, opts)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return rep
}

// snapshot dumps a tenant's rows (as JSON per table) to compare states.
func (e *env) snapshot(t *testing.T, tenant uuid.UUID) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range []string{"contents", "content_revisions", "categories", "tags", "content_categories", "content_tags", "media", "routes", "tenants"} {
		col := "tenant_id"
		if table == "tenants" {
			col = "id"
		}
		rows, err := e.admin.Query(context.Background(), "SELECT row_to_json(x)::text FROM "+table+" x WHERE "+col+" = $1 ORDER BY 1", tenant)
		if err != nil {
			t.Fatal(err)
		}
		s, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		out[table] = s
	}
	return out
}

type contentRow struct {
	Number      int64
	Type        string
	Slug        *string
	ParentNum   *int64
	Published   bool
	PublishedAt *time.Time
	Scheduled   bool
	Title       string
}

func (e *env) contents(t *testing.T) []contentRow {
	t.Helper()
	rows, err := e.admin.Query(context.Background(), `
		SELECT c.number, c.type, c.slug, p.number, c.published_revision_id IS NOT NULL,
		       c.published_at, c.scheduled_revision_id IS NOT NULL, r.title
		FROM contents c
		LEFT JOIN contents p ON p.tenant_id = c.tenant_id AND p.id = c.parent_id
		LEFT JOIN LATERAL (SELECT title FROM content_revisions WHERE tenant_id = c.tenant_id AND content_id = c.id ORDER BY id DESC LIMIT 1) r ON true
		WHERE c.tenant_id = $1 ORDER BY c.number`, e.tenantA)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (contentRow, error) {
		var c contentRow
		var title *string
		err := r.Scan(&c.Number, &c.Type, &c.Slug, &c.ParentNum, &c.Published, &c.PublishedAt, &c.Scheduled, &title)
		if title != nil {
			c.Title = *title
		}
		return c, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type routeRow struct {
	Path, Kind, Target string
}

func (e *env) routes(t *testing.T) []routeRow {
	t.Helper()
	rows, err := e.admin.Query(context.Background(), `
		SELECT r.path, r.kind, coalesce('content:' || c.number, 'category:' || cat.slug, 'tag:' || tg.slug)
		FROM routes r
		LEFT JOIN contents c ON c.tenant_id = r.tenant_id AND c.id = r.content_id
		LEFT JOIN categories cat ON cat.tenant_id = r.tenant_id AND cat.id = r.category_id
		LEFT JOIN tags tg ON tg.tenant_id = r.tenant_id AND tg.id = r.tag_id
		WHERE r.tenant_id = $1 ORDER BY r.path`, e.tenantA)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[routeRow])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func gmt(s string) *time.Time {
	tm, err := time.Parse(time.DateTime, s)
	if err != nil {
		panic(err)
	}
	return &tm
}

func TestImportKeepURLs(t *testing.T) {
	e := newEnv(t)
	before := e.snapshot(t, e.tenantB)

	rep := e.importFixture(t, e.options(true, false))

	wantCounts := map[string]wxrimport.Counts{
		"posts":      {Created: 4, Skipped: 2},
		"pages":      {Created: 3},
		"other":      {Skipped: 1},
		"categories": {Created: 3},
		"tags":       {Created: 2},
		"media":      {Created: 1},
	}
	gotCounts := map[string]wxrimport.Counts{
		"posts": rep.Posts, "pages": rep.Pages, "other": rep.Other,
		"categories": rep.Categories, "tags": rep.Tags, "media": rep.Media,
	}
	if diff := cmp.Diff(wantCounts, gotCounts); diff != "" {
		t.Errorf("counts (-want +got):\n%s", diff)
	}
	if rep.Published != 4 || rep.Drafts != 2 || rep.Scheduled != 1 {
		t.Errorf("published/drafts/scheduled = %d/%d/%d, want 4/2/1", rep.Published, rep.Drafts, rep.Scheduled)
	}

	wantContents := []contentRow{
		{Number: 1, Type: "page", Slug: ptr("sale")}, // seeded
		{Number: 101, Type: "post", Slug: ptr("開店のお知らせ"), Published: true, PublishedAt: gmt("2024-05-01 01:00:00"), Title: "開店のお知らせ"},
		{Number: 102, Type: "post", Slug: ptr("summer-sale"), Published: true, PublishedAt: gmt("2024-07-01 00:00:00"), Title: "夏のセール"},
		{Number: 103, Type: "post", Title: "書きかけの記事"},
		{Number: 106, Type: "post", Slug: ptr("winter"), Scheduled: true, Title: "冬の新商品"},
		{Number: 201, Type: "page", Slug: ptr("company"), Published: true, PublishedAt: gmt("2024-04-01 00:00:00"), Title: "会社概要"},
		{Number: 202, Type: "page", Slug: ptr("access"), ParentNum: ptr[int64](201), Published: true, PublishedAt: gmt("2024-04-02 00:00:00"), Title: "アクセス"},
		{Number: 203, Type: "page", Slug: ptr("internal"), Title: "社内向け"},
	}
	if diff := cmp.Diff(wantContents, e.contents(t), cmpopts.EquateApproxTime(0)); diff != "" {
		t.Errorf("contents (-want +got):\n%s", diff)
	}

	wantRoutes := []routeRow{
		{"/2024/05/開店のお知らせ/", "canonical", "content:101"},
		{"/2024/07/summer-sale/", "canonical", "content:1"}, // taken before the import
		{"/2026/12/winter/", "canonical", "content:106"},
		{"/category/news/", "canonical", "category:news"},
		{"/category/news/event/", "canonical", "category:event"},
		{"/category/お店/", "canonical", "category:お店"},
		{"/company/", "canonical", "content:201"},
		{"/company/access/", "canonical", "content:202"},
		{"/internal/", "canonical", "content:203"},
		{"/posts/103/", "canonical", "content:103"}, // the old URL was ?p=103
		{"/posts/summer-sale/", "canonical", "content:102"},
		{"/tag/sale/", "canonical", "tag:sale"},
		{"/tag/新商品/", "canonical", "tag:新商品"},
	}
	if diff := cmp.Diff(wantRoutes, e.routes(t)); diff != "" {
		t.Errorf("routes (-want +got):\n%s", diff)
	}

	// The tenant counter moved past the imported numbers.
	var last int64
	if err := e.admin.QueryRow(context.Background(), "SELECT last_content_number FROM tenants WHERE id = $1", e.tenantA).Scan(&last); err != nil {
		t.Fatal(err)
	}
	if last != 203 {
		t.Errorf("last_content_number = %d, want 203", last)
	}

	// Body, source, fields and media of post 101.
	var body, fields []byte
	var source, format string
	if err := e.admin.QueryRow(context.Background(), `
		SELECT r.body, r.fields, c.source_body, c.source_format FROM contents c
		JOIN content_revisions r ON r.tenant_id = c.tenant_id AND r.content_id = c.id
		WHERE c.tenant_id = $1 AND c.number = 101`, e.tenantA).Scan(&body, &fields, &source, &format); err != nil {
		t.Fatal(err)
	}
	if format != "wordpress_html" || !strings.Contains(source, "<!-- wp:embed") {
		t.Errorf("source = %q (%s)", source[:40], format)
	}
	var mediaID int64
	if err := e.admin.QueryRow(context.Background(), "SELECT id FROM media WHERE tenant_id = $1 AND storage_key = 'wp/2024/05/shop.jpg' AND width = 4 AND height = 3", e.tenantA).Scan(&mediaID); err != nil {
		t.Fatalf("media row: %v", err)
	}
	wantImage := map[string]any{"type": "image", "attrs": map[string]any{
		"src": "/media/wp/2024/05/shop.jpg", "alt": "店の外観", "title": nil, "mediaId": float64(mediaID),
	}}
	var doc struct{ Content []map[string]any }
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	var gotImage map[string]any
	for _, n := range doc.Content {
		if n["type"] == "image" {
			gotImage = n
		}
	}
	if diff := cmp.Diff(wantImage, gotImage); diff != "" {
		t.Errorf("image node (-want +got):\n%s", diff)
	}
	var gotFields map[string]any
	if err := json.Unmarshal(fields, &gotFields); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[string]any{"price": "1200", "excerpt": "開店しました。"}, gotFields); diff != "" {
		t.Errorf("fields (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(filepath.Join(e.mediaRoot, e.tenantA.String(), "wp", "2024", "05", "shop.jpg")); err != nil {
		t.Errorf("media file: %v", err)
	}

	var cats []string
	rows, err := e.admin.Query(context.Background(), `
		SELECT cat.slug || ':' || coalesce(parent.slug, '') FROM content_categories cc
		JOIN contents c ON c.tenant_id = cc.tenant_id AND c.id = cc.content_id
		JOIN categories cat ON cat.tenant_id = cc.tenant_id AND cat.id = cc.category_id
		LEFT JOIN categories parent ON parent.tenant_id = cat.tenant_id AND parent.id = cat.parent_id
		WHERE c.tenant_id = $1 AND c.number = 101 ORDER BY 1`, e.tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if cats, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"event:news", "news:"}, cats); diff != "" {
		t.Errorf("categories of 101 (-want +got):\n%s", diff)
	}

	// Things that could not be converted are in the report.
	text := reportText(t, rep)
	for _, want := range []string{
		"ショートコード（[contact-form-7",
		"ショートコード（[caption]）",
		"仮に「bread_photo」",
		"/posts/summer-sale/",
		"ゴミ箱に入っています",
		"種類「nav_menu_item」は取り込みません",
		"<script> は変換できない",
		"非公開（private）",
		"コメント 1 件",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}

	if diff := cmp.Diff(before, e.snapshot(t, e.tenantB)); diff != "" {
		t.Errorf("tenant B changed (-before +after):\n%s", diff)
	}

	// Running again finds everything and creates nothing.
	afterFirst := e.snapshot(t, e.tenantA)
	downloads := e.downloads.Load()
	again := e.importFixture(t, e.options(true, false))
	wantAgain := map[string]wxrimport.Counts{
		"posts":      {Existing: 4, Skipped: 2},
		"pages":      {Existing: 3},
		"other":      {Skipped: 1},
		"categories": {Existing: 3},
		"tags":       {Existing: 2},
		"media":      {Existing: 1},
	}
	gotAgain := map[string]wxrimport.Counts{
		"posts": again.Posts, "pages": again.Pages, "other": again.Other,
		"categories": again.Categories, "tags": again.Tags, "media": again.Media,
	}
	if diff := cmp.Diff(wantAgain, gotAgain); diff != "" {
		t.Errorf("second run counts (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(afterFirst, e.snapshot(t, e.tenantA)); diff != "" {
		t.Errorf("second run changed tenant A (-before +after):\n%s", diff)
	}
	if n := e.downloads.Load(); n != downloads {
		t.Errorf("second run downloaded %d files", n-downloads)
	}
	if !strings.Contains(reportText(t, again), "番号 101 のコンテンツがすでにあります") {
		t.Errorf("second run report does not mention duplicates:\n%s", reportText(t, again))
	}
}

func TestImportPagewrightURLs(t *testing.T) {
	e := newEnv(t)
	rep := e.importFixture(t, e.options(false, false))
	if rep.Posts.Created != 4 || rep.Pages.Created != 3 {
		t.Fatalf("posts %+v pages %+v", rep.Posts, rep.Pages)
	}
	want := []routeRow{
		{"/2024/05/開店のお知らせ/", "redirect", "content:101"},
		{"/2024/07/summer-sale/", "canonical", "content:1"}, // seeded; no redirect possible
		{"/2026/12/winter/", "redirect", "content:106"},
		{"/category/news/", "redirect", "category:news"},
		{"/category/news/event/", "redirect", "category:event"},
		{"/category/お店/", "redirect", "category:お店"},
		{"/company/", "canonical", "content:201"},
		{"/company/access/", "canonical", "content:202"},
		{"/internal/", "canonical", "content:203"},
		{"/posts/103/", "canonical", "content:103"},
		{"/posts/category/event/", "canonical", "category:event"},
		{"/posts/category/news/", "canonical", "category:news"},
		{"/posts/category/お店/", "canonical", "category:お店"},
		{"/posts/summer-sale/", "canonical", "content:102"},
		{"/posts/tag/sale/", "canonical", "tag:sale"},
		{"/posts/tag/新商品/", "canonical", "tag:新商品"},
		{"/posts/winter/", "canonical", "content:106"},
		{"/posts/開店のお知らせ/", "canonical", "content:101"},
		{"/tag/sale/", "redirect", "tag:sale"},
		{"/tag/新商品/", "redirect", "tag:新商品"},
	}
	if diff := cmp.Diff(want, e.routes(t)); diff != "" {
		t.Errorf("routes (-want +got):\n%s", diff)
	}
	if !strings.Contains(reportText(t, rep), "リダイレクトを作りませんでした") {
		t.Errorf("collision not reported:\n%s", reportText(t, rep))
	}
}

func TestImportDryRun(t *testing.T) {
	e := newEnv(t)
	beforeA, beforeB := e.snapshot(t, e.tenantA), e.snapshot(t, e.tenantB)

	dry := e.importFixture(t, e.options(true, true))

	if diff := cmp.Diff(beforeA, e.snapshot(t, e.tenantA)); diff != "" {
		t.Errorf("dry run changed tenant A (-before +after):\n%s", diff)
	}
	if diff := cmp.Diff(beforeB, e.snapshot(t, e.tenantB)); diff != "" {
		t.Errorf("dry run changed tenant B (-before +after):\n%s", diff)
	}
	if entries, err := os.ReadDir(e.mediaRoot); err != nil || len(entries) != 0 {
		t.Errorf("dry run wrote media files: %v %v", entries, err)
	}
	if !strings.Contains(reportText(t, dry), "ドライラン") {
		t.Error("report does not say it is a dry run")
	}

	real := e.importFixture(t, e.options(true, false))
	if diff := cmp.Diff(real, dry, cmpopts.IgnoreFields(wxrimport.Report{}, "DryRun")); diff != "" {
		t.Errorf("dry run report differs from the real one (-real +dry):\n%s", diff)
	}
}

func TestImportRejectsNonMember(t *testing.T) {
	e := newEnv(t)
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	opts := e.options(true, false)
	opts.AuthorID = e.bob // member of B only
	if _, err := wxrimport.Import(context.Background(), e.db, e.tenantA, f, opts); err == nil {
		t.Fatal("import with an author from another tenant: want error")
	}
	if got := e.contents(t); len(got) != 1 {
		t.Errorf("contents after failed import: %+v", got)
	}
}

func reportText(t *testing.T, r wxrimport.Report) string {
	t.Helper()
	var b strings.Builder
	if err := r.WriteText(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
