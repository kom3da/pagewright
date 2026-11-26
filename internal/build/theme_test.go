package build_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// TestBuildWithAnotherTheme builds with a minimal theme to make sure the
// core carries no wording or markup of the corporate theme.
func TestBuildWithAnotherTheme(t *testing.T) {
	env := dbtest.New(t)
	s := newSeeder(t, env, "Acme", "owner@example.com")
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	post := s.content("post", ptr("hello"), nil)
	s.publish(post.ID, s.revision(post.ID, "", paragraph("body")).ID, at)
	contact := s.content("page", ptr("contact"), nil)
	s.publish(contact.ID, s.revision(contact.ID, "Contact", `{"type":"doc","content":[{"type":"contactForm"}]}`).ID, at)
	cat := s.category("Updates", "updates")
	s.link(post.ID, cat)

	opts := build.Options{
		OutDir: t.TempDir(), ThemeFS: os.DirFS("testdata/minimal-theme"), FormBaseURL: "https://admin.example",
	}
	if _, err := build.Build(context.Background(), env.DB, s.tenant(), opts); err != nil {
		t.Fatal(err)
	}
	files := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))

	checks := map[string][]string{
		"/index.html":                        {"Jan 2, 2026", "(untitled)", ">News<"},
		"/posts/hello/index.html":            {"<li>Home</li>", "<li>News</li>"},
		"/posts/category/updates/index.html": {"Category: Updates"},
		"/contact/index.html":                {`class="minimal-form"`, `action="https://admin.example/f/`},
		"/feed.xml":                          {"News from Acme"},
	}
	for file, wants := range checks {
		for _, want := range wants {
			if !strings.Contains(files[file], want) {
				t.Errorf("%s lacks %q", file, want)
			}
		}
	}
	japanese := regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)
	corporate := regexp.MustCompile(`class="[^"]*\b(hero|nav-button|contact-form|form-field|site-mark|cta|eyebrow)\b`)
	for name, body := range files {
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".xml") {
			continue
		}
		if m := japanese.FindString(body); m != "" {
			t.Errorf("%s has Japanese text %q from the core", name, m)
		}
		if m := corporate.FindString(body); m != "" {
			t.Errorf("%s has corporate markup %q", name, m)
		}
	}
}

func TestLoadThemeErrors(t *testing.T) {
	base := func() fstest.MapFS {
		fsys := fstest.MapFS{}
		for _, f := range []string{"theme.json", "templates/base.html", "templates/home.html", "templates/single.html",
			"templates/list.html", "templates/404.html", "templates/search.html", "assets/style.css"} {
			b, err := os.ReadFile(filepath.Join("testdata/minimal-theme", f))
			if err != nil {
				t.Fatal(err)
			}
			fsys[f] = &fstest.MapFile{Data: b}
		}
		return fsys
	}
	if _, err := build.LoadTheme(base()); err != nil {
		t.Fatalf("minimal theme: %v", err)
	}
	tests := []struct {
		name string
		edit func(fstest.MapFS)
		want string
	}{
		{"no manifest", func(f fstest.MapFS) { delete(f, "theme.json") }, "theme.json is missing"},
		{"missing label", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `"search": "Search",`, "", 1))
		}, "labels.search is missing"},
		{"unknown key", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `"name"`, `"colour": "red", "name"`, 1))
		}, "colour"},
		{"missing page template", func(f fstest.MapFS) { delete(f, "templates/404.html") }, "templates/404.html"},
		{"no main", func(f fstest.MapFS) {
			f["templates/search.html"] = &fstest.MapFile{Data: []byte(`{{define "other"}}{{end}}`)}
		}, `templates/search.html must define "main"`},
		{"no stylesheet", func(f fstest.MapFS) { delete(f, "assets/style.css") }, "assets/style.css is missing"},
		{"bad setting type", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `"dateFormat"`, `"settings": [{"key": "phone", "label": "Phone", "type": "number"}], "dateFormat"`, 1))
		}, "type must be text, tel, url or page"},
		{"duplicate setting", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `"dateFormat"`, `"settings": [{"key": "a", "label": "A", "type": "text"}, {"key": "a", "label": "A2", "type": "text"}], "dateFormat"`, 1))
		}, "declared twice"},
	}
	for _, tt := range tests {
		fsys := base()
		tt.edit(fsys)
		_, err := build.LoadTheme(fsys)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want it to mention %q", tt.name, err, tt.want)
		}
	}
}

func (s *seeder) category(name, slug string) int64 {
	s.t.Helper()
	var id int64
	s.do(func(q *dbgen.Queries) error {
		c, err := q.CreateCategory(s.ctx, dbgen.CreateCategoryParams{Name: name, Slug: slug})
		id = c.ID
		return err
	})
	return id
}

func (s *seeder) link(contentID, categoryID int64) {
	s.t.Helper()
	s.do(func(q *dbgen.Queries) error {
		return q.AddContentCategory(s.ctx, dbgen.AddContentCategoryParams{ContentID: contentID, CategoryID: categoryID})
	})
}

func TestCheckThemeSettings(t *testing.T) {
	info := build.ThemeInfo{Settings: []build.SettingDecl{
		{Key: "phone", Label: "電話番号", Type: "tel"},
		{Key: "site", Label: "URL", Type: "url"},
		{Key: "note", Label: "メモ", Type: "text", MaxLength: 5},
		{Key: "about", Label: "紹介ページ", Type: "page"},
	}}
	got, err := info.CheckSettings(map[string]any{"phone": " 03-1234-5678 ", "site": "/about/", "note": "", "about": float64(12)})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"phone": "03-1234-5678", "site": "/about/", "about": int64(12)}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("CheckSettings (-want +got):\n%s", diff)
	}
	for _, bad := range []map[string]any{
		{"phone": "call me"}, {"site": "javascript:alert(1)"}, {"note": "123456"},
		{"note": "a\nb"}, {"about": "12"}, {"about": float64(1.5)}, {"color": "red"},
	} {
		if _, err := info.CheckSettings(bad); err == nil {
			t.Errorf("CheckSettings(%v) accepted", bad)
		}
	}
}

// TestContentRelations checks the parent, child and sibling pages and the
// neighboring posts given to themes, and the focusable table region.
func TestContentRelations(t *testing.T) {
	env := dbtest.New(t)
	s := newSeeder(t, env, "Acme", "owner@example.com")
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	page := func(slug, title string, parent *int64, body string) int64 {
		c := s.content("page", ptr(slug), parent)
		s.publish(c.ID, s.revision(c.ID, title, body).ID, at)
		return c.ID
	}
	table := `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}]}]}]}`
	about := page("about", "About", nil, paragraph("a"))
	page("history", "History", &about, table)
	page("access", "Access", &about, paragraph("c"))
	s.content("page", ptr("draft"), &about) // never published
	for i, slug := range []string{"first", "second", "third"} {
		c := s.content("post", ptr(slug), nil)
		s.publish(c.ID, s.revision(c.ID, slug, paragraph("p")).ID, at.Add(time.Duration(i)*time.Hour))
	}

	opts := build.Options{OutDir: t.TempDir(), ThemeFS: os.DirFS("testdata/minimal-theme")}
	if _, err := build.Build(context.Background(), env.DB, s.tenant(), opts); err != nil {
		t.Fatal(err)
	}
	files := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))
	checks := map[string][]string{
		"/about/index.html": {`<ul class="children"><li><a href="/about/history/">History</a></li><li><a href="/about/access/">Access</a></li></ul>`},
		"/about/history/index.html": {
			`<p class="parent"><a href="/about/">About</a></p>`,
			`<ul class="siblings"><li>History</li><li><a href="/about/access/">Access</a></li></ul>`,
			`<div class="table-wrap" role="region" tabindex="0" aria-label="Table"><table>`,
		},
		"/posts/second/index.html": {`<a class="older" rel="prev" href="/posts/first/">first</a>`, `<a class="newer" rel="next" href="/posts/third/">third</a>`},
	}
	for file, wants := range checks {
		for _, want := range wants {
			if !strings.Contains(files[file], want) {
				t.Errorf("%s lacks %q\n%s", file, want, files[file])
			}
		}
	}
	for file, unwanted := range map[string]string{
		"/about/index.html":        `class="siblings"`,
		"/posts/first/index.html":  `class="older"`,
		"/posts/third/index.html":  `class="newer"`,
		"/about/access/index.html": "draft",
	} {
		if strings.Contains(files[file], unwanted) {
			t.Errorf("%s has %q", file, unwanted)
		}
	}
}
