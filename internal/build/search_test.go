package build_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

func TestSearchIndex(t *testing.T) {
	bin, err := exec.LookPath("pagefind_extended")
	if err != nil {
		t.Skip("pagefind_extended is not installed (mise install adds it)")
	}
	env := dbtest.New(t)
	s := newSeeder(t, env, "検索株式会社", "owner@example.com")
	c := s.content("post", ptr("search"), nil)
	r := s.revision(c.ID, "検索できる記事", paragraph("日本語の全文検索を試します。"))
	s.publish(c.ID, r.ID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	opts := build.Options{OutDir: t.TempDir(), Pagefind: bin}
	res, err := build.Build(context.Background(), env.DB, s.tenant(), opts)
	if err != nil {
		t.Fatal(err)
	}
	files := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))
	for _, f := range []string{"/pagefind/pagefind.js", "/pagefind/pagefind-ui.js", "/search/index.html"} {
		if _, ok := files[f]; !ok {
			t.Errorf("%s missing; changed: %v", f, res.Changed)
		}
	}
	if !strings.Contains(files["/index.html"], `href="/search/"`) {
		t.Error("menu lacks the search link")
	}
	if !strings.Contains(files["/posts/search/index.html"], "data-pagefind-body") {
		t.Error("post page is not marked for indexing")
	}
	// The date and categories are not part of what search results quote.
	if !strings.Contains(files["/posts/search/index.html"], `class="entry-meta" data-pagefind-ignore`) {
		t.Error("post date row is indexed")
	}
	// A rebuild without changes changes nothing, index included.
	res, err = build.Build(context.Background(), env.DB, s.tenant(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 0 {
		t.Errorf("rebuild changed %v", res.Changed)
	}
}

func TestSearchFailureFallsBack(t *testing.T) {
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("no false binary")
	}
	env := dbtest.New(t)
	s := newSeeder(t, env, "検索株式会社", "owner@example.com")
	c := s.content("post", ptr("x"), nil)
	s.publish(c.ID, s.revision(c.ID, "記事", paragraph("本文")).ID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	opts := build.Options{OutDir: t.TempDir(), Pagefind: falseBin}
	res, err := build.Build(context.Background(), env.DB, s.tenant(), opts)
	if err != nil {
		t.Fatalf("build failed instead of falling back: %v", err)
	}
	files := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))
	if _, ok := files["/search/index.html"]; ok {
		t.Error("search page written although indexing failed")
	}
	if strings.Contains(files["/index.html"], `href="/search/"`) {
		t.Error("menu links to search although indexing failed")
	}
	if len(res.Warnings) == 0 {
		t.Error("no warning about the missing search")
	}
}
