package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/kom3da/pagewright/themes/corporate"
)

// The data a theme template receives. Templates are executed with a *Page.

// Site is shared by every page of a build.
type Site struct {
	Name          string
	BaseURL       string // no trailing slash
	Menu          []Link // top-level pages, then お知らせ
	PostsURL      string // first page of the post list
	FeedURL       string
	StylesheetURL string // with a content hash for cache busting
	// LegacyRedirects is set when /_legacy.json maps WordPress query URLs
	// (/?p=123); the home page then forwards such visits.
	LegacyRedirects bool
	// SearchURL is the search page, or empty when search is off.
	SearchURL string
}

// Link is a titled URL path.
type Link struct {
	Title   string
	URL     string
	Current bool // in pagination: the page being shown
}

// Page is one HTML page.
type Page struct {
	Site *Site
	// Kind is home, post, page, posts, category, tag or 404.
	Kind        string
	Title       string // for <title>; empty on the home page
	Heading     string // h1 of list pages
	Path        string // public path, decoded
	Canonical   string // absolute URL
	Description string
	NoIndex     bool
	Breadcrumbs []Link // last one has no URL (the current page)
	Content     *Content
	Posts       []PostItem
	Pagination  *Pagination
}

// Content is a rendered post or page.
type Content struct {
	Type        string // post or page
	Title       string
	Body        template.HTML
	PublishedAt time.Time
	Categories  []Link
	Tags        []Link
}

// PostItem is a post in a list.
type PostItem struct {
	Title       string
	URL         string
	PublishedAt time.Time
	Categories  []Link
}

// Pagination links the pages of a list.
type Pagination struct {
	Current, Total int
	Prev, Next     string
	Pages          []Link
}

// theme is a parsed theme: one template set per page kind.
type theme struct {
	pages  map[string]*template.Template // home, single, list, 404
	assets map[string][]byte             // file name under assets/ → content
	css    string                        // URL of the main stylesheet
}

const assetsDir = "_assets"

var jst = time.FixedZone("JST", 9*60*60)

var themeFuncs = template.FuncMap{
	// date formats a date the Japanese way: 2026年11月27日.
	"date": func(t time.Time) string { return t.In(jst).Format("2006年1月2日") },
	"isoDate": func(t time.Time) string {
		return t.In(jst).Format("2006-01-02")
	},
	// current returns the aria-current value of a menu link on the page at
	// path: "page" for the page itself, "true" for its section.
	"current": func(path, link string) string {
		switch {
		case path == link:
			return "page"
		case link != "/" && strings.HasPrefix(path, link):
			return "true"
		}
		return ""
	},
}

var (
	corporateOnce  sync.Once
	corporateTheme *theme
	corporateErr   error
)

func defaultTheme() (*theme, error) {
	corporateOnce.Do(func() { corporateTheme, corporateErr = loadTheme(corporate.FS) })
	return corporateTheme, corporateErr
}

func loadTheme(fsys fs.FS) (*theme, error) {
	t := &theme{pages: map[string]*template.Template{}, assets: map[string][]byte{}}
	for _, kind := range []string{"home", "single", "list", "404", "search"} {
		tmpl, err := template.New(kind).Funcs(themeFuncs).ParseFS(fsys, "templates/base.html", "templates/"+kind+".html")
		if err != nil {
			return nil, fmt.Errorf("parse theme template %s: %w", kind, err)
		}
		t.pages[kind] = tmpl
	}
	entries, err := fs.ReadDir(fsys, "assets")
	if err != nil {
		return nil, fmt.Errorf("read theme assets: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := fs.ReadFile(fsys, "assets/"+e.Name())
		if err != nil {
			return nil, err
		}
		t.assets[e.Name()] = b
	}
	css, ok := t.assets["style.css"]
	if !ok {
		return nil, fmt.Errorf("theme has no assets/style.css")
	}
	sum := sha256.Sum256(css)
	t.css = "/" + assetsDir + "/style.css?v=" + hex.EncodeToString(sum[:4])
	return t, nil
}

func (t *theme) render(kind string, p *Page) ([]byte, error) {
	var b bytes.Buffer
	if err := t.pages[kind].ExecuteTemplate(&b, "base", p); err != nil {
		return nil, fmt.Errorf("render %s %s: %w", kind, p.Path, err)
	}
	return b.Bytes(), nil
}
