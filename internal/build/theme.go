package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kom3da/pagewright/internal/render"
	"github.com/kom3da/pagewright/internal/site"
	"github.com/kom3da/pagewright/themes"
)

// The data a theme template receives. Templates are executed with a *Page.

// Site is shared by every page of a build.
type Site struct {
	Name          string
	BaseURL       string // no trailing slash
	Menu          []Link // configured, or top-level pages then the post list
	PostsURL      string // first page of the post list
	FeedURL       string
	StylesheetURL string // with a content hash for cache busting
	// SearchURL is the search page, or empty when search is off.
	SearchURL string
	// ContactURL is the page with the contact form, or empty.
	ContactURL string
	// PrivacyURL is the page about personal data chosen in the settings,
	// or empty.
	PrivacyURL string
	// Pages are the published pages without a parent, in number order,
	// except the front page.
	Pages []Summary
	// Theme holds the values of the settings the theme declares in
	// theme.json, by key: a string, or a *Link for page settings. Unset
	// values are absent.
	Theme map[string]any
	// LogoURL is the logo image, or empty to show the name as text.
	LogoURL string
	// StyleVars overrides theme CSS variables (colors, font) from the site
	// settings, e.g. "--color-accent: #0a4f8f;". Values are validated.
	StyleVars template.CSS
}

// Link is a titled URL path.
type Link struct {
	Title   string
	URL     string
	Current bool // in pagination: the page being shown
}

// Summary describes a published page for themes that list or feature
// pages; how to show it is up to the theme.
type Summary struct {
	Title string
	URL   string
	// Lead is the first paragraph, shortened; Excerpt is a shorter one.
	Lead, Excerpt string
	// Image is the first image of the body, if any.
	Image *render.Image
	// Rest is the body without the first paragraph and the first image,
	// for themes that show those as Lead and Image (pages only).
	Rest template.HTML
	// More reports that Rest has any text.
	More bool
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
	// Front is the page featured on the home page (Content then holds its
	// body), or nil.
	Front *Summary
	// SelfURL is this page's own URL on preview pages, which the core
	// serves under a <base> pointing at the public site: in-page links
	// should be written as {{.SelfURL}}#id. Empty elsewhere.
	SelfURL string
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
	manifest Manifest
	pages    map[string]*template.Template // one per page template, with base.html
	assets   map[string][]byte             // file name under assets/ → content
	css      string                        // URL of the main stylesheet
	// parts are pieces the core places inside bodies (templates/parts/),
	// e.g. "contact-form".
	parts *template.Template
}

// Manifest is a theme's theme.json. See docs/themes.md.
type Manifest struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Labels are the words the core puts into pages (breadcrumbs, list
	// headings, titles), so the core holds no wording of its own.
	Labels Labels `json:"labels"`
	// DateFormat is a Go time layout for the "date" function.
	DateFormat string `json:"dateFormat"`
	// Settings are extra fields the admin shows for this theme; their
	// values reach templates as .Site.Theme.<key>.
	Settings []SettingDecl `json:"settings"`
}

// SettingDecl declares one theme setting.
type SettingDecl struct {
	Key         string `json:"key"`         // letters and digits, starting with a letter
	Label       string `json:"label"`       // shown in the admin
	Description string `json:"description"` // optional help text
	// Type is "text" (one line), "tel", "url" or "page" (a published page).
	Type string `json:"type"`
	// MaxLength bounds text values (default 100).
	MaxLength int `json:"maxLength"`
}

// Labels are the theme's words for what the core generates.
type Labels struct {
	Home            string `json:"home"`            // breadcrumb root
	Posts           string `json:"posts"`           // the post list (menu, breadcrumbs, heading)
	Untitled        string `json:"untitled"`        // a content without a title
	CategoryHeading string `json:"categoryHeading"` // fmt layout with the category name: %s
	TagHeading      string `json:"tagHeading"`      // fmt layout with the tag name: %s
	PageHeading     string `json:"pageHeading"`     // fmt layout for page 2+ of a list: %s, %d
	Search          string `json:"search"`          // title of the search page
	NotFound        string `json:"notFound"`        // title of the 404 page
	FeedDescription string `json:"feedDescription"` // fmt layout with the site name: %s
}

// pageTemplates are the page templates every theme provides, each defining
// "main" for the frame "base" in templates/base.html.
var pageTemplates = []string{"home", "single", "list", "404", "search"}

// ContactForm is the data of the theme's "contact-form" part.
type ContactForm struct {
	Action     string // where the form posts
	Page       string // the public path of the page the form is on
	PrivacyURL string // the page about personal data, or empty
}

// part renders the named part; ok is false when the theme has none.
func (t *theme) part(name string, data any) (template.HTML, bool, error) {
	if t.parts == nil || t.parts.Lookup(name) == nil {
		return "", false, nil
	}
	var b bytes.Buffer
	if err := t.parts.ExecuteTemplate(&b, name, data); err != nil {
		return "", true, fmt.Errorf("render part %s: %w", name, err)
	}
	return template.HTML(b.String()), true, nil //nolint:gosec // html/template output
}

const assetsDir = "_assets"

// funcs are the template functions; "date" uses the theme's layout.
func funcs(dateFormat string) template.FuncMap {
	return template.FuncMap{
		// date formats a date with the theme's layout. Times arrive already
		// in the site's time zone.
		"date": func(t time.Time) string { return t.Format(dateFormat) },
		"isoDate": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		// initial is the first character of s, e.g. to stand in for a logo.
		"initial": func(s string) string {
			for _, r := range s {
				return string(r)
			}
			return ""
		},
		// current returns the aria-current value of a menu link on the page
		// at path: "page" for the page itself, "true" for its section.
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
}

var themeCache sync.Map // name → *loaded

type loaded struct {
	once sync.Once
	t    *theme
	err  error
}

// pickTheme is fsys when given, else the theme the site settings name.
func pickTheme(fsys fs.FS, settings []byte) (*theme, error) {
	if fsys != nil {
		return LoadTheme(fsys)
	}
	return themeNamed(site.Parse(settings).Theme)
}

// themeNamed loads a built-in theme once per process. An empty name is the
// default theme.
func themeNamed(name string) (*theme, error) {
	if name == "" {
		name = themes.Default
	}
	fsys, ok := themes.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("theme %q is not installed", name)
	}
	v, _ := themeCache.LoadOrStore(name, &loaded{})
	l := v.(*loaded)
	l.once.Do(func() { l.t, l.err = LoadTheme(fsys) })
	return l.t, l.err
}

// LoadTheme reads and checks a theme. Errors say what is missing in terms
// a theme author can act on.
func LoadTheme(fsys fs.FS) (*theme, error) {
	raw, err := fs.ReadFile(fsys, "theme.json")
	if err != nil {
		return nil, fmt.Errorf("theme: theme.json is missing: %w", err)
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("theme: theme.json: %w", err)
	}
	if err := m.check(); err != nil {
		return nil, err
	}
	t := &theme{manifest: m, pages: map[string]*template.Template{}, assets: map[string][]byte{}}
	fm := funcs(m.DateFormat)
	for _, kind := range pageTemplates {
		tmpl, err := template.New(kind).Funcs(fm).ParseFS(fsys, "templates/base.html", "templates/"+kind+".html")
		if err != nil {
			return nil, fmt.Errorf("theme %s: templates/%s.html: %w", m.Name, kind, err)
		}
		if tmpl.Lookup("base") == nil {
			return nil, fmt.Errorf("theme %s: templates/base.html must define \"base\"", m.Name)
		}
		if tmpl.Lookup("main") == nil {
			return nil, fmt.Errorf("theme %s: templates/%s.html must define \"main\"", m.Name, kind)
		}
		t.pages[kind] = tmpl
	}
	if parts, _ := fs.Glob(fsys, "templates/parts/*.html"); len(parts) > 0 {
		tmpl, err := template.New("parts").Funcs(fm).ParseFS(fsys, parts...)
		if err != nil {
			return nil, fmt.Errorf("theme %s: templates/parts: %w", m.Name, err)
		}
		t.parts = tmpl
	}
	entries, err := fs.ReadDir(fsys, "assets")
	if err != nil {
		return nil, fmt.Errorf("theme %s: assets/style.css is missing (no assets/ directory)", m.Name)
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
		return nil, fmt.Errorf("theme %s: assets/style.css is missing", m.Name)
	}
	sum := sha256.Sum256(css)
	t.css = "/" + assetsDir + "/style.css?v=" + hex.EncodeToString(sum[:4])
	return t, nil
}

func (m Manifest) check() error {
	if m.Name == "" || m.Title == "" {
		return errors.New("theme: theme.json needs name and title")
	}
	l := m.Labels
	for _, f := range []struct{ key, value string }{
		{"home", l.Home}, {"posts", l.Posts}, {"untitled", l.Untitled},
		{"categoryHeading", l.CategoryHeading}, {"tagHeading", l.TagHeading},
		{"pageHeading", l.PageHeading}, {"search", l.Search}, {"notFound", l.NotFound},
		{"feedDescription", l.FeedDescription},
	} {
		if f.value == "" {
			return fmt.Errorf("theme %s: theme.json labels.%s is missing", m.Name, f.key)
		}
	}
	if m.DateFormat == "" {
		return fmt.Errorf("theme %s: theme.json dateFormat is missing (a Go time layout, e.g. \"2006年1月2日\")", m.Name)
	}
	return m.checkSettings()
}

func (t *theme) render(kind string, p *Page) ([]byte, error) {
	var b bytes.Buffer
	if err := t.pages[kind].ExecuteTemplate(&b, "base", p); err != nil {
		return nil, fmt.Errorf("render %s %s: %w", kind, p.Path, err)
	}
	return b.Bytes(), nil
}

// inject adds core markup to a rendered page: after <head> (headStart) and
// before </body> (bodyEnd), so themes need not know about it.
func inject(page []byte, headStart, bodyStart, bodyEnd string) []byte {
	s := string(page)
	if headStart != "" {
		if i := strings.Index(s, "<head>"); i >= 0 {
			s = s[:i+len("<head>")] + "\n" + headStart + s[i+len("<head>"):]
		}
	}
	if bodyStart != "" {
		if loc := bodyTag.FindStringIndex(s); loc != nil {
			s = s[:loc[1]] + "\n" + bodyStart + s[loc[1]:]
		}
	}
	if bodyEnd != "" {
		if i := strings.LastIndex(s, "</body>"); i >= 0 {
			s = s[:i] + bodyEnd + "\n" + s[i:]
		}
	}
	return []byte(s)
}

var bodyTag = regexp.MustCompile(`<body[^>]*>`)
