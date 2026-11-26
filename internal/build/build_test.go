package build_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// seeder creates content in one tenant through the app's access layer.
type seeder struct {
	t      *testing.T
	env    *dbtest.Env
	ctx    context.Context
	author uuid.UUID
}

func newSeeder(t *testing.T, env *dbtest.Env, tenantName, email string) *seeder {
	t.Helper()
	tenant := env.CreateTenant(t, tenantName)
	author := uuid.New()
	ctx := context.Background()
	if _, err := env.Admin.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, $2)", author, email); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Admin.Exec(ctx, "INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')", tenant, author); err != nil {
		t.Fatal(err)
	}
	return &seeder{t: t, env: env, ctx: db.WithTenant(ctx, tenant), author: author}
}

func (s *seeder) tenant() uuid.UUID {
	id, _ := db.TenantFrom(s.ctx)
	return id
}

func (s *seeder) do(fn func(q *dbgen.Queries) error) {
	s.t.Helper()
	if err := s.env.DB.InTenant(s.ctx, fn); err != nil {
		s.t.Fatal(err)
	}
}

func (s *seeder) content(typ string, slug *string, parent *int64) dbgen.Content {
	s.t.Helper()
	var c dbgen.Content
	s.do(func(q *dbgen.Queries) (err error) {
		c, err = q.CreateContent(s.ctx, dbgen.CreateContentParams{Type: typ, Slug: slug, ParentID: parent, AuthorID: &s.author})
		return err
	})
	return c
}

func (s *seeder) revision(contentID int64, title, body string) dbgen.ContentRevision {
	s.t.Helper()
	var r dbgen.ContentRevision
	s.do(func(q *dbgen.Queries) (err error) {
		r, err = q.CreateRevision(s.ctx, dbgen.CreateRevisionParams{
			ContentID: contentID, Title: title, Body: []byte(body), BodySchemaVersion: 1,
			Fields: []byte("{}"), CreatedBy: &s.author,
		})
		return err
	})
	return r
}

// publish makes rev live with a fixed publication date.
func (s *seeder) publish(contentID, revID int64, at time.Time) {
	s.t.Helper()
	s.do(func(q *dbgen.Queries) error {
		_, err := q.PublishRevision(s.ctx, dbgen.PublishRevisionParams{ContentID: contentID, RevisionID: &revID})
		return err
	})
	s.admin("UPDATE contents SET published_at = $1 WHERE id = $2", at, contentID)
}

func (s *seeder) unpublish(contentID int64) {
	s.t.Helper()
	s.admin("UPDATE contents SET published_revision_id = NULL WHERE id = $1", contentID)
}

func (s *seeder) admin(sql string, args ...any) {
	s.t.Helper()
	if _, err := s.env.Admin.Exec(context.Background(), sql, args...); err != nil {
		s.t.Fatalf("%s: %v", sql, err)
	}
}

func (s *seeder) route(path, kind string, contentID, categoryID *int64) {
	s.t.Helper()
	s.do(func(q *dbgen.Queries) error {
		_, err := q.CreateRoute(s.ctx, dbgen.CreateRouteParams{Path: path, Kind: kind, ContentID: contentID, CategoryID: categoryID})
		return err
	})
}

func ptr[T any](v T) *T { return &v }

func paragraph(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":` + strconv.Quote(text) + `}]}]}`
}

// fixture is tenant A's seeded site.
type fixture struct {
	a, b          *seeder
	opts          build.Options
	founding      dbgen.Content // published post with category, tag and image
	edited        dbgen.Content // published, with a newer unpublished draft
	editedDraft   dbgen.ContentRevision
	draftOnly     dbgen.Content
	unpublished   dbgen.Content
	company       dbgen.Content
	access        dbgen.Content // child of company
	history       dbgen.Content // has a Japanese canonical route
	extraPosts    int
	base          time.Time
	siteDirA      string
	siteDirB      string
	mediaContents []byte
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	env := dbtest.New(t)
	f := &fixture{
		a:    newSeeder(t, env, "株式会社サンプル", "alice@example.com"),
		b:    newSeeder(t, env, "Other Co", "bob@example.com"),
		base: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
	}
	out, mediaRoot := t.TempDir(), t.TempDir()
	f.opts = build.Options{OutDir: out, MediaRoot: mediaRoot, BaseURL: "https://example.jp/"}
	f.siteDirA = filepath.Join(out, f.a.tenant().String())
	f.siteDirB = filepath.Join(out, f.b.tenant().String())
	a := f.a

	// Media: one file on disk for tenant A.
	f.mediaContents = []byte("fake jpeg")
	mediaPath := filepath.Join(mediaRoot, a.tenant().String(), "2026", "01", "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, f.mediaContents, 0o644); err != nil {
		t.Fatal(err)
	}
	var media dbgen.Medium
	var cat dbgen.Category
	var tag dbgen.Tag
	a.do(func(q *dbgen.Queries) (err error) {
		if media, err = q.CreateMedia(a.ctx, dbgen.CreateMediaParams{
			StorageKey: "2026/01/photo.jpg", Filename: "photo.jpg", ContentType: "image/jpeg",
			SizeBytes: 9, Width: ptr[int64](1200), Height: ptr[int64](800), AltText: "社屋",
		}); err != nil {
			return err
		}
		if cat, err = q.CreateCategory(a.ctx, dbgen.CreateCategoryParams{Name: "ニュース", Slug: "news"}); err != nil {
			return err
		}
		tag, err = q.CreateTag(a.ctx, dbgen.CreateTagParams{Name: "イベント", Slug: "event"})
		return err
	})

	f.founding = a.content("post", ptr("founding"), nil)
	rev := a.revision(f.founding.ID, "創業のお知らせ", fmt.Sprintf(
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"創業しました。"}]},`+
			`{"type":"image","attrs":{"src":"/media/2026/01/photo.jpg","alt":"社屋の外観","mediaId":%d}}]}`, media.ID))
	a.publish(f.founding.ID, rev.ID, f.base.Add(48*time.Hour))
	a.do(func(q *dbgen.Queries) error {
		if err := q.AddContentCategory(a.ctx, dbgen.AddContentCategoryParams{ContentID: f.founding.ID, CategoryID: cat.ID}); err != nil {
			return err
		}
		return q.AddContentTag(a.ctx, dbgen.AddContentTagParams{ContentID: f.founding.ID, TagID: tag.ID})
	})

	f.edited = a.content("post", nil, nil)
	rev = a.revision(f.edited.ID, "公開中のタイトル", paragraph("公開中の本文"))
	a.publish(f.edited.ID, rev.ID, f.base.Add(24*time.Hour))
	f.editedDraft = a.revision(f.edited.ID, "編集中のタイトル", paragraph("編集中の本文"))

	f.draftOnly = a.content("post", ptr("draft"), nil)
	a.revision(f.draftOnly.ID, "下書きだけの記事", paragraph("下書きだけの本文"))

	f.unpublished = a.content("post", ptr("withdrawn"), nil)
	rev = a.revision(f.unpublished.ID, "取り下げた記事", paragraph("取り下げた本文"))
	a.publish(f.unpublished.ID, rev.ID, f.base)
	a.unpublish(f.unpublished.ID)

	// Enough older posts to need a second list page.
	f.extraPosts = build.PostsPerPage
	for i := range f.extraPosts {
		c := a.content("post", ptr(fmt.Sprintf("old-%02d", i)), nil)
		rev := a.revision(c.ID, fmt.Sprintf("過去の記事%02d", i), paragraph("過去の本文"))
		a.publish(c.ID, rev.ID, f.base.Add(-time.Duration(i+1)*time.Hour))
	}

	f.company = a.content("page", ptr("company"), nil)
	rev = a.revision(f.company.ID, "会社概要", paragraph("会社概要の本文"))
	a.publish(f.company.ID, rev.ID, f.base)
	f.access = a.content("page", ptr("access"), &f.company.ID)
	rev = a.revision(f.access.ID, "アクセス", paragraph("駅から徒歩5分"))
	a.publish(f.access.ID, rev.ID, f.base)
	f.history = a.content("page", ptr("history"), nil)
	rev = a.revision(f.history.ID, "沿革", paragraph("沿革の本文"))
	a.publish(f.history.ID, rev.ID, f.base)

	a.route("/沿革/", "canonical", &f.history.ID, nil)
	a.route("/old-company/", "redirect", &f.company.ID, nil)
	a.route("/旧ページ/", "redirect", &f.founding.ID, nil)
	a.route("/blog/category/news/", "redirect", nil, &cat.ID)
	a.route("/gone/", "redirect", &f.draftOnly.ID, nil) // target not public: no redirect

	// Tenant B: content that must never show up in A's site.
	b := f.b
	secret := b.content("post", ptr("secret"), nil)
	rev = b.revision(secret.ID, "B社の秘密", paragraph("B社の秘密の本文"))
	b.publish(secret.ID, rev.ID, f.base)
	return f
}

func (f *fixture) build(t *testing.T, s *seeder) build.Result {
	t.Helper()
	res, err := build.Build(context.Background(), f.a.env.DB, s.tenant(), f.opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// siteFiles lists the files of a built site as public paths.
func siteFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files["/"+filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestBuild(t *testing.T) {
	f := newFixture(t)
	res := f.build(t, f.a)
	files := siteFiles(t, f.siteDirA)

	edited := "/posts/" + strconv.FormatInt(f.edited.Number, 10) + "/index.html"
	wantFiles := []string{
		"/404.html",
		"/_assets/style.css",
		"/_redirects.nginx",
		"/blog/category/news/index.html",
		"/company/access/index.html",
		"/company/index.html",
		"/feed.xml",
		"/index.html",
		"/media/2026/01/photo.jpg",
		"/old-company/index.html",
		"/posts/category/news/index.html",
		"/posts/founding/index.html",
		"/posts/index.html",
		"/posts/page/2/index.html",
		"/posts/tag/event/index.html",
		edited,
		"/sitemap.xml",
		"/旧ページ/index.html",
		"/沿革/index.html",
	}
	for i := range f.extraPosts {
		wantFiles = append(wantFiles, fmt.Sprintf("/posts/old-%02d/index.html", i))
	}
	slices.Sort(wantFiles)
	gotFiles := slices.Sorted(func(yield func(string) bool) {
		for p := range files {
			if !yield(p) {
				return
			}
		}
	})
	if diff := cmp.Diff(wantFiles, gotFiles); diff != "" {
		t.Errorf("files (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantFiles, res.Changed); diff != "" {
		t.Errorf("first build Changed (-want +got):\n%s", diff)
	}
	if len(res.Removed) != 0 || res.Files != len(wantFiles) {
		t.Errorf("Removed = %v, Files = %d", res.Removed, res.Files)
	}

	t.Run("only published revisions are rendered", func(t *testing.T) {
		if !strings.Contains(files[edited], "公開中のタイトル") || !strings.Contains(files[edited], "公開中の本文") {
			t.Errorf("published revision missing from %s", edited)
		}
		for p, body := range files {
			for _, secret := range []string{"編集中", "下書きだけ", "取り下げた", "/posts/draft/", "/posts/withdrawn/"} {
				if strings.Contains(body, secret) {
					t.Errorf("%s contains %q", p, secret)
				}
			}
		}
	})

	t.Run("other tenants never appear", func(t *testing.T) {
		for p, body := range files {
			if strings.Contains(body, "B社") || strings.Contains(body, "secret") || strings.Contains(body, "Other Co") {
				t.Errorf("%s contains tenant B content", p)
			}
		}
		if _, err := os.Stat(f.siteDirB); err == nil {
			t.Errorf("building A wrote B's directory")
		}
	})

	t.Run("pages", func(t *testing.T) {
		post := files["/posts/founding/index.html"]
		for _, want := range []string{
			`<html lang="ja">`,
			`<title>創業のお知らせ | 株式会社サンプル</title>`,
			`<a class="skip-link" href="#main">本文へ移動</a>`,
			`<main id="main"`,
			`<h1>創業のお知らせ</h1>`,
			`<time datetime="2026-01-12">2026年1月12日</time>`,
			`<a class="badge" href="/posts/category/news/">ニュース</a>`,
			`<a href="/posts/tag/event/">#イベント</a>`,
			`<img src="/media/2026/01/photo.jpg" alt="社屋の外観" width="1200" height="800"`,
			`<link rel="canonical" href="https://example.jp/posts/founding/">`,
			`<a href="/company/">会社概要</a>`,
			`<a href="/posts/" aria-current="true">お知らせ</a>`,
		} {
			if !strings.Contains(post, want) {
				t.Errorf("post page lacks %s", want)
			}
		}
		if strings.Contains(post, `href="/company/access/"`) {
			t.Errorf("child page in the top menu")
		}
		page := files["/沿革/index.html"]
		if !strings.Contains(page, `<link rel="canonical" href="https://example.jp/%E6%B2%BF%E9%9D%A9/">`) ||
			!strings.Contains(page, `<a href="/%e6%b2%bf%e9%9d%a9/" aria-current="page">沿革</a>`) {
			t.Errorf("japanese page links wrong:\n%s", page)
		}
		access := files["/company/access/index.html"]
		if !strings.Contains(access, `<a href="/company/">会社概要</a></li>`) || !strings.Contains(access, `<span aria-current="page">アクセス</span>`) {
			t.Errorf("breadcrumbs of child page wrong:\n%s", access)
		}
		home := files["/index.html"]
		if !strings.Contains(home, `<h1 class="hero__title">株式会社サンプル</h1>`) || !strings.Contains(home, "創業のお知らせ") {
			t.Errorf("home page wrong:\n%s", home)
		}
		list2 := files["/posts/page/2/index.html"]
		if !strings.Contains(list2, `<title>お知らせ（2ページ目）`) || !strings.Contains(list2, `rel="prev"`) {
			t.Errorf("second list page wrong:\n%s", list2)
		}
		notFound := files["/404.html"]
		if !strings.Contains(notFound, `<meta name="robots" content="noindex">`) || !strings.Contains(notFound, "ページが見つかりません") {
			t.Errorf("404 page wrong:\n%s", notFound)
		}
		if files["/media/2026/01/photo.jpg"] != string(f.mediaContents) {
			t.Errorf("media not copied")
		}
	})

	t.Run("redirects", func(t *testing.T) {
		want := "# Generated by Pagewright. Do not edit.\n" +
			"location = \"/blog/category/news/\" { return 301 \"/posts/category/news/\"; }\n" +
			"location = \"/old-company/\" { return 301 \"/company/\"; }\n" +
			"location = \"/旧ページ/\" { return 301 \"/posts/founding/\"; }\n"
		if diff := cmp.Diff(want, files["/_redirects.nginx"]); diff != "" {
			t.Errorf("_redirects.nginx (-want +got):\n%s", diff)
		}
		page := files["/old-company/index.html"]
		if !strings.Contains(page, `<meta http-equiv="refresh" content="0; url=/company/">`) {
			t.Errorf("redirect page wrong:\n%s", page)
		}
	})

	t.Run("sitemap", func(t *testing.T) {
		var set struct {
			URLs []struct {
				Loc     string `xml:"loc"`
				Lastmod string `xml:"lastmod"`
			} `xml:"url"`
		}
		if err := xml.Unmarshal([]byte(files["/sitemap.xml"]), &set); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, u := range set.URLs {
			got = append(got, u.Loc)
		}
		want := []string{
			"https://example.jp/",
			"https://example.jp/company/",
			"https://example.jp/company/access/",
			"https://example.jp/posts/",
			"https://example.jp/posts/" + strconv.FormatInt(f.edited.Number, 10) + "/",
			"https://example.jp/posts/category/news/",
			"https://example.jp/posts/founding/",
		}
		for i := range f.extraPosts {
			want = append(want, fmt.Sprintf("https://example.jp/posts/old-%02d/", i))
		}
		want = append(want, "https://example.jp/posts/tag/event/", "https://example.jp/%E6%B2%BF%E9%9D%A9/")
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("sitemap (-want +got):\n%s", diff)
		}
	})

	t.Run("feed", func(t *testing.T) {
		var rss struct {
			Channel struct {
				Title string `xml:"title"`
				Items []struct {
					Title   string `xml:"title"`
					Link    string `xml:"link"`
					PubDate string `xml:"pubDate"`
				} `xml:"item"`
			} `xml:"channel"`
		}
		if err := xml.Unmarshal([]byte(files["/feed.xml"]), &rss); err != nil {
			t.Fatal(err)
		}
		if rss.Channel.Title != "株式会社サンプル" || len(rss.Channel.Items) != 2+f.extraPosts {
			t.Fatalf("channel %q with %d items", rss.Channel.Title, len(rss.Channel.Items))
		}
		first := rss.Channel.Items[0]
		if first.Title != "創業のお知らせ" || first.Link != "https://example.jp/posts/founding/" || first.PubDate != "Mon, 12 Jan 2026 00:00:00 +0000" {
			t.Errorf("first item = %+v", first)
		}
		if rss.Channel.Items[1].Title != "公開中のタイトル" {
			t.Errorf("second item = %+v", rss.Channel.Items[1])
		}
	})

	t.Run("rebuild without changes changes nothing", func(t *testing.T) {
		res := f.build(t, f.a)
		if len(res.Changed) != 0 || len(res.Removed) != 0 {
			t.Errorf("Changed = %v, Removed = %v", res.Changed, res.Removed)
		}
	})

	t.Run("publishing and unpublishing", func(t *testing.T) {
		f.a.publish(f.edited.ID, f.editedDraft.ID, f.base.Add(24*time.Hour))
		f.a.unpublish(f.founding.ID)
		res := f.build(t, f.a)
		for _, p := range []string{edited, "/posts/index.html", "/feed.xml"} {
			if !slices.Contains(res.Changed, p) {
				t.Errorf("Changed lacks %s: %v", p, res.Changed)
			}
		}
		wantRemoved := []string{
			"/media/2026/01/photo.jpg",
			"/posts/founding/index.html",
			"/旧ページ/index.html", // redirect to an unpublished post
		}
		if diff := cmp.Diff(wantRemoved, res.Removed); diff != "" {
			t.Errorf("Removed (-want +got):\n%s", diff)
		}
		b, err := os.ReadFile(filepath.Join(f.siteDirA, filepath.FromSlash(edited)))
		if err != nil || !strings.Contains(string(b), "編集中の本文") {
			t.Errorf("newly published revision not rendered: %v", err)
		}
		if _, err := os.Stat(filepath.Join(f.siteDirA, "posts", "founding")); err == nil {
			t.Errorf("empty directory of removed page left behind")
		}
	})

	t.Run("second tenant", func(t *testing.T) {
		f.build(t, f.b)
		files := siteFiles(t, f.siteDirB)
		if !strings.Contains(files["/posts/secret/index.html"], "B社の秘密") {
			t.Errorf("tenant B post missing")
		}
		for p, body := range files {
			if strings.Contains(body, "株式会社サンプル") || strings.Contains(body, "会社概要") {
				t.Errorf("%s of tenant B contains tenant A content", p)
			}
		}
	})
}

func TestBuildRequiresTenant(t *testing.T) {
	if _, err := build.Build(context.Background(), nil, uuid.Nil, build.Options{OutDir: t.TempDir()}); err == nil {
		t.Error("Build without tenant succeeded")
	}
}
