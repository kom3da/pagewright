// Package routing keeps the routes table in step with contents,
// categories and tags: canonical URLs, and old URLs kept as redirects.
package routing

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

var (
	// ErrURLTaken means another content, category or tag has the path.
	ErrURLTaken = errors.New("routing: path is taken")
	// ErrReservedPath means a top-level page would take a path the site
	// itself uses.
	ErrReservedPath = errors.New("routing: path is reserved")
)

// CategoryPath is the URL of a category.
func CategoryPath(slug string) string { return "/posts/category/" + slug + "/" }

// TagPath is the URL of a tag.
func TagPath(slug string) string { return "/posts/tag/" + slug + "/" }

// ContentPath is the MVP URL of a content: /posts/{seg}/ for posts and
// /{ancestor segs}/{seg}/ for pages, where seg is the slug or the number.
func ContentPath(ctx context.Context, q *dbgen.Queries, c dbgen.Content) (string, error) {
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
			return "", ErrReservedPath
		}
	}
	return path, nil
}

// reservedPrefixes are paths the site itself uses (post lists, media).
var reservedPrefixes = []string{"/posts/", "/media/"}

// SyncContent makes the canonical route of a content match its URL.
// A previous canonical URL is kept as a redirect. Pages below it follow.
// Contents that were never published (no route yet) are left alone until
// they are published.
func SyncContent(ctx context.Context, q *dbgen.Queries, id int64) error {
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
	path, err := ContentPath(ctx, q, c)
	if err != nil {
		return err
	}
	if hasRoute && cur.Path == path {
		return nil
	}
	if err := MoveCanonical(ctx, q, cur, hasRoute, path, dbgen.CreateRouteParams{ContentID: &id}); err != nil {
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
		if err := SyncContent(ctx, q, child); err != nil {
			return err
		}
	}
	return nil
}

// MoveCanonical points the target at path: the old canonical route (if
// any) becomes a redirect, and a redirect of the same target already at
// path is replaced. Another target at path is a conflict.
func MoveCanonical(ctx context.Context, q *dbgen.Queries, cur dbgen.Route, hasCur bool, path string, target dbgen.CreateRouteParams) error {
	if hasCur {
		if err := q.DemoteRouteToRedirect(ctx, cur.ID); err != nil {
			return err
		}
	}
	existing, err := q.GetRouteByPath(ctx, path)
	switch {
	case err == nil:
		if !sameTarget(existing, target) {
			return ErrURLTaken
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
