package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// liveState is what the public site uses for a content.
type liveState struct {
	Slug        *string
	PublishedAt time.Time
	Categories  []int64
	Tags        []int64
	Canonical   string
	Pending     bool
}

func (e *env) liveState(id int64) liveState {
	e.t.Helper()
	ctx := context.Background()
	var s liveState
	if err := e.db.Admin.QueryRow(ctx, `
		SELECT c.slug, c.published_at, c.pending_settings IS NOT NULL,
		  coalesce((SELECT array_agg(category_id ORDER BY category_id) FROM content_categories WHERE content_id = c.id), '{}'),
		  coalesce((SELECT array_agg(tag_id ORDER BY tag_id) FROM content_tags WHERE content_id = c.id), '{}'),
		  coalesce((SELECT path FROM routes WHERE content_id = c.id AND kind = 'canonical'), '')
		FROM contents c WHERE c.id = $1`, id).Scan(&s.Slug, &s.PublishedAt, &s.Pending, &s.Categories, &s.Tags, &s.Canonical); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func TestPendingSettings(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	path := func(id int64, suffix string) string { return fmt.Sprintf("/api/contents/%d%s", id, suffix) }
	var cat, tag struct{ ID int64 }
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "お知らせ", "slug": "news"}, &cat, http.StatusCreated)
	alice.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "夏", "slug": "summer"}, &tag, http.StatusCreated)

	p := alice.newContent("post", "公開中の記事", true)
	oldURL := fmt.Sprintf("/posts/%d/", p.Number)
	before := e.liveState(p.ID)
	builds := e.builds.count(e.tenantA)

	// Setting a value back to the live one leaves nothing pending.
	var d detail
	alice.mustDo(http.MethodPatch, path(p.ID, ""), map[string]any{"slug": "x"}, &d, http.StatusOK)
	if !d.PendingSettings || !d.HasUnpublishedChanges {
		t.Errorf("after a pending slug: %+v, want pending settings", d)
	}
	alice.mustDo(http.MethodPatch, path(p.ID, ""), map[string]any{"slug": nil}, &d, http.StatusOK)
	if d.PendingSettings || d.HasUnpublishedChanges {
		t.Errorf("after setting the slug back: %+v, want nothing pending", d)
	}

	// Saved settings of a live post wait: the editor sees them, the site does not.
	at := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	alice.mustDo(http.MethodPatch, path(p.ID, ""), map[string]any{
		"slug": "summer-festival", "categoryIds": []int64{cat.ID}, "tagIds": []int64{tag.ID}, "publishedAt": at,
	}, &d, http.StatusOK)
	if d.Slug == nil || *d.Slug != "summer-festival" || !cmp.Equal(d.CategoryIDs, []int64{cat.ID}) ||
		!cmp.Equal(d.TagIDs, []int64{tag.ID}) || d.PublishedAt == nil || !d.PublishedAt.Equal(at) ||
		!d.PendingSettings || !d.HasUnpublishedChanges || d.URL == nil || *d.URL != oldURL {
		t.Errorf("detail with pending settings = %+v, want them shown and the live url %s", d, oldURL)
	}
	alice.mustDo(http.MethodGet, path(p.ID, ""), nil, &d, http.StatusOK)
	if d.Slug == nil || *d.Slug != "summer-festival" || !d.PendingSettings {
		t.Errorf("GET with pending settings = %+v", d)
	}
	after := e.liveState(p.ID)
	before.Pending = true
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("live state changed by a pending save (-want +got):\n%s", diff)
	}
	if got := e.builds.count(e.tenantA); got != builds {
		t.Errorf("builds after a pending save = %d, want %d", got, builds)
	}
	var list []detail
	alice.mustDo(http.MethodGet, "/api/contents?type=post", nil, &list, http.StatusOK)
	if len(list) != 1 || list[0].Slug != nil || !list[0].HasUnpublishedChanges {
		t.Errorf("list = %+v, want the live slug and unpublished changes", list)
	}

	// Publishing applies them, as a normal URL change would.
	alice.mustDo(http.MethodPost, path(p.ID, "/publish"), nil, &d, http.StatusOK)
	if d.URL == nil || *d.URL != "/posts/summer-festival/" || d.PendingSettings || d.HasUnpublishedChanges {
		t.Errorf("after publishing = %+v, want the new url and nothing pending", d)
	}
	slug := "summer-festival"
	want := liveState{Slug: &slug, PublishedAt: at, Categories: []int64{cat.ID}, Tags: []int64{tag.ID}, Canonical: "/posts/summer-festival/"}
	if diff := cmp.Diff(want, e.liveState(p.ID)); diff != "" {
		t.Errorf("live state after publishing (-want +got):\n%s", diff)
	}
	var kind string
	if err := e.db.Admin.QueryRow(context.Background(), "SELECT kind FROM routes WHERE path = $1", oldURL).Scan(&kind); err != nil || kind != "redirect" {
		t.Errorf("old url %s: kind=%q err=%v, want redirect", oldURL, kind, err)
	}

	// Unpublishing applies them too.
	alice.mustDo(http.MethodPatch, path(p.ID, ""), map[string]any{"tagIds": []int64{}}, &d, http.StatusOK)
	alice.mustDo(http.MethodPost, path(p.ID, "/unpublish"), nil, &d, http.StatusOK)
	if got := e.liveState(p.ID); d.Status != "draft" || d.PendingSettings || len(got.Tags) != 0 || got.Pending {
		t.Errorf("after unpublishing: %+v, live %+v, want tags removed and nothing pending", d, got)
	}

	// A draft's settings are saved directly.
	alice.mustDo(http.MethodPatch, path(p.ID, ""), map[string]any{"slug": "draft-url"}, &d, http.StatusOK)
	if got := e.liveState(p.ID); d.PendingSettings || got.Slug == nil || *got.Slug != "draft-url" {
		t.Errorf("draft patch: %+v, live slug %v, want it saved directly", d, got.Slug)
	}

	// A category deleted while waiting is left out.
	q := alice.newContent("post", "もう一つ", true)
	var gone struct{ ID int64 }
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "消す", "slug": "gone"}, &gone, http.StatusCreated)
	alice.mustDo(http.MethodPatch, path(q.ID, ""), map[string]any{"categoryIds": []int64{cat.ID, gone.ID}}, nil, http.StatusOK)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/categories/%d", gone.ID), nil, nil, http.StatusNoContent)
	alice.mustDo(http.MethodPost, path(q.ID, "/publish"), nil, &d, http.StatusOK)
	if !cmp.Equal(d.CategoryIDs, []int64{cat.ID}) || d.PendingSettings {
		t.Errorf("after publishing with a deleted category = %+v, want only %d", d, cat.ID)
	}
}

// TestPendingSettingsConflict publishes a pending URL that another
// content took meanwhile: publishing fails and changes nothing.
func TestPendingSettingsConflict(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	path := func(id int64, suffix string) string { return fmt.Sprintf("/api/contents/%d%s", id, suffix) }
	first := alice.newContent("post", "一つ目", true)
	second := alice.newContent("post", "二つ目", true)
	alice.mustDo(http.MethodPatch, path(first.ID, ""), map[string]any{"slug": "taken"}, nil, http.StatusOK)
	alice.mustDo(http.MethodPatch, path(second.ID, ""), map[string]any{"slug": "taken"}, nil, http.StatusOK)
	alice.mustDo(http.MethodPost, path(second.ID, "/publish"), nil, nil, http.StatusOK)

	before := e.liveState(first.ID)
	builds := e.builds.count(e.tenantA)
	var out struct {
		Error struct{ Code, Field string } `json:"error"`
	}
	if got := alice.do(http.MethodPost, path(first.ID, "/publish"), nil, &out); got != http.StatusConflict || out.Error.Code != "conflict" || out.Error.Field != "slug" {
		t.Errorf("publishing a taken URL = %d %+v, want 409 conflict on slug", got, out.Error)
	}
	if diff := cmp.Diff(before, e.liveState(first.ID)); diff != "" {
		t.Errorf("failed publishing changed the content (-before +after):\n%s", diff)
	}
	if got := e.builds.count(e.tenantA); got != builds {
		t.Errorf("failed publishing scheduled a build")
	}
	var d detail
	alice.mustDo(http.MethodGet, path(first.ID, ""), nil, &d, http.StatusOK)
	if !d.PendingSettings || d.Slug == nil || *d.Slug != "taken" {
		t.Errorf("after a failed publish = %+v, want the slug still pending", d)
	}

	// Saving a URL already in use is refused right away.
	if got, code := alice.errorCode(http.MethodPatch, path(first.ID, ""), map[string]any{"slug": "taken"}); got != http.StatusConflict || code != "conflict" {
		t.Errorf("saving a taken URL = %d %s, want 409 conflict", got, code)
	}
}

// TestPendingSettingsPreview shows saved settings in the preview before
// they are published.
func TestPendingSettingsPreview(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	var cat struct{ ID int64 }
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "夏のお知らせ", "slug": "summer"}, &cat, http.StatusCreated)
	p := alice.newContent("post", "公開中の記事", true)
	at := time.Date(2025, 8, 5, 10, 0, 0, 0, time.UTC)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", p.ID), map[string]any{"categoryIds": []int64{cat.ID}, "publishedAt": at}, nil, http.StatusOK)

	status, _, page := alice.getPage(fmt.Sprintf("/api/contents/%d/preview/", p.ID))
	if status != http.StatusOK {
		t.Fatalf("preview = %d:\n%s", status, page)
	}
	for _, want := range []string{`href="/posts/category/summer/">夏のお知らせ</a>`, `datetime="2025-08-05"`} {
		if !strings.Contains(page, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
}
