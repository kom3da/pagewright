// Package wxrimport imports a WordPress export (WXR) into one tenant.
//
// The export is parsed into the intermediate model (internal/migrate/wxr),
// bodies are converted with internal/migrate/htmlpm, and rows are written
// through db.InTenant, so row level security confines everything to the
// tenant.
//
// Each term, attachment and post is written in its own transaction: one
// failing entry is reported and the others continue. Importing is
// idempotent: a post whose number (the WordPress post ID) already exists,
// a category or tag whose slug exists, and media whose storage key exists
// are reported as Existing and left untouched. A dry run does all of the
// work in one transaction (with a savepoint per entry) and rolls it back.
package wxrimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/migrate/htmlpm"
	"github.com/kom3da/pagewright/internal/migrate/model"
	"github.com/kom3da/pagewright/internal/migrate/wxr"
)

// SourceFormat is stored in contents.source_format with the original HTML.
const SourceFormat = "wordpress_html"

// Options control an import.
type Options struct {
	// AuthorID is an existing member of the tenant. All content gets this
	// author, and revisions and media are recorded as created by them.
	AuthorID uuid.UUID
	// DryRun does everything except commit (and except writing media
	// files), producing the same Report.
	DryRun bool
	// KeepURLs makes the old WordPress URL the canonical URL. Otherwise
	// Pagewright's URL rules apply and old URLs become redirects.
	KeepURLs bool
	// DownloadMedia downloads image attachments into
	// <MediaRoot>/<tenantID>/<storage key> and points body images at them.
	DownloadMedia bool
	MediaRoot     string
	// HTTPClient downloads media. Nil uses a client that reaches only
	// public addresses, with a 30 second timeout.
	HTTPClient *http.Client
}

var errDryRun = errors.New("dry run: roll back")

// Import reads a WXR export from r and imports it into the tenant. The
// error is for problems that stop the whole import (unreadable file,
// author not in the tenant, database unavailable); per-entry problems are
// in the Report.
func Import(ctx context.Context, d *db.DB, tenantID uuid.UUID, r io.Reader, opts Options) (Report, error) {
	exp, err := wxr.ReadAll(r)
	if err != nil {
		return Report{}, fmt.Errorf("read WXR: %w", err)
	}
	return ImportExport(ctx, d, tenantID, exp, opts)
}

// ImportExport imports an already parsed export.
func ImportExport(ctx context.Context, d *db.DB, tenantID uuid.UUID, exp *model.Export, opts Options) (Report, error) {
	ctx = db.WithTenant(ctx, tenantID)
	im := &importer{
		opts:       opts,
		tenantID:   tenantID,
		exp:        exp,
		rep:        &Report{DryRun: opts.DryRun, Site: exp.Site.Title, SiteURL: exp.Site.URL},
		client:     opts.HTTPClient,
		categories: map[string]int64{},
		tags:       map[string]int64{},
		media:      map[string]mediaRef{},
		mediaByKey: map[string]mediaRef{},
		keys:       map[string]int64{},
		alts:       map[string]string{},
		segments:   map[int64]string{},
	}
	if im.client == nil {
		im.client = publicClient()
	}
	if opts.DownloadMedia && opts.MediaRoot == "" && !opts.DryRun {
		return Report{}, errors.New("MediaRoot is required to download media")
	}

	if err := d.InTenant(ctx, func(q *dbgen.Queries) error {
		if _, err := q.GetCurrentTenant(ctx); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("site %s not found", tenantID)
			}
			return err
		}
		ok, err := q.IsMember(ctx, opts.AuthorID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("author %s is not a member of site %s", opts.AuthorID, tenantID)
		}
		return nil
	}); err != nil {
		return Report{}, err
	}

	if !opts.DryRun {
		im.unit = func(fn func(*dbgen.Queries) error) error { return d.InTenant(ctx, fn) }
		err := im.run(ctx)
		im.log(ctx)
		return *im.rep, err
	}
	err := d.InTenant(ctx, func(q *dbgen.Queries) error {
		im.unit = func(fn func(*dbgen.Queries) error) error {
			if err := q.Savepoint(ctx); err != nil {
				return err
			}
			if err := fn(q); err != nil {
				if rbErr := q.RollbackToSavepoint(ctx); rbErr != nil {
					return errors.Join(err, rbErr)
				}
				return err
			}
			return q.ReleaseSavepoint(ctx)
		}
		if err := im.run(ctx); err != nil {
			return err
		}
		return errDryRun
	})
	im.log(ctx)
	if errors.Is(err, errDryRun) {
		err = nil
	}
	return *im.rep, err
}

type importer struct {
	opts     Options
	tenantID uuid.UUID
	exp      *model.Export
	rep      *Report
	client   *http.Client
	// unit runs fn in its own transaction (or savepoint in a dry run).
	unit func(fn func(*dbgen.Queries) error) error

	categories map[string]int64    // slug → id
	tags       map[string]int64    // slug → id
	media      map[string]mediaRef // urlKey → media
	mediaByKey map[string]mediaRef // storage key → media
	keys       map[string]int64    // storage key → attachment ID
	alts       map[string]string   // urlKey → attachment alt
	segments   map[int64]string    // content number → last URL segment
	byID       map[int64]model.Item
}

func (im *importer) log(ctx context.Context) {
	r := im.rep
	slog.InfoContext(ctx, "WordPress import finished",
		"tenant", im.tenantID, "dryRun", r.DryRun,
		"posts", r.Posts.Created, "pages", r.Pages.Created,
		"existing", r.Posts.Existing+r.Pages.Existing,
		"failed", r.Posts.Failed+r.Pages.Failed,
		"warnings", r.Warnings())
}

func (im *importer) run(ctx context.Context) error {
	for _, a := range im.exp.Attachments {
		if a.Alt != "" {
			im.alts[urlKey(a.URL)] = a.Alt
		}
	}
	im.rep.Notes = append(im.rep.Notes, "作成者はすべて指定したユーザーにしました（WordPress の投稿者は引き継いでいません）")

	im.importCategories(ctx)
	im.importTags(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if im.opts.DownloadMedia {
		for _, a := range im.exp.Attachments {
			if err := ctx.Err(); err != nil {
				return err
			}
			im.importMedia(ctx, a)
		}
	} else if len(im.exp.Attachments) > 0 {
		im.rep.Notes = append(im.rep.Notes, fmt.Sprintf("画像 %d 件はダウンロードしていません。本文の画像は元のURLのままです", len(im.exp.Attachments)))
	}

	comments := 0
	for _, it := range im.orderedItems() {
		if err := ctx.Err(); err != nil {
			return err
		}
		comments += it.CommentCount
		im.importItem(ctx, it)
	}
	if comments > 0 {
		im.rep.Notes = append(im.rep.Notes, fmt.Sprintf("コメント %d 件は取り込んでいません（Pagewright にコメント機能はありません）", comments))
	}
	return nil
}

// orderedItems puts parents before their children.
func (im *importer) orderedItems() []model.Item {
	byID := map[int64]model.Item{}
	for _, it := range im.exp.Items {
		byID[it.SourceID] = it
	}
	depth := func(it model.Item) int {
		d := 0
		for seen := map[int64]bool{}; it.ParentID != 0 && !seen[it.ParentID]; d++ {
			seen[it.ParentID] = true
			parent, ok := byID[it.ParentID]
			if !ok {
				break
			}
			it = parent
		}
		return d
	}
	items := slices.Clone(im.exp.Items)
	slices.SortStableFunc(items, func(a, b model.Item) int { return depth(a) - depth(b) })
	return items
}

// skipReason tells why an item is not imported, or "" to import it.
func skipReason(it model.Item) string {
	if it.Type != model.TypePost && it.Type != model.TypePage {
		return "種類「" + string(it.Type) + "」は取り込みません"
	}
	switch it.Status {
	case model.StatusTrash:
		return "ゴミ箱に入っています"
	case model.StatusOther:
		return "状態「" + it.RawStatus + "」（自動保存など）は取り込みません"
	}
	return ""
}

func (im *importer) importItem(ctx context.Context, it model.Item) {
	e := Entry{Kind: string(it.Type), SourceID: it.SourceID, Title: it.Title}
	if reason := skipReason(it); reason != "" {
		e.Outcome, e.Reason = Skipped, reason
		im.rep.add(e)
		return
	}
	var st itemStats
	err := im.unit(func(q *dbgen.Queries) error {
		e.Warnings, st = nil, itemStats{}
		return im.writeItem(ctx, q, it, &e, &st)
	})
	if err != nil {
		e.Outcome, e.Reason = Failed, err.Error()
		e.Path = ""
		slog.WarnContext(ctx, "import item failed", "id", it.SourceID, "err", err)
	} else if e.Outcome == Created {
		im.rep.Published += st.published
		im.rep.Drafts += st.drafts
		im.rep.Scheduled += st.scheduled
		im.rep.Routes += st.routes
		im.rep.Redirects += st.redirects
	}
	im.rep.add(e)
}

type itemStats struct {
	published, drafts, scheduled, routes, redirects int
}

func (im *importer) writeItem(ctx context.Context, q *dbgen.Queries, it model.Item, e *Entry, st *itemStats) error {
	if _, err := q.GetContentByNumber(ctx, it.SourceID); err == nil {
		e.Outcome, e.Reason = Existing, "番号 "+strconv.FormatInt(it.SourceID, 10)+" のコンテンツがすでにあります"
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	typ := string(it.Type)
	var parentID *int64
	if it.ParentID != 0 {
		parent, err := q.GetContentByNumber(ctx, it.ParentID)
		switch {
		case err == nil && parent.Type == typ:
			parentID = &parent.ID
		case err == nil || errors.Is(err, pgx.ErrNoRows):
			e.warn(WarnParent, fmt.Sprintf("親（ID %d）が取り込まれていないため、最上位に置きました", it.ParentID), "")
		default:
			return err
		}
	}

	slug, err := im.slug(ctx, q, it, typ, parentID, e)
	if err != nil {
		return err
	}
	source := it.Body
	format := SourceFormat
	c, err := q.CreateImportedContent(ctx, dbgen.CreateImportedContentParams{
		Number:       it.SourceID,
		Type:         typ,
		Slug:         slug,
		ParentID:     parentID,
		AuthorID:     &im.opts.AuthorID,
		SourceFormat: &format,
		SourceBody:   &source,
	})
	if err != nil {
		return fmt.Errorf("create content: %w", err)
	}

	body, warns := htmlpm.Convert(im.rewriteMediaURLs(it.Body), htmlpm.Options{Image: im.imageInfo})
	e.Warnings = append(e.Warnings, warns...)
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}
	fieldsJSON, err := json.Marshal(im.fields(it, e))
	if err != nil {
		return err
	}
	rev, err := q.CreateRevision(ctx, dbgen.CreateRevisionParams{
		ContentID:         c.ID,
		Title:             it.Title,
		Body:              bodyJSON,
		BodySchemaVersion: htmlpm.SchemaVersion,
		Fields:            fieldsJSON,
		CreatedBy:         &im.opts.AuthorID,
	})
	if err != nil {
		return fmt.Errorf("create revision: %w", err)
	}
	if err := im.status(ctx, q, it, c.ID, rev.ID, e, st); err != nil {
		return err
	}
	if err := im.linkTerms(ctx, q, it, c.ID, e); err != nil {
		return err
	}
	seg := strconv.FormatInt(it.SourceID, 10)
	if slug != nil {
		seg = *slug
	}
	im.segments[it.SourceID] = seg
	if err := im.contentRoutes(ctx, q, it, c.ID, published(it), e, st); err != nil {
		return err
	}
	e.Outcome = Created
	return nil
}

func published(it model.Item) bool {
	return it.Status == model.StatusPublished && it.Password == ""
}

// slug returns the slug to store, or nil to fall back to the number.
func (im *importer) slug(ctx context.Context, q *dbgen.Queries, it model.Item, typ string, parentID *int64, e *Entry) (*string, error) {
	s := it.Slug
	switch {
	case s == "":
		return nil, nil
	case !validSlug(s):
		e.warn(WarnSlug, "スラッグ「"+s+"」は使えない形（数字だけ、または / を含む）のため、URL には番号を使います", "")
		return nil, nil
	}
	taken, err := q.ContentSlugTaken(ctx, dbgen.ContentSlugTakenParams{Type: typ, ParentID: parentID, Slug: &s})
	if err != nil {
		return nil, err
	}
	if taken {
		e.warn(WarnSlug, "スラッグ「"+s+"」は同じ階層のほかのコンテンツが使っているため、URL には番号を使います", "")
		return nil, nil
	}
	return &s, nil
}

func (im *importer) imageInfo(src string) htmlpm.ImageInfo {
	if key, ok := strings.CutPrefix(src, "/media/"); ok {
		if ref, ok := im.mediaByKey[key]; ok {
			id := ref.id
			return htmlpm.ImageInfo{MediaID: &id, Alt: ref.alt}
		}
	}
	return htmlpm.ImageInfo{Alt: im.alts[urlKey(src)]}
}

// fields keeps custom fields (ACF values and other postmeta) as they are.
// Keys starting with "_" are WordPress internals and are left out.
func (im *importer) fields(it model.Item, e *Entry) map[string]any {
	f := map[string]any{}
	for k, v := range it.Meta {
		if strings.HasPrefix(k, "_") {
			continue
		}
		f[k] = v
	}
	if it.Excerpt != "" {
		if _, taken := f["excerpt"]; !taken {
			f["excerpt"] = it.Excerpt
		}
	}
	if _, ok := it.Meta["_thumbnail_id"]; ok {
		e.warn(WarnField, "アイキャッチ画像は取り込んでいません", "")
	}
	return f
}

func (im *importer) status(ctx context.Context, q *dbgen.Queries, it model.Item, contentID, revID int64, e *Entry, st *itemStats) error {
	date := it.Date
	if date.IsZero() {
		date = time.Now().UTC()
	}
	switch it.Status {
	case model.StatusPublished:
		if it.Password != "" {
			e.warn(WarnStatus, "パスワードで保護された記事は、パスワードなしで公開されないよう下書きとして取り込みました", "")
			st.drafts++
			return nil
		}
		_, err := q.PublishImportedRevision(ctx, dbgen.PublishImportedRevisionParams{RevisionID: &revID, PublishedAt: &date, ContentID: contentID})
		st.published++
		return err
	case model.StatusScheduled:
		if !date.After(time.Now()) {
			e.warn(WarnStatus, "予約日時を過ぎた予約投稿のため、下書きとして取り込みました", "")
			st.drafts++
			return nil
		}
		_, err := q.ScheduleImportedRevision(ctx, dbgen.ScheduleImportedRevisionParams{RevisionID: &revID, ScheduledAt: &date, ContentID: contentID})
		st.scheduled++
		return err
	case model.StatusPrivate:
		e.warn(WarnStatus, "非公開（private）のため、下書きとして取り込みました", "")
	case model.StatusPending:
		e.warn(WarnStatus, "レビュー待ちのため、下書きとして取り込みました", "")
	}
	st.drafts++
	return nil
}

func (im *importer) linkTerms(ctx context.Context, q *dbgen.Queries, it model.Item, contentID int64, e *Entry) error {
	seen := map[string]bool{}
	for _, slug := range it.Categories {
		if seen["c:"+slug] {
			continue
		}
		seen["c:"+slug] = true
		id, ok := im.categories[slug]
		if !ok {
			e.warn(WarnTerm, "カテゴリ「"+slug+"」が取り込まれていないため、付けていません", "")
			continue
		}
		if err := q.AddContentCategory(ctx, dbgen.AddContentCategoryParams{ContentID: contentID, CategoryID: id}); err != nil {
			return fmt.Errorf("add category: %w", err)
		}
	}
	for _, slug := range it.Tags {
		if seen["t:"+slug] {
			continue
		}
		seen["t:"+slug] = true
		id, ok := im.tags[slug]
		if !ok {
			e.warn(WarnTerm, "タグ「"+slug+"」が取り込まれていないため、付けていません", "")
			continue
		}
		if err := q.AddContentTag(ctx, dbgen.AddContentTagParams{ContentID: contentID, TagID: id}); err != nil {
			return fmt.Errorf("add tag: %w", err)
		}
	}
	return nil
}

// pagewrightPath is the URL by Pagewright's rules: /posts/{slug or
// number}/ for posts, /{parent path}/{slug or number}/ for pages.
func (im *importer) pagewrightPath(it model.Item) string {
	if it.Type == model.TypePost {
		return pathOf("posts", im.segment(it.SourceID, it))
	}
	if im.byID == nil {
		im.byID = make(map[int64]model.Item, len(im.exp.Items))
		for _, x := range im.exp.Items {
			im.byID[x.SourceID] = x
		}
	}
	var segs []string
	seen := map[int64]bool{}
	for cur := it; ; {
		segs = append([]string{im.segment(cur.SourceID, cur)}, segs...)
		seen[cur.SourceID] = true
		parent, ok := im.byID[cur.ParentID]
		if cur.ParentID == 0 || !ok || seen[parent.SourceID] {
			break
		}
		cur = parent
	}
	return pathOf(segs...)
}

func (im *importer) segment(id int64, it model.Item) string {
	if s, ok := im.segments[id]; ok {
		return s
	}
	if validSlug(it.Slug) {
		return it.Slug
	}
	return strconv.FormatInt(id, 10)
}

// contentRoutes adds the canonical URL of a content and, when the old URL
// is not kept, a redirect from it.
func (im *importer) contentRoutes(ctx context.Context, q *dbgen.Queries, it model.Item, contentID int64, live bool, e *Entry, st *itemStats) error {
	pw := im.pagewrightPath(it)
	old, oldErr := oldPath(it.Link)
	if oldErr != nil && live {
		if errors.Is(oldErr, errQueryURL) {
			e.warn(WarnURL, "旧URL（"+it.Link+"）は ?p= のような形で、静的なサイトでは使えないため、"+pw+" にしました。旧URLからのリダイレクトも作れません", "")
		} else {
			e.warn(WarnURL, "旧URL（"+it.Link+"）を読み取れないため、"+pw+" にしました", "")
		}
	}
	target := dbgen.CreateRouteParams{ContentID: &contentID}
	var candidates []string
	if im.opts.KeepURLs && oldErr == nil {
		candidates = []string{old, pw}
	} else {
		candidates = []string{pw}
	}
	canonical, err := im.addRoute(ctx, q, "canonical", candidates, target, e)
	if err != nil {
		return err
	}
	if canonical != "" {
		e.Path = canonical
		st.routes++
	}
	if oldErr != nil || im.opts.KeepURLs || old == canonical {
		return nil
	}
	redirect, err := im.addRoute(ctx, q, "redirect", []string{old}, target, e)
	if err != nil {
		return err
	}
	if redirect != "" {
		st.redirects++
	}
	return nil
}

// addRoute creates a route at the first free candidate path and returns
// it, or "" (with a warning) when every candidate is taken or invalid.
func (im *importer) addRoute(ctx context.Context, q *dbgen.Queries, kind string, candidates []string, target dbgen.CreateRouteParams, e *Entry) (string, error) {
	for i, p := range candidates {
		if !validPath(p) {
			e.warn(WarnURL, "URL「"+p+"」は使えない文字を含むため、作りませんでした", "")
			continue
		}
		_, err := q.GetRouteByPath(ctx, p)
		if err == nil {
			msg := "URL「" + p + "」はほかのページが使っているため"
			if i+1 < len(candidates) {
				msg += "、代わりに「" + candidates[i+1] + "」を使います"
			} else if kind == "canonical" {
				msg += "、公開URLがありません。取り込み後にURLを設定してください"
			} else {
				msg += "、リダイレクトを作りませんでした"
			}
			e.warn(WarnURL, msg, "")
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		target.Path, target.Kind = p, kind
		if _, err := q.CreateRoute(ctx, target); err != nil {
			return "", fmt.Errorf("create route %s: %w", p, err)
		}
		return p, nil
	}
	return "", nil
}
