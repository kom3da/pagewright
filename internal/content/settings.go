// Package content holds the settings of a content that are edited beside
// its revisions: the URL (slug and parent page), categories, tags and the
// publication date. For a live content, saved settings wait in
// contents.pending_settings until its changes are published.
package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/routing"
)

// ErrParentLoop means the parent page is the page itself or below it.
var ErrParentLoop = errors.New("content: parent is the page itself or below it")

// Settings are settings of a content. Unset fields are left as they are.
type Settings struct {
	SlugSet     bool
	Slug        *string // nil: the number is used
	ParentSet   bool
	ParentID    *int64
	CategoryIDs *[]int64 // sorted, without duplicates
	TagIDs      *[]int64 // sorted, without duplicates
	PublishedAt *time.Time
}

// stored is the JSON form of Settings in contents.pending_settings. Absent
// keys are unset.
type stored struct {
	Slug        json.RawMessage `json:"slug,omitempty"`
	ParentID    json.RawMessage `json:"parentId,omitempty"`
	CategoryIDs *[]int64        `json:"categoryIds,omitempty"`
	TagIDs      *[]int64        `json:"tagIds,omitempty"`
	PublishedAt *time.Time      `json:"publishedAt,omitempty"`
}

// Empty reports whether nothing is set.
func (s Settings) Empty() bool {
	return !s.SlugSet && !s.ParentSet && s.CategoryIDs == nil && s.TagIDs == nil && s.PublishedAt == nil
}

// Over returns base with the fields set in s replaced.
func (s Settings) Over(base Settings) Settings {
	if s.SlugSet {
		base.SlugSet, base.Slug = true, s.Slug
	}
	if s.ParentSet {
		base.ParentSet, base.ParentID = true, s.ParentID
	}
	if s.CategoryIDs != nil {
		base.CategoryIDs = s.CategoryIDs
	}
	if s.TagIDs != nil {
		base.TagIDs = s.TagIDs
	}
	if s.PublishedAt != nil {
		base.PublishedAt = s.PublishedAt
	}
	return base
}

// ChangesFrom keeps only the fields of s that differ from live.
func (s Settings) ChangesFrom(live Settings) Settings {
	if s.SlugSet && equalPtr(s.Slug, live.Slug) {
		s.SlugSet, s.Slug = false, nil
	}
	if s.ParentSet && equalPtr(s.ParentID, live.ParentID) {
		s.ParentSet, s.ParentID = false, nil
	}
	if s.CategoryIDs != nil && live.CategoryIDs != nil && slices.Equal(*s.CategoryIDs, *live.CategoryIDs) {
		s.CategoryIDs = nil
	}
	if s.TagIDs != nil && live.TagIDs != nil && slices.Equal(*s.TagIDs, *live.TagIDs) {
		s.TagIDs = nil
	}
	if s.PublishedAt != nil && live.PublishedAt != nil && s.PublishedAt.Equal(*live.PublishedAt) {
		s.PublishedAt = nil
	}
	return s
}

// withoutURL drops the slug and the parent.
func (s Settings) withoutURL() Settings {
	s.SlugSet, s.Slug, s.ParentSet, s.ParentID = false, nil, false, nil
	return s
}

// onlyURL keeps only the slug and the parent.
func (s Settings) onlyURL() Settings {
	return Settings{SlugSet: s.SlugSet, Slug: s.Slug, ParentSet: s.ParentSet, ParentID: s.ParentID}
}

// Marshal returns the stored form, or nil when nothing is set.
func (s Settings) Marshal() ([]byte, error) {
	if s.Empty() {
		return nil, nil
	}
	out := stored{CategoryIDs: s.CategoryIDs, TagIDs: s.TagIDs, PublishedAt: s.PublishedAt}
	var err error
	if s.SlugSet {
		if out.Slug, err = json.Marshal(s.Slug); err != nil {
			return nil, err
		}
	}
	if s.ParentSet {
		if out.ParentID, err = json.Marshal(s.ParentID); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

// Pending returns the settings of c that wait to be published.
func Pending(c dbgen.Content) (Settings, error) {
	var s Settings
	if len(c.PendingSettings) == 0 {
		return s, nil
	}
	var in stored
	if err := json.Unmarshal(c.PendingSettings, &in); err != nil {
		return s, fmt.Errorf("pending settings of content %d: %w", c.ID, err)
	}
	if len(in.Slug) > 0 {
		s.SlugSet = true
		if err := json.Unmarshal(in.Slug, &s.Slug); err != nil {
			return s, fmt.Errorf("pending slug of content %d: %w", c.ID, err)
		}
	}
	if len(in.ParentID) > 0 {
		s.ParentSet = true
		if err := json.Unmarshal(in.ParentID, &s.ParentID); err != nil {
			return s, fmt.Errorf("pending parent of content %d: %w", c.ID, err)
		}
	}
	s.CategoryIDs, s.TagIDs, s.PublishedAt = in.CategoryIDs, in.TagIDs, in.PublishedAt
	return s, nil
}

// Live returns the settings of c as they are now, all set.
func Live(ctx context.Context, q *dbgen.Queries, c dbgen.Content) (Settings, error) {
	cats, err := q.ListContentCategoryIDs(ctx, c.ID)
	if err != nil {
		return Settings{}, err
	}
	tags, err := q.ListContentTagIDs(ctx, c.ID)
	if err != nil {
		return Settings{}, err
	}
	cats, tags = nonNil(cats), nonNil(tags)
	return Settings{
		SlugSet: true, Slug: c.Slug, ParentSet: true, ParentID: c.ParentID,
		CategoryIDs: &cats, TagIDs: &tags, PublishedAt: c.PublishedAt,
	}, nil
}

// Apply writes s to the content's columns and joins, and moves its routes
// when the URL changes. The caller has validated s; categories and tags
// deleted since are skipped. A URL that is taken or reserved fails with
// routing.ErrURLTaken or routing.ErrReservedPath, a parent that would make
// a loop with ErrParentLoop.
func Apply(ctx context.Context, q *dbgen.Queries, c dbgen.Content, s Settings) error {
	urlChanged := false
	if s.SlugSet && !equalPtr(s.Slug, c.Slug) {
		if err := q.UpdateContentSlug(ctx, dbgen.UpdateContentSlugParams{ID: c.ID, Slug: s.Slug}); err != nil {
			return err
		}
		urlChanged = true
	}
	if s.ParentSet && !equalPtr(s.ParentID, c.ParentID) {
		if s.ParentID != nil {
			chain, err := q.ContentAncestry(ctx, *s.ParentID)
			if err != nil {
				return err
			}
			for _, a := range chain {
				if a.ID == c.ID {
					return ErrParentLoop
				}
			}
		}
		if err := q.UpdateContentParent(ctx, dbgen.UpdateContentParentParams{ID: c.ID, ParentID: s.ParentID}); err != nil {
			return err
		}
		urlChanged = true
	}
	if s.PublishedAt != nil {
		if err := q.SetPublishedAt(ctx, dbgen.SetPublishedAtParams{ID: c.ID, PublishedAt: s.PublishedAt}); err != nil {
			return err
		}
	}
	if s.CategoryIDs != nil {
		ids, err := q.ListCategoryIDsIn(ctx, *s.CategoryIDs)
		if err != nil {
			return err
		}
		if err := q.ClearContentCategories(ctx, c.ID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := q.AddContentCategory(ctx, dbgen.AddContentCategoryParams{ContentID: c.ID, CategoryID: id}); err != nil {
				return err
			}
		}
	}
	if s.TagIDs != nil {
		ids, err := q.ListTagIDsIn(ctx, *s.TagIDs)
		if err != nil {
			return err
		}
		if err := q.ClearContentTags(ctx, c.ID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := q.AddContentTag(ctx, dbgen.AddContentTagParams{ContentID: c.ID, TagID: id}); err != nil {
				return err
			}
		}
	}
	if !urlChanged {
		return nil
	}
	if c.Type == "page" {
		updated, err := q.GetContent(ctx, c.ID)
		if err != nil {
			return err
		}
		if _, err := routing.ContentPath(ctx, q, updated); err != nil {
			return err
		}
	}
	return routing.SyncContent(ctx, q, c.ID)
}

// Try runs Apply in a savepoint and undoes it, to find a problem when the
// settings are saved rather than when they are published.
func Try(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, c dbgen.Content, s Settings) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	err = Apply(ctx, q, c, s)
	if rerr := sp.Rollback(ctx); err == nil {
		err = rerr
	}
	return err
}

// ApplyPending applies the pending settings of c and clears them. On a
// problem nothing is changed.
func ApplyPending(ctx context.Context, q *dbgen.Queries, c dbgen.Content) error {
	p, err := Pending(c)
	if err != nil || p.Empty() {
		return err
	}
	if err := Apply(ctx, q, c, p); err != nil {
		return err
	}
	return q.SetPendingSettings(ctx, dbgen.SetPendingSettingsParams{ID: c.ID})
}

// ApplyPendingExceptURL is ApplyPending for when c goes live or down
// anyway. If the pending URL (slug or parent) cannot be applied, the rest
// still is and skipped says why; with keepURL the URL change stays pending,
// otherwise it is dropped.
func ApplyPendingExceptURL(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, c dbgen.Content, keepURL bool) (skipped, err error) {
	p, err := Pending(c)
	if err != nil || p.Empty() {
		return nil, err
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if skipped = Apply(ctx, q, c, p); skipped == nil {
		if err := sp.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, q.SetPendingSettings(ctx, dbgen.SetPendingSettingsParams{ID: c.ID})
	}
	if err := sp.Rollback(ctx); err != nil {
		return nil, err
	}
	if !IsProblem(skipped) || p.onlyURL().Empty() {
		return nil, skipped
	}
	if err := Apply(ctx, q, c, p.withoutURL()); err != nil {
		return nil, err
	}
	var left []byte
	if keepURL {
		if left, err = p.onlyURL().Marshal(); err != nil {
			return nil, err
		}
	}
	return skipped, q.SetPendingSettings(ctx, dbgen.SetPendingSettingsParams{ID: c.ID, PendingSettings: left})
}

// IsProblem reports whether err is a problem with the settings themselves
// (a URL in use or reserved, a parent loop, a missing parent) rather than
// a failure of the database.
func IsProblem(err error) bool {
	var pe *pgconn.PgError
	return errors.Is(err, routing.ErrURLTaken) || errors.Is(err, routing.ErrReservedPath) ||
		errors.Is(err, ErrParentLoop) ||
		errors.As(err, &pe) && (pe.Code == "23505" || pe.Code == "23503" || pe.Code == "23514")
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
