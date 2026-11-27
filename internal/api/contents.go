package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/body"
	"github.com/kom3da/pagewright/internal/content"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/routing"
)

type contentSummary struct {
	ID                    int64      `json:"id"`
	Number                int64      `json:"number"`
	Type                  string     `json:"type"`
	Title                 string     `json:"title"`
	Slug                  *string    `json:"slug"`
	Status                string     `json:"status"`
	HasUnpublishedChanges bool       `json:"hasUnpublishedChanges"`
	URL                   *string    `json:"url"`
	PublishedAt           *time.Time `json:"publishedAt"`
	ScheduledAt           *time.Time `json:"scheduledAt"`
	ScheduledRevisionID   *int64     `json:"scheduledRevisionId"`
	LatestRevisionID      int64      `json:"latestRevisionId"`
	ParentID              *int64     `json:"parentId"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

type revisionJSON struct {
	ID                int64           `json:"id"`
	Title             string          `json:"title"`
	Body              json.RawMessage `json:"body"`
	BodySchemaVersion int64           `json:"bodySchemaVersion"`
	Fields            json.RawMessage `json:"fields"`
	CreatedAt         time.Time       `json:"createdAt"`
}

type contentDetail struct {
	contentSummary
	CategoryIDs         []int64      `json:"categoryIds"`
	TagIDs              []int64      `json:"tagIds"`
	Revision            revisionJSON `json:"revision"`
	PublishedRevisionID *int64       `json:"publishedRevisionId"`
	// PendingSettings tells that the settings shown (slug, parentId,
	// categoryIds, tagIds, publishedAt) wait to be published; url is the
	// live one.
	PendingSettings bool `json:"pendingSettings"`
}

func status(c dbgen.Content) string {
	switch {
	case c.PublishedRevisionID != nil:
		return "published"
	case c.ScheduledRevisionID != nil:
		return "scheduled"
	}
	return "draft"
}

func summary(c dbgen.Content, title string, latestRevisionID int64, url *string) contentSummary {
	return contentSummary{
		ID: c.ID, Number: c.Number, Type: c.Type, Title: title, Slug: c.Slug, Status: status(c),
		HasUnpublishedChanges: c.PublishedRevisionID != nil &&
			(*c.PublishedRevisionID != latestRevisionID || c.PendingSettings != nil),
		URL: url, PublishedAt: c.PublishedAt, ScheduledAt: c.ScheduledAt, ParentID: c.ParentID,
		ScheduledRevisionID: c.ScheduledRevisionID, LatestRevisionID: latestRevisionID,
		UpdatedAt: c.UpdatedAt,
	}
}

// likeEscaper escapes ILIKE wildcards so searches match text literally.
var likeEscaper = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func contentType(t string) error {
	if t != "post" && t != "page" {
		return invalid("invalid_type", "種類は記事か固定ページを選んでください")
	}
	return nil
}

// maxListRows is the most contents one list request returns.
const maxListRows = 1000

func (s *Server) listContents(w http.ResponseWriter, r *http.Request, _ *session) error {
	query := r.URL.Query()
	typ := query.Get("type")
	if err := contentType(typ); err != nil {
		return err
	}
	params := dbgen.ListContentsParams{Type: typ, MaxRows: maxListRows}
	if v := query.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return invalid("invalid_before", "ページの指定が正しくありません")
		}
		params.Before = &n
	}
	if v := query.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxListRows {
			return invalid("invalid_limit", fmt.Sprintf("件数は1から%dのあいだで指定してください", maxListRows))
		}
		params.MaxRows = int32(n)
	}
	if q := strings.TrimSpace(query.Get("q")); q != "" {
		if len([]rune(q)) > 100 || !plainText(q, false) {
			return invalid("invalid_q", "検索する言葉は100文字以内で入力してください")
		}
		pattern := "%" + likeEscaper.Replace(q) + "%"
		params.Q = &pattern
	}
	if st := query.Get("status"); st != "" {
		if st != "draft" && st != "scheduled" && st != "published" && st != "changed" {
			return invalid("invalid_status", "状態の指定が正しくありません")
		}
		params.Status = &st
	}
	// sort=updated lists the last updated first, for the admin home; it
	// takes no filters or paging.
	sort := query.Get("sort")
	if sort != "" && sort != "updated" || sort == "updated" && (params.Q != nil || params.Status != nil || params.Before != nil) {
		return invalid("invalid_sort", "並べ替えの指定が正しくありません")
	}
	out := []contentSummary{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		var rows []dbgen.ListContentsRow
		var err error
		if sort == "updated" {
			var recent []dbgen.ListRecentContentsRow
			recent, err = q.ListRecentContents(r.Context(), dbgen.ListRecentContentsParams{Type: typ, MaxRows: params.MaxRows})
			for _, row := range recent {
				rows = append(rows, dbgen.ListContentsRow(row))
			}
		} else {
			rows, err = q.ListContents(r.Context(), params)
		}
		for _, row := range rows {
			c := dbgen.Content{
				TenantID: row.TenantID, ID: row.ID, Number: row.Number, Type: row.Type, Slug: row.Slug,
				ParentID: row.ParentID, PublishedRevisionID: row.PublishedRevisionID,
				ScheduledRevisionID: row.ScheduledRevisionID, ScheduledAt: row.ScheduledAt,
				PublishedAt: row.PublishedAt, PendingSettings: row.PendingSettings, UpdatedAt: row.UpdatedAt,
			}
			out = append(out, summary(c, row.Title, row.LatestRevisionID, row.Url))
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// typeCounts are the contents of one type by what the admin home shows.
type typeCounts struct {
	Draft     int64 `json:"draft"`
	Scheduled int64 `json:"scheduled"`
	Published int64 `json:"published"`
	// Changed are published contents with unpublished changes
	// (hasUnpublishedChanges).
	Changed int64 `json:"changed"`
	// NextScheduledAt is the earliest scheduledAt of the scheduled ones.
	NextScheduledAt *time.Time `json:"nextScheduledAt"`
}

func (s *Server) countContents(w http.ResponseWriter, r *http.Request, _ *session) error {
	out := map[string]*typeCounts{"post": {}, "page": {}}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		counts, err := q.CountContents(r.Context())
		if err != nil {
			return err
		}
		for _, c := range counts {
			if t, ok := out[c.Type]; ok {
				t.Draft, t.Scheduled, t.Published, t.Changed = c.Draft, c.Scheduled, c.Published, c.Changed
			}
		}
		next, err := q.NextScheduledContents(r.Context())
		for _, n := range next {
			if t, ok := out[n.Type]; ok {
				t.NextScheduledAt = n.ScheduledAt
			}
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// loadDetail reads a content with its latest revision. A content of
// another tenant is invisible and yields pgx.ErrNoRows.
func loadDetail(ctx context.Context, q *dbgen.Queries, id int64) (contentDetail, error) {
	c, err := q.GetContent(ctx, id)
	if err != nil {
		return contentDetail{}, err
	}
	rev, err := q.GetLatestRevision(ctx, id)
	if err != nil {
		return contentDetail{}, fmt.Errorf("latest revision: %w", err)
	}
	var url *string
	if p, err := q.GetContentURL(ctx, &id); err == nil {
		url = &p
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return contentDetail{}, err
	}
	live, err := content.Live(ctx, q, c)
	if err != nil {
		return contentDetail{}, err
	}
	pending, err := content.Pending(c)
	if err != nil {
		return contentDetail{}, err
	}
	// The editor shows what was saved, pending or not.
	set := pending.Over(live)
	sum := summary(c, rev.Title, rev.ID, url)
	sum.Slug, sum.ParentID, sum.PublishedAt = set.Slug, set.ParentID, set.PublishedAt
	return contentDetail{
		contentSummary:  sum,
		CategoryIDs:     *set.CategoryIDs,
		TagIDs:          *set.TagIDs,
		PendingSettings: !pending.Empty(),
		Revision: revisionJSON{
			ID: rev.ID, Title: rev.Title, Body: rev.Body, BodySchemaVersion: rev.BodySchemaVersion,
			Fields: rev.Fields, CreatedAt: rev.CreatedAt,
		},
		PublishedRevisionID: c.PublishedRevisionID,
	}, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (s *Server) getContent(w http.ResponseWriter, r *http.Request, _ *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var d contentDetail
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		d, err = loadDetail(r.Context(), q, id)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) createContent(w http.ResponseWriter, r *http.Request, sess *session) error {
	var in struct {
		Type string `json:"type"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := contentType(in.Type); err != nil {
		return err
	}
	ctx := r.Context()
	var d contentDetail
	err := s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		c, err := q.CreateContent(ctx, dbgen.CreateContentParams{Type: in.Type, AuthorID: &sess.userID})
		if err != nil {
			return err
		}
		if _, err := q.CreateRevision(ctx, dbgen.CreateRevisionParams{
			ContentID: c.ID, Body: body.EmptyDoc, BodySchemaVersion: body.SchemaVersion,
			Fields: json.RawMessage(`{}`), CreatedBy: &sess.userID,
		}); err != nil {
			return err
		}
		d, err = loadDetail(ctx, q, c.ID)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, d)
	return nil
}

func (s *Server) saveDraft(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Title          string          `json:"title"`
		Body           json.RawMessage `json:"body"`
		Fields         json.RawMessage `json:"fields"`
		BaseRevisionID *int64          `json:"baseRevisionId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	title := strings.TrimSpace(in.Title)
	if len([]rune(title)) > 200 {
		return invalid("invalid_title", "タイトルは200文字以内で入力してください")
	}
	doc, err := body.Parse(in.Body)
	if err != nil {
		return err
	}
	if len(in.Fields) > 0 {
		var f map[string]any
		if err := json.Unmarshal(in.Fields, &f); err != nil || f == nil {
			return invalid("invalid_fields", "追加項目の形式が正しくありません")
		}
	}

	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if _, err := q.LockContent(ctx, id); err != nil {
			return err
		}
		prev, err := q.GetLatestRevision(ctx, id)
		if err != nil {
			return err
		}
		if in.BaseRevisionID != nil && *in.BaseRevisionID != prev.ID {
			return errEditConflict
		}
		if sess.role != "admin" {
			prevDoc, err := body.Parse(prev.Body)
			if err != nil {
				prevDoc = body.Node{}
			}
			if !rawHTMLUnchanged(body.RawHTML(prevDoc), body.RawHTML(doc)) {
				return &apiError{http.StatusForbidden, "forbidden", "HTMLブロックを追加・変更できるのは管理者だけです"}
			}
		}
		fields := in.Fields
		if len(fields) == 0 {
			fields = prev.Fields
		}
		if _, err := q.CreateRevision(ctx, dbgen.CreateRevisionParams{
			ContentID: id, Title: title, Body: in.Body, BodySchemaVersion: body.SchemaVersion,
			Fields: fields, CreatedBy: &sess.userID,
		}); err != nil {
			return err
		}
		if err := q.TouchContent(ctx, id); err != nil {
			return err
		}
		d, err = loadDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return err
	}
	// After the commit, so saves of one tenant do not queue up on the
	// job's unique key. A failure only delays pruning to a later save.
	if s.Jobs != nil {
		if err := s.Jobs.EnqueuePruneRevisions(ctx, sess.tenantID); err != nil {
			s.Logger.Warn("schedule revision pruning", "tenant_id", sess.tenantID, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

// rawHTMLUnchanged reports whether next only keeps (or drops) rawHtml
// blocks that prev already had, so editors can edit around migrated HTML.
func rawHTMLUnchanged(prev, next []string) bool {
	left := map[string]int{}
	for _, h := range prev {
		left[h]++
	}
	for _, h := range next {
		if left[h] == 0 {
			return false
		}
		left[h]--
	}
	return true
}

// patchContent changes the settings. On a live content they wait in
// pending_settings until its changes are published, like the title and
// body; otherwise they are saved directly.
func (s *Server) patchContent(w http.ResponseWriter, r *http.Request, _ *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Slug        json.RawMessage `json:"slug"`
		ParentID    json.RawMessage `json:"parentId"`
		CategoryIDs *[]int64        `json:"categoryIds"`
		TagIDs      *[]int64        `json:"tagIds"`
		PublishedAt *time.Time      `json:"publishedAt"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	var set content.Settings
	if len(in.Slug) > 0 {
		if err := json.Unmarshal(in.Slug, &set.Slug); err != nil {
			return invalid("invalid_slug", slugMessage)
		}
		if set.Slug != nil && *set.Slug == "" {
			set.Slug = nil
		}
		set.SlugSet = true
	}
	if len(in.ParentID) > 0 {
		if err := json.Unmarshal(in.ParentID, &set.ParentID); err != nil {
			return invalid("invalid_parent", "親ページの指定が正しくありません")
		}
		set.ParentSet = true
	}
	if in.PublishedAt != nil {
		at := in.PublishedAt.UTC().Truncate(time.Second)
		if at.Year() < 1900 || at.After(time.Now().Add(time.Minute)) {
			return invalid("invalid_published_at", "公開日は今より前の日時にしてください。先の日時に公開するときは「日時を指定して公開」を使ってください")
		}
		set.PublishedAt = &at
	}
	if in.CategoryIDs != nil {
		ids := dedupe(*in.CategoryIDs)
		set.CategoryIDs = &ids
	}
	if in.TagIDs != nil {
		ids := dedupe(*in.TagIDs)
		set.TagIDs = &ids
	}

	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockContent(ctx, id); err != nil {
			return err
		}
		c, err := q.GetContent(ctx, id)
		if err != nil {
			return err
		}
		if err := checkSettings(ctx, q, c, set); err != nil {
			return err
		}
		if c.PublishedRevisionID == nil {
			if err := content.Apply(ctx, q, c, set); err != nil {
				return err
			}
		} else {
			pending, err := content.Pending(c)
			if err != nil {
				return err
			}
			live, err := content.Live(ctx, q, c)
			if err != nil {
				return err
			}
			next := set.Over(pending).ChangesFrom(live)
			// A URL in use is reported now rather than when publishing.
			if err := content.Try(ctx, tx, q, c, next); err != nil {
				return err
			}
			b, err := next.Marshal()
			if err != nil {
				return err
			}
			if err := q.SetPendingSettings(ctx, dbgen.SetPendingSettingsParams{ID: id, PendingSettings: b}); err != nil {
				return err
			}
		}
		d, err = loadDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

// checkSettings validates settings typed in the admin for c. A slug or
// parent equal to the live one is not checked again, so slugs migrated
// from WordPress can stay.
func checkSettings(ctx context.Context, q *dbgen.Queries, c dbgen.Content, set content.Settings) error {
	if set.SlugSet && set.Slug != nil && !equalPtr(set.Slug, c.Slug) {
		if err := checkSlug(*set.Slug); err != nil {
			return err
		}
	}
	if set.ParentSet && !equalPtr(set.ParentID, c.ParentID) {
		if err := checkParent(ctx, q, c, set.ParentID); err != nil {
			return err
		}
	}
	missing := invalid("invalid_reference", "指定したものが見つかりません")
	if set.CategoryIDs != nil {
		ids, err := q.ListCategoryIDsIn(ctx, *set.CategoryIDs)
		if err != nil {
			return err
		}
		if len(ids) != len(*set.CategoryIDs) {
			return missing
		}
	}
	if set.TagIDs != nil {
		ids, err := q.ListTagIDsIn(ctx, *set.TagIDs)
		if err != nil {
			return err
		}
		if len(ids) != len(*set.TagIDs) {
			return missing
		}
	}
	return nil
}

func checkParent(ctx context.Context, q *dbgen.Queries, c dbgen.Content, parent *int64) error {
	if parent == nil {
		return nil
	}
	if c.Type != "page" {
		return invalid("invalid_parent", "親ページを選べるのは固定ページだけです")
	}
	p, err := q.GetContent(ctx, *parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("invalid_parent", "親ページが見つかりません")
	}
	if err != nil {
		return err
	}
	if p.Type != "page" {
		return invalid("invalid_parent", "親には固定ページを選んでください")
	}
	chain, err := q.ContentAncestry(ctx, *parent)
	if err != nil {
		return err
	}
	for _, a := range chain {
		if a.ID == c.ID {
			return invalid("invalid_parent", "親ページに、このページ自身やその下のページは選べません")
		}
	}
	return nil
}

func (s *Server) publish(w http.ResponseWriter, r *http.Request, sess *session) error {
	return s.setPublished(w, r, sess, true)
}

func (s *Server) unpublish(w http.ResponseWriter, r *http.Request, sess *session) error {
	return s.setPublished(w, r, sess, false)
}

func (s *Server) setPublished(w http.ResponseWriter, r *http.Request, sess *session, publish bool) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	// baseRevisionId (optional) is the revision the editor saw: publishing
	// refuses if someone saved a newer one meanwhile.
	var in struct {
		BaseRevisionID *int64 `json:"baseRevisionId"`
	}
	if err := decodeOptional(w, r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockContent(ctx, id); err != nil {
			return err
		}
		var rev *int64
		if publish {
			latest, err := q.GetLatestRevision(ctx, id)
			if err != nil {
				return err
			}
			if in.BaseRevisionID != nil && *in.BaseRevisionID != latest.ID {
				return errEditConflict
			}
			rev = &latest.ID
		}
		c, err := q.GetContent(ctx, id)
		if err != nil {
			return err
		}
		if publish {
			// Settings saved while live go out with the changes; a URL
			// taken meanwhile fails the publishing as it would the save.
			if err := content.ApplyPending(ctx, q, c); err != nil {
				return err
			}
		} else {
			// Unpublishing must not be blocked by a URL that cannot be
			// applied: it then stays as it is.
			skipped, err := content.ApplyPendingExceptURL(ctx, tx, q, c, false)
			if err != nil {
				return err
			}
			if skipped != nil {
				s.Logger.WarnContext(ctx, "pending URL dropped on unpublish", "content_id", id, "err", skipped)
			}
		}
		if _, err := q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: id, RevisionID: rev}); err != nil {
			return err
		}
		// Publishing now or unpublishing supersedes any schedule: a pending
		// one would otherwise republish an older revision later.
		if _, err := q.ScheduleRevision(ctx, dbgen.ScheduleRevisionParams{ContentID: id}); err != nil {
			return err
		}
		if publish {
			if err := routing.SyncContent(ctx, q, id); err != nil {
				return err
			}
		}
		if err := s.enqueueBuild(ctx, tx, sess.tenantID); err != nil {
			return err
		}
		d, err = loadDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) deleteContent(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		children, err := q.ListChildContentIDs(ctx, &id)
		if err != nil {
			return err
		}
		if len(children) > 0 {
			return invalid("has_children", "下に固定ページがあるため削除できません。先に下のページを別の場所へ移すか、削除してください")
		}
		n, err := q.DeleteContent(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return errNotFound
		}
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func dedupe(ids []int64) []int64 {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// minScheduleLead keeps schedules from landing in the past by the time the
// request commits.
const minScheduleLead = time.Minute

func (s *Server) schedule(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		PublishAt      time.Time `json:"publishAt"`
		BaseRevisionID *int64    `json:"baseRevisionId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.PublishAt.Before(time.Now().Add(minScheduleLead)) {
		return invalid("invalid_publish_at", "公開する日時は、今から1分以上あとにしてください")
	}
	at := in.PublishAt.UTC().Truncate(time.Second)
	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockContent(ctx, id); err != nil {
			return err
		}
		latest, err := q.GetLatestRevision(ctx, id)
		if err != nil {
			return err
		}
		if in.BaseRevisionID != nil && *in.BaseRevisionID != latest.ID {
			return errEditConflict
		}
		if _, err := q.ScheduleRevision(ctx, dbgen.ScheduleRevisionParams{ContentID: id, RevisionID: &latest.ID, ScheduledAt: &at}); err != nil {
			return err
		}
		if s.Jobs != nil {
			if err := s.Jobs.EnqueuePublish(ctx, tx, sess.tenantID, id, latest.ID, at); err != nil {
				return fmt.Errorf("enqueue publish: %w", err)
			}
		}
		d, err = loadDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) unschedule(w http.ResponseWriter, r *http.Request, _ *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if _, err := q.GetContent(ctx, id); err != nil {
			return err
		}
		if _, err := q.ScheduleRevision(ctx, dbgen.ScheduleRevisionParams{ContentID: id}); err != nil {
			return err
		}
		d, err = loadDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}
