package build

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/render"
)

// PostsPerPage is the length of post lists and archives.
const PostsPerPage = 10

// homePosts is how many posts the home page lists.
const homePosts = 5

// feedPosts is how many posts feed.xml carries.
const feedPosts = 20

// item is a published content ready to render.
type item struct {
	row     dbgen.ListPublishedContentsRow
	path    string
	body    render.Result
	cats    []Link
	tags    []Link
	parents []int64 // ancestor ids, root first
}

type generator struct {
	snap     *snapshot
	opts     Options
	tenantID uuid.UUID
	theme    *theme
	out      *output
	warn     func(format string, args ...any)

	site       *Site
	items      []*item // published, newest first
	itemByID   map[int64]*item
	itemByPath map[string]*item
	legacy     map[string]map[string]string // WordPress query param -> id -> path
	treeByID   map[int64]dbgen.ListContentTreeRow
	catPath    map[int64]string
	tagPath    map[int64]string
	postsOfCat map[int64][]*item
	postsOfTag map[int64][]*item
	sitemap    []sitemapURL
	redirects  []redirect
	loc        *time.Location // site time zone for dates
	logoKey    string         // storage key of the logo, copied with the media
}

type sitemapURL struct {
	path    string
	lastmod time.Time
}

type redirect struct{ from, to string }

func (g *generator) run() error {
	g.reserveSystemFiles()
	g.index()
	g.buildSite()
	g.collectLegacy()
	if err := g.writeAssets(); err != nil {
		return err
	}
	steps := []func() error{
		g.writeHome, g.writeContents, g.writePostList, g.writeArchives,
		g.write404, g.writeSearch, g.writeRedirects, g.writeSitemap, g.writeFeed, g.copyMedia,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) reserveSystemFiles() {
	for _, f := range []string{"404.html", "sitemap.xml", "feed.xml", redirectsFile, legacyFile} {
		g.out.reserve(f)
	}
	for name := range g.theme.assets {
		g.out.reserve(assetsDir + "/" + name)
	}
	if g.opts.Pagefind != "" {
		g.out.reserve(strings.TrimPrefix(searchPath, "/") + "index.html")
	}
}

// index works out the public path of everything and renders bodies.
func (g *generator) index() {
	s := g.snap
	g.treeByID = map[int64]dbgen.ListContentTreeRow{}
	for _, t := range s.tree {
		g.treeByID[t.ID] = t
	}

	// Canonical routes win; the MVP rule fills in what has none, unless the
	// computed path already belongs to something else.
	contentRoute := map[int64]string{}
	g.catPath, g.tagPath = map[int64]string{}, map[int64]string{}
	taken := map[string]bool{}
	for _, r := range s.routes {
		taken[r.Path] = true
		if r.Kind != "canonical" {
			continue
		}
		switch {
		case r.ContentID != nil:
			contentRoute[*r.ContentID] = r.Path
		case r.CategoryID != nil:
			g.catPath[*r.CategoryID] = r.Path
		case r.TagID != nil:
			g.tagPath[*r.TagID] = r.Path
		}
	}
	ruled := func(what, p string) (string, bool) {
		if taken[p] {
			g.warn("%s: computed URL %s is used by another route; not published", what, p)
			return "", false
		}
		taken[p] = true
		return p, true
	}
	for _, c := range s.categories {
		if _, ok := g.catPath[c.ID]; !ok {
			if p, ok := ruled("category "+c.Slug, postsBase+"category/"+c.Slug+"/"); ok {
				g.catPath[c.ID] = p
			}
		}
	}
	for _, t := range s.tags {
		if _, ok := g.tagPath[t.ID]; !ok {
			if p, ok := ruled("tag "+t.Slug, postsBase+"tag/"+t.Slug+"/"); ok {
				g.tagPath[t.ID] = p
			}
		}
	}

	catByID := map[int64]dbgen.Category{}
	for _, c := range s.categories {
		catByID[c.ID] = c
	}
	tagByID := map[int64]dbgen.Tag{}
	for _, t := range s.tags {
		tagByID[t.ID] = t
	}
	mediaByID := map[int64]dbgen.Medium{}
	for _, m := range s.media {
		mediaByID[m.ID] = m
	}
	renderOpts := render.Options{MediaSize: func(id int64) (int64, int64, bool) {
		m, ok := mediaByID[id]
		if !ok || m.Width == nil || m.Height == nil {
			return 0, 0, false
		}
		return *m.Width, *m.Height, true
	}}
	renderOpts.ContactFormAction = g.contactAction()

	g.itemByID, g.itemByPath = map[int64]*item{}, map[string]*item{}
	for _, row := range s.contents {
		what := fmt.Sprintf("%s %d", row.Type, row.Number)
		if row.Type != "post" && row.Type != "page" {
			g.warn("%s: unknown content type; not published", what)
			continue
		}
		if row.BodySchemaVersion != 1 {
			g.warn("%s: body schema version %d is not supported; not published", what, row.BodySchemaVersion)
			continue
		}
		it := &item{row: row}
		if p, ok := contentRoute[row.ID]; ok {
			it.path = p
		} else {
			p, parents, ok := g.rulePath(row.ID)
			if !ok {
				g.warn("%s: parent chain is broken; not published", what)
				continue
			}
			if it.path, ok = ruled(what, p); !ok {
				continue
			}
			it.parents = parents
		}
		if row.Type == "page" && it.parents == nil {
			_, it.parents, _ = g.rulePath(row.ID)
		}
		opts := renderOpts
		opts.PagePath = it.path
		body, err := render.Body(row.Body, opts)
		if err != nil {
			g.warn("%s: %v; not published", what, err)
			continue
		}
		for _, w := range body.Warnings {
			g.warn("%s: %s", what, w)
		}
		it.body = body
		g.items = append(g.items, it)
		g.itemByID[row.ID] = it
		g.itemByPath[it.path] = it
	}

	g.postsOfCat, g.postsOfTag = map[int64][]*item{}, map[int64][]*item{}
	for _, cc := range s.contentCats {
		it, ok := g.itemByID[cc.ContentID]
		c, cok := catByID[cc.CategoryID]
		p, pok := g.catPath[cc.CategoryID]
		if !ok || !cok || !pok {
			continue
		}
		it.cats = append(it.cats, Link{Title: c.Name, URL: p})
		if it.row.Type == "post" {
			g.postsOfCat[c.ID] = append(g.postsOfCat[c.ID], it)
		}
	}
	for _, ct := range s.contentTags {
		it, ok := g.itemByID[ct.ContentID]
		t, tok := tagByID[ct.TagID]
		p, pok := g.tagPath[ct.TagID]
		if !ok || !tok || !pok {
			continue
		}
		it.tags = append(it.tags, Link{Title: t.Name, URL: p})
		if it.row.Type == "post" {
			g.postsOfTag[t.ID] = append(g.postsOfTag[t.ID], it)
		}
	}
	// Lists keep the newest-first order of g.items.
	order := map[*item]int{}
	for i, it := range g.items {
		order[it] = i
	}
	for _, m := range []map[int64][]*item{g.postsOfCat, g.postsOfTag} {
		for _, list := range m {
			slices.SortFunc(list, func(a, b *item) int { return order[a] - order[b] })
		}
	}
}

// rulePath computes the MVP URL of a content: /posts/{seg}/ for posts and
// /{parent segs}/{seg}/ for pages. It also returns the ancestors.
func (g *generator) rulePath(id int64) (string, []int64, bool) {
	c, ok := g.treeByID[id]
	if !ok {
		return "", nil, false
	}
	if c.Type == "post" {
		return postsBase + segment(c.Slug, c.Number) + "/", nil, true
	}
	segs := []string{segment(c.Slug, c.Number)}
	var parents []int64
	for p := c.ParentID; p != nil; {
		if len(parents) > 32 {
			return "", nil, false
		}
		pc, ok := g.treeByID[*p]
		if !ok {
			return "", nil, false
		}
		segs = append(segs, segment(pc.Slug, pc.Number))
		parents = append(parents, pc.ID)
		p = pc.ParentID
	}
	slices.Reverse(segs)
	slices.Reverse(parents)
	return "/" + strings.Join(segs, "/") + "/", parents, true
}

func (g *generator) buildSite() {
	g.site = &Site{
		Name:          g.snap.tenant.Name,
		BaseURL:       g.opts.BaseURL,
		PostsURL:      postsBase,
		FeedURL:       "/feed.xml",
		StylesheetURL: g.theme.css,
	}
	if g.opts.Pagefind != "" {
		g.site.SearchURL = searchPath
	}
	g.applySettings()
	if len(g.snap.menu) > 0 {
		g.site.Menu = g.configuredMenu()
		return
	}
	var pages []*item
	for _, it := range g.items {
		if it.row.Type == "page" && it.row.ParentID == nil && it.path != "/" {
			pages = append(pages, it)
		}
	}
	slices.SortFunc(pages, func(a, b *item) int { return cmp.Compare(a.row.Number, b.row.Number) })
	for _, it := range pages {
		g.site.Menu = append(g.site.Menu, Link{Title: title(it), URL: it.path})
	}
	g.site.Menu = append(g.site.Menu, Link{Title: "お知らせ", URL: postsBase})
}

func title(it *item) string {
	if t := strings.TrimSpace(it.row.Title); t != "" {
		return t
	}
	return "（無題）"
}

// excerpt shortens text for meta descriptions.
func excerpt(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	r := []rune(text)
	return string(r[:n]) + "…"
}

func (g *generator) page(kind, p string) *Page {
	return &Page{Site: g.site, Kind: kind, Path: p, Canonical: absURL(g.opts.BaseURL, p)}
}

// writePage renders pg and lists it in the sitemap (first pages only).
func (g *generator) writePage(tmpl, who string, pg *Page, lastmod time.Time) error {
	rel, ok := fileForPath(pg.Path)
	if !ok {
		g.warn("%s: unsafe path %q; skipped", who, pg.Path)
		return nil
	}
	b, err := g.theme.render(tmpl, pg)
	if err != nil {
		return err
	}
	if err := g.out.write(rel, who, b); err != nil {
		g.warn("%v; skipped", err)
		return nil
	}
	if !pg.NoIndex && (pg.Pagination == nil || pg.Pagination.Current == 1) {
		g.sitemap = append(g.sitemap, sitemapURL{path: pg.Path, lastmod: lastmod})
	}
	return nil
}

func (g *generator) writeAssets() error {
	for name, b := range g.theme.assets {
		if err := g.out.write(assetsDir+"/"+name, systemOwner, b); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) posts() []*item {
	var out []*item
	for _, it := range g.items {
		if it.row.Type == "post" {
			out = append(out, it)
		}
	}
	return out
}

// contactAction is where contact forms post, or empty when forms are off.
func (g *generator) contactAction() string {
	if g.opts.FormBaseURL == "" {
		return ""
	}
	return g.opts.FormBaseURL + "/f/" + g.tenantID.String() + "/contact"
}

func (g *generator) postItems(list []*item) []PostItem {
	out := make([]PostItem, 0, len(list))
	for _, it := range list {
		out = append(out, PostItem{Title: title(it), URL: it.path, PublishedAt: it.row.PublishedAt.In(g.loc), Categories: it.cats})
	}
	return out
}

func (g *generator) content(it *item) *Content {
	return &Content{
		Type:        it.row.Type,
		Title:       title(it),
		Body:        it.body.HTML,
		PublishedAt: it.row.PublishedAt.In(g.loc),
		Categories:  it.cats,
		Tags:        it.tags,
	}
}

func (g *generator) writeHome() error {
	pg := g.page("home", "/")
	var lastmod time.Time
	if it := g.itemByPath["/"]; it != nil && it.row.Type == "page" {
		pg.Content = g.content(it)
		pg.Description = excerpt(it.body.Text, 120)
		lastmod = it.row.RevisionCreatedAt
	}
	posts := g.posts()
	pg.Posts = g.postItems(posts[:min(homePosts, len(posts))])
	return g.writePage("home", "home page", pg, lastmod)
}

func (g *generator) writeContents() error {
	home := Link{Title: "ホーム", URL: "/"}
	for _, it := range g.items {
		if it.path == "/" && it.row.Type == "page" {
			continue // rendered by writeHome
		}
		pg := g.page(it.row.Type, it.path)
		pg.Title = title(it)
		pg.Content = g.content(it)
		pg.Description = excerpt(it.body.Text, 120)
		crumbs := []Link{home}
		if it.row.Type == "post" {
			crumbs = append(crumbs, Link{Title: "お知らせ", URL: postsBase})
		}
		for _, pid := range it.parents {
			if p, ok := g.itemByID[pid]; ok {
				crumbs = append(crumbs, Link{Title: title(p), URL: p.path})
			}
		}
		pg.Breadcrumbs = append(crumbs, Link{Title: pg.Title})
		who := fmt.Sprintf("%s %d", it.row.Type, it.row.Number)
		if err := g.writePage("single", who, pg, it.row.RevisionCreatedAt); err != nil {
			return err
		}
	}
	return nil
}

// writeList writes a paginated list of posts at base.
func (g *generator) writeList(kind, base, heading, who string, crumbs []Link, list []*item) error {
	total := max(1, (len(list)+PostsPerPage-1)/PostsPerPage)
	for n := 1; n <= total; n++ {
		pg := g.page(kind, pagePath(base, n))
		pg.Heading = heading
		pg.Title = heading
		if n > 1 {
			pg.Title = fmt.Sprintf("%s（%dページ目）", heading, n)
		}
		pg.Breadcrumbs = append(slices.Clone(crumbs), Link{Title: heading})
		pg.Posts = g.postItems(list[(n-1)*PostsPerPage : min(n*PostsPerPage, len(list))])
		pag := &Pagination{Current: n, Total: total}
		if n > 1 {
			pag.Prev = pagePath(base, n-1)
		}
		if n < total {
			pag.Next = pagePath(base, n+1)
		}
		for i := 1; i <= total; i++ {
			pag.Pages = append(pag.Pages, Link{Title: strconv.Itoa(i), URL: pagePath(base, i), Current: i == n})
		}
		pg.Pagination = pag
		if err := g.writePage("list", who, pg, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) writePostList() error {
	return g.writeList("posts", postsBase, "お知らせ", "post list", []Link{{Title: "ホーム", URL: "/"}}, g.posts())
}

func (g *generator) writeArchives() error {
	crumbs := []Link{{Title: "ホーム", URL: "/"}, {Title: "お知らせ", URL: postsBase}}
	for _, c := range g.snap.categories {
		p, ok := g.catPath[c.ID]
		if !ok {
			continue
		}
		if err := g.writeList("category", p, "カテゴリ："+c.Name, "category "+c.Slug, crumbs, g.postsOfCat[c.ID]); err != nil {
			return err
		}
	}
	for _, t := range g.snap.tags {
		p, ok := g.tagPath[t.ID]
		if !ok {
			continue
		}
		if err := g.writeList("tag", p, "タグ："+t.Name, "tag "+t.Slug, crumbs, g.postsOfTag[t.ID]); err != nil {
			return err
		}
	}
	return nil
}

// writeSearch writes the search page when Pagefind is enabled.
func (g *generator) writeSearch() error {
	if g.site.SearchURL == "" {
		return nil
	}
	pg := g.page("search", searchPath)
	pg.Title = "サイト内検索"
	return g.writePage("search", systemOwner, pg, time.Time{})
}

func (g *generator) write404() error {
	pg := g.page("404", "/404.html")
	pg.Title = "ページが見つかりません"
	pg.NoIndex = true
	b, err := g.theme.render("404", pg)
	if err != nil {
		return err
	}
	return g.out.write("404.html", systemOwner, b)
}

func (g *generator) copyMedia() error {
	byID := map[int64]dbgen.Medium{}
	byKey := map[string]dbgen.Medium{}
	for _, m := range g.snap.media {
		byID[m.ID] = m
		byKey[m.StorageKey] = m
	}
	used := map[string]dbgen.Medium{}
	if m, ok := byKey[g.logoKey]; ok {
		used[m.StorageKey] = m
	}
	for _, it := range g.items {
		for _, id := range it.body.MediaIDs {
			if m, ok := byID[id]; ok {
				used[m.StorageKey] = m
			}
		}
		for _, k := range it.body.MediaKeys {
			if m, ok := byKey[k]; ok {
				used[m.StorageKey] = m
			} else {
				g.warn("%s %d: image /media/%s is not in the media library", it.row.Type, it.row.Number, k)
			}
		}
	}
	if len(used) == 0 {
		return nil
	}
	if g.opts.MediaRoot == "" {
		g.warn("MediaRoot is not set; %d media files not copied", len(used))
		return nil
	}
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range used {
			if !yield(k) {
				return
			}
		}
	})
	for _, k := range keys {
		if !validRel(k) {
			g.warn("media %d: unsafe storage key %q; skipped", used[k].ID, k)
			continue
		}
		src := filepath.Join(g.opts.MediaRoot, g.tenantID.String(), filepath.FromSlash(k))
		if err := g.out.link("media/"+k, "media "+strconv.FormatInt(used[k].ID, 10), src); err != nil {
			g.warn("media %d: %v; skipped", used[k].ID, err)
		}
	}
	return nil
}
