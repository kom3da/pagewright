package build

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/render"
)

// PreviewOptions configures Preview.
type PreviewOptions struct {
	// PublicBaseURL is the public site; links on the page point there.
	PublicBaseURL string
	// SelfURL is the preview page's URL (ending in /). Its _assets/ and
	// media/ serve the stylesheet and images.
	SelfURL string
	// FormBaseURL is as in Options.
	FormBaseURL string
	// Notice is shown at the top of the page.
	Notice string
}

// RenderPreview renders one content with its latest revision, as the site
// would show it once published, for a preview link.
func RenderPreview(ctx context.Context, d *db.DB, tenantID uuid.UUID, contentID int64, opts PreviewOptions) ([]byte, error) {
	ctx = db.WithTenant(ctx, tenantID)
	snap, err := load(ctx, d)
	if err != nil {
		return nil, err
	}
	th, err := pickTheme(nil, snap.tenant.Settings)
	if err != nil {
		return nil, err
	}
	var c dbgen.Content
	var rev dbgen.ContentRevision
	err = d.InTenant(ctx, func(q *dbgen.Queries) error {
		var err error
		if c, err = q.GetContent(ctx, contentID); err != nil {
			return err
		}
		rev, err = q.GetLatestRevision(ctx, contentID)
		return err
	})
	if err != nil {
		return nil, err
	}

	base := strings.TrimSuffix(opts.PublicBaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	g := &generator{
		snap: snap, opts: Options{BaseURL: base, FormBaseURL: opts.FormBaseURL}, tenantID: tenantID, theme: th,
		warn: func(string, ...any) {},
	}
	g.prepare()
	g.index()
	g.buildSite()
	g.site.StylesheetURL = opts.SelfURL + strings.TrimPrefix(th.css, "/")
	if key := g.logoKey; key != "" {
		g.site.LogoURL = opts.SelfURL + "media/" + key
	}

	publishedAt := time.Now()
	if c.PublishedAt != nil {
		publishedAt = *c.PublishedAt
	}
	path, parents, ok := g.rulePath(c.ID)
	if it, published := g.itemByID[c.ID]; published {
		path, parents, ok = it.path, it.parents, true
	}
	if !ok {
		path = "/"
	}
	body, err := render.Body(rev.Body, render.Options{
		ContactForm: g.contactForm(path),
		PagePath:    path,
		MediaPrefix: opts.SelfURL + "media/",
		TableLabel:  th.manifest.Labels.Table,
	})
	if err != nil {
		return nil, fmt.Errorf("render preview: %w", err)
	}
	it := &item{
		row: dbgen.ListPublishedContentsRow{
			ID: c.ID, Number: c.Number, Type: c.Type, Slug: c.Slug, ParentID: c.ParentID,
			PublishedAt: &publishedAt, RevisionID: rev.ID, Title: rev.Title, Body: rev.Body,
			BodySchemaVersion: rev.BodySchemaVersion, Fields: rev.Fields, RevisionCreatedAt: rev.CreatedAt,
		},
		path: path, body: body, parents: parents,
	}
	pg := g.page(c.Type, path)
	pg.Title = g.title(it)
	pg.Content = g.content(it)
	pg.NoIndex = true
	pg.SelfURL = opts.SelfURL
	crumbs := []Link{{Title: th.manifest.Labels.Home, URL: "/"}}
	for _, pid := range parents {
		if p, ok := g.itemByID[pid]; ok {
			crumbs = append(crumbs, Link{Title: g.title(p), URL: p.path})
		}
	}
	pg.Breadcrumbs = append(crumbs, Link{Title: pg.Title})
	b, err := th.render("single", pg)
	if err != nil {
		return nil, err
	}
	// The core marks previews, whatever the theme: links resolve on the
	// public site, and a notice says what this is.
	notice := `<div role="note" style="padding:0.6rem 1rem;background:#fff4ce;color:#1c2430;border-bottom:2px solid #9a6700;font:700 15px/1.6 system-ui,sans-serif;text-align:center">` +
		template.HTMLEscapeString(opts.Notice) + `</div>`
	return inject(b, `<base href="`+template.HTMLEscapeString(base+"/")+`">`, notice, ""), nil
}

// ThemeAsset returns a file of the assets/ directory of the theme the
// tenant's site uses.
func ThemeAsset(ctx context.Context, d *db.DB, tenantID uuid.UUID, name string) ([]byte, bool) {
	var settings []byte
	err := d.InTenant(db.WithTenant(ctx, tenantID), func(q *dbgen.Queries) error {
		row, err := q.GetSite(ctx)
		settings = row.Settings
		return err
	})
	if err != nil {
		return nil, false
	}
	th, err := pickTheme(nil, settings)
	if err != nil {
		return nil, false
	}
	b, ok := th.assets[name]
	return b, ok
}
