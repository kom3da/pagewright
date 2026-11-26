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
	"github.com/kom3da/pagewright/internal/db/dbgen"
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
	ParentID            *int64       `json:"parentId"`
	CategoryIDs         []int64      `json:"categoryIds"`
	TagIDs              []int64      `json:"tagIds"`
	Revision            revisionJSON `json:"revision"`
	PublishedRevisionID *int64       `json:"publishedRevisionId"`
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
		HasUnpublishedChanges: c.PublishedRevisionID != nil && *c.PublishedRevisionID != latestRevisionID,
		URL:                   url, PublishedAt: c.PublishedAt, UpdatedAt: c.UpdatedAt,
	}
}

func contentType(t string) error {
	if t != "post" && t != "page" {
		return invalid("invalid_type", "種類は記事か固定ページを選んでください")
	}
	return nil
}

func (s *Server) listContents(w http.ResponseWriter, r *http.Request, _ *session) error {
	typ := r.URL.Query().Get("type")
	if err := contentType(typ); err != nil {
		return err
	}
	out := []contentSummary{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListContents(r.Context(), typ)
		for _, row := range rows {
			c := dbgen.Content{
				TenantID: row.TenantID, ID: row.ID, Number: row.Number, Type: row.Type, Slug: row.Slug,
				ParentID: row.ParentID, PublishedRevisionID: row.PublishedRevisionID,
				ScheduledRevisionID: row.ScheduledRevisionID, PublishedAt: row.PublishedAt, UpdatedAt: row.UpdatedAt,
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
	cats, err := q.ListContentCategoryIDs(ctx, id)
	if err != nil {
		return contentDetail{}, err
	}
	tags, err := q.ListContentTagIDs(ctx, id)
	if err != nil {
		return contentDetail{}, err
	}
	return contentDetail{
		contentSummary: summary(c, rev.Title, rev.ID, url),
		ParentID:       c.ParentID,
		CategoryIDs:    nonNil(cats),
		TagIDs:         nonNil(tags),
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
		Title  string          `json:"title"`
		Body   json.RawMessage `json:"body"`
		Fields json.RawMessage `json:"fields"`
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
		if _, err := q.GetContent(ctx, id); err != nil {
			return err
		}
		prev, err := q.GetLatestRevision(ctx, id)
		if err != nil {
			return err
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

func (s *Server) patchContent(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Slug        json.RawMessage `json:"slug"`
		ParentID    json.RawMessage `json:"parentId"`
		CategoryIDs *[]int64        `json:"categoryIds"`
		TagIDs      *[]int64        `json:"tagIds"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	var slug, parent *json.RawMessage
	if len(in.Slug) > 0 {
		slug = &in.Slug
	}
	if len(in.ParentID) > 0 {
		parent = &in.ParentID
	}

	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		c, err := q.GetContent(ctx, id)
		if err != nil {
			return err
		}
		urlChanged := false
		if slug != nil {
			var v *string
			if err := json.Unmarshal(*slug, &v); err != nil {
				return invalid("invalid_slug", slugMessage)
			}
			if v != nil && *v == "" {
				v = nil
			}
			if !equalPtr(v, c.Slug) {
				if v != nil {
					if err := checkSlug(*v); err != nil {
						return err
					}
				}
				if err := q.UpdateContentSlug(ctx, dbgen.UpdateContentSlugParams{ID: id, Slug: v}); err != nil {
					return err
				}
				urlChanged = true
			}
		}
		if parent != nil {
			var v *int64
			if err := json.Unmarshal(*parent, &v); err != nil {
				return invalid("invalid_parent", "親ページの指定が正しくありません")
			}
			if !equalPtr(v, c.ParentID) {
				if err := checkParent(ctx, q, c, v); err != nil {
					return err
				}
				if err := q.UpdateContentParent(ctx, dbgen.UpdateContentParentParams{ID: id, ParentID: v}); err != nil {
					return err
				}
				urlChanged = true
			}
		}
		if in.CategoryIDs != nil {
			if err := q.ClearContentCategories(ctx, id); err != nil {
				return err
			}
			for _, cid := range dedupe(*in.CategoryIDs) {
				if err := q.AddContentCategory(ctx, dbgen.AddContentCategoryParams{ContentID: id, CategoryID: cid}); err != nil {
					return err
				}
			}
		}
		if in.TagIDs != nil {
			if err := q.ClearContentTags(ctx, id); err != nil {
				return err
			}
			for _, tid := range dedupe(*in.TagIDs) {
				if err := q.AddContentTag(ctx, dbgen.AddContentTagParams{ContentID: id, TagID: tid}); err != nil {
					return err
				}
			}
		}
		if urlChanged && c.Type == "page" {
			updated, err := q.GetContent(ctx, id)
			if err != nil {
				return err
			}
			if _, err := contentPath(ctx, q, updated); err != nil {
				return err
			}
		}
		if urlChanged {
			if err := syncContentRoute(ctx, q, id); err != nil {
				return err
			}
		}
		if c.PublishedRevisionID != nil {
			if err := s.enqueueBuild(ctx, tx, sess.tenantID); err != nil {
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
	ctx := r.Context()
	var d contentDetail
	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.GetContent(ctx, id); err != nil {
			return err
		}
		var rev *int64
		if publish {
			latest, err := q.GetLatestRevision(ctx, id)
			if err != nil {
				return err
			}
			rev = &latest.ID
		}
		if _, err := q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: id, RevisionID: rev}); err != nil {
			return err
		}
		if publish {
			if err := syncContentRoute(ctx, q, id); err != nil {
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

// contentPath is the MVP URL of a content: /posts/{seg}/ for posts and
// /{ancestor segs}/{seg}/ for pages, where seg is the slug or the number.
func contentPath(ctx context.Context, q *dbgen.Queries, c dbgen.Content) (string, error) {
	seg := func(slug *string, number int64) string {
		if slug != nil {
			return *slug
		}
		return strconv.FormatInt(number, 10)
	}
	if c.Type != "page" {
		return "/posts/" + seg(c.Slug, c.Number) + "/", nil
	}
	chain, err := q.ContentAncestry(ctx, c.ID)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(chain))
	for _, a := range chain {
		parts = append(parts, seg(a.Slug, a.Number))
	}
	path := "/" + strings.Join(parts, "/") + "/"
	for _, reserved := range reservedPrefixes {
		if strings.HasPrefix(path, reserved) {
			return "", invalid("reserved_slug", "「posts」と「media」はサイトの仕組みで使っているため、いちばん上の固定ページのURLには使えません")
		}
	}
	return path, nil
}

// reservedPrefixes are paths the site itself uses (post lists, media).
var reservedPrefixes = []string{"/posts/", "/media/"}

// syncContentRoute makes the canonical route of a content match its URL.
// A previous canonical URL is kept as a redirect. Pages below it follow.
// Contents that were never published (no route yet) are left alone until
// they are published.
func syncContentRoute(ctx context.Context, q *dbgen.Queries, id int64) error {
	c, err := q.GetContent(ctx, id)
	if err != nil {
		return err
	}
	cur, err := q.GetCanonicalRouteForContent(ctx, &id)
	hasRoute := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !hasRoute && c.PublishedRevisionID == nil {
		return nil
	}
	path, err := contentPath(ctx, q, c)
	if err != nil {
		return err
	}
	if hasRoute && cur.Path == path {
		return nil
	}
	if err := moveCanonical(ctx, q, cur, hasRoute, path, dbgen.CreateRouteParams{ContentID: &id}); err != nil {
		return err
	}
	if c.Type != "page" {
		return nil
	}
	children, err := q.ListChildContentIDs(ctx, &id)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := syncContentRoute(ctx, q, child); err != nil {
			return err
		}
	}
	return nil
}

// moveCanonical points the target at path: the old canonical route (if
// any) becomes a redirect, and a redirect of the same target already at
// path is replaced. Another target at path is a conflict.
func moveCanonical(ctx context.Context, q *dbgen.Queries, cur dbgen.Route, hasCur bool, path string, target dbgen.CreateRouteParams) error {
	if hasCur {
		if err := q.DemoteRouteToRedirect(ctx, cur.ID); err != nil {
			return err
		}
	}
	existing, err := q.GetRouteByPath(ctx, path)
	switch {
	case err == nil:
		if !sameTarget(existing, target) {
			return errURLTaken
		}
		if err := q.DeleteRoute(ctx, existing.ID); err != nil {
			return err
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	target.Path, target.Kind = path, "canonical"
	_, err = q.CreateRoute(ctx, target)
	return err
}

func sameTarget(r dbgen.Route, t dbgen.CreateRouteParams) bool {
	return equalPtr(r.ContentID, t.ContentID) && equalPtr(r.CategoryID, t.CategoryID) && equalPtr(r.TagID, t.TagID)
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
