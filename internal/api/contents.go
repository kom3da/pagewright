package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/body"
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
	ScheduledRevisionID *int64       `json:"scheduledRevisionId"`
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
		URL:                   url, PublishedAt: c.PublishedAt, ScheduledAt: c.ScheduledAt, ParentID: c.ParentID,
		UpdatedAt: c.UpdatedAt,
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
				ScheduledRevisionID: row.ScheduledRevisionID, ScheduledAt: row.ScheduledAt,
				PublishedAt: row.PublishedAt, UpdatedAt: row.UpdatedAt,
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
		CategoryIDs:    nonNil(cats),
		TagIDs:         nonNil(tags),
		Revision: revisionJSON{
			ID: rev.ID, Title: rev.Title, Body: rev.Body, BodySchemaVersion: rev.BodySchemaVersion,
			Fields: rev.Fields, CreatedAt: rev.CreatedAt,
		},
		PublishedRevisionID: c.PublishedRevisionID,
		ScheduledRevisionID: c.ScheduledRevisionID,
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
			return &apiError{http.StatusConflict, "edit_conflict", "ほかの人がこの内容を先に保存しました。いまの編集内容を控えてから、画面を読み込み直してください"}
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
			if _, err := routing.ContentPath(ctx, q, updated); err != nil {
				return err
			}
		}
		if urlChanged {
			if err := routing.SyncContent(ctx, q, id); err != nil {
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
		PublishAt time.Time `json:"publishAt"`
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
		if _, err := q.GetContent(ctx, id); err != nil {
			return err
		}
		latest, err := q.GetLatestRevision(ctx, id)
		if err != nil {
			return err
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
