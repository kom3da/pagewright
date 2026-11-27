package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// Ids are numbered per site, so two sites have the same ids. Every entry
// point acts on the asking site's item and never on the other site's.
func TestOverlappingIDs(t *testing.T) {
	e := newEnvOverlappingIDs(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	type site struct {
		name  string
		c     *client
		post  detail
		cat   struct{ ID int64 }
		media mediaOut
		image []byte
		link  struct {
			ID  int64
			URL string
		}
		sub struct {
			ID   int64
			Name string
		}
		poster string
	}
	sites := []*site{{name: "A", c: alice}, {name: "B", c: bob}}
	for i, s := range sites {
		s.post = s.c.newContent("post", s.name+"の記事", true)
		s.c.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": s.name, "slug": "news"}, &s.cat, http.StatusCreated)
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2+i, 2))); err != nil {
			t.Fatal(err)
		}
		s.image = b.Bytes()
		if got := s.c.upload(s.name+".png", s.image, s.name+"の画像", &s.media); got != http.StatusCreated {
			t.Fatalf("%s upload = %d", s.name, got)
		}
		s.c.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/preview-links", s.post.ID), map[string]any{"days": 1}, &s.link, http.StatusCreated)
		form := contactForm()
		s.poster = s.name + "さん"
		form.Set("name", s.poster)
		tenant := e.tenantA
		if s.name == "B" {
			tenant = e.tenantB
		}
		if status, page := e.postContact(tenant, form); status != http.StatusOK {
			t.Fatalf("%s contact = %d:\n%s", s.name, status, page)
		}
		var subs []struct {
			ID   int64
			Name string
		}
		s.c.mustDo(http.MethodGet, "/api/forms/submissions", nil, &subs, http.StatusOK)
		if len(subs) != 1 || subs[0].Name != s.poster {
			t.Fatalf("%s submissions = %+v", s.name, subs)
		}
		s.sub = subs[0]
	}
	a, b := sites[0], sites[1]
	if a.post.ID != b.post.ID || a.cat.ID != b.cat.ID || a.media.ID != b.media.ID || a.link.ID != b.link.ID || a.sub.ID != b.sub.ID {
		t.Fatalf("ids differ, so this test checks nothing: A %d/%d/%d/%d/%d, B %d/%d/%d/%d/%d",
			a.post.ID, a.cat.ID, a.media.ID, a.link.ID, a.sub.ID, b.post.ID, b.cat.ID, b.media.ID, b.link.ID, b.sub.ID)
	}

	for _, s := range sites {
		var got detail
		s.c.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", s.post.ID), nil, &got, http.StatusOK)
		if got.Title != s.name+"の記事" {
			t.Errorf("%s: content %d = %q", s.name, s.post.ID, got.Title)
		}
		var posts []struct{ Title string }
		s.c.mustDo(http.MethodGet, "/api/contents?type=post", nil, &posts, http.StatusOK)
		if len(posts) != 1 || posts[0].Title != s.name+"の記事" {
			t.Errorf("%s: posts = %+v", s.name, posts)
		}
		var cats []struct{ Name string }
		s.c.mustDo(http.MethodGet, "/api/categories", nil, &cats, http.StatusOK)
		if len(cats) != 1 || cats[0].Name != s.name {
			t.Errorf("%s: categories = %+v", s.name, cats)
		}
		var media []mediaOut
		s.c.mustDo(http.MethodGet, "/api/media", nil, &media, http.StatusOK)
		if len(media) != 1 || media[0].AltText != s.name+"の画像" {
			t.Errorf("%s: media = %+v", s.name, media)
		}
		// The file of media id N is the site's own file.
		if status, _, body := s.c.getPage(fmt.Sprintf("/api/media/%d/file", s.media.ID)); status != http.StatusOK || body != string(s.image) {
			t.Errorf("%s: media file %d = %d, own bytes %v", s.name, s.media.ID, status, body == string(s.image))
		}
		var links []struct{ ID int64 }
		s.c.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d/preview-links", s.post.ID), nil, &links, http.StatusOK)
		if len(links) != 1 {
			t.Errorf("%s: %d preview links, want 1", s.name, len(links))
		}
		// A preview link shows its own site's content of that id.
		if status, _, page := e.getPreview(s.link.URL, ""); status != http.StatusOK || !strings.Contains(page, s.name+"の記事") {
			t.Errorf("%s: preview = %d, has own title %v", s.name, status, strings.Contains(page, s.name+"の記事"))
		}
	}

	// Changing and deleting item N in A leaves B's item N alone.
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", a.post.ID),
		map[string]any{"title": "Aの記事（改）", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/preview-links/%d", a.link.ID), nil, nil, http.StatusNoContent)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/forms/submissions/%d", a.sub.ID), nil, nil, http.StatusNoContent)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/categories/%d", a.cat.ID), nil, nil, http.StatusNoContent)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/contents/%d", a.post.ID), nil, nil, http.StatusNoContent)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/media/%d", a.media.ID), nil, nil, http.StatusNoContent)

	var got detail
	bob.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", b.post.ID), nil, &got, http.StatusOK)
	if got.Title != "Bの記事" || got.Status != "published" || got.HasUnpublishedChanges {
		t.Errorf("B's content after A's changes = %+v", got)
	}
	var cats []json.RawMessage
	bob.mustDo(http.MethodGet, "/api/categories", nil, &cats, http.StatusOK)
	var subs []json.RawMessage
	bob.mustDo(http.MethodGet, "/api/forms/submissions", nil, &subs, http.StatusOK)
	var links []json.RawMessage
	bob.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d/preview-links", b.post.ID), nil, &links, http.StatusOK)
	if len(cats) != 1 || len(subs) != 1 || len(links) != 1 {
		t.Errorf("B after A's deletes: %d categories, %d submissions, %d links, want 1 each", len(cats), len(subs), len(links))
	}
	if status, _, body := bob.getPage(fmt.Sprintf("/api/media/%d/file", b.media.ID)); status != http.StatusOK || body != string(b.image) {
		t.Errorf("B's media file after A's delete = %d", status)
	}
	if status, _, page := e.getPreview(b.link.URL, ""); status != http.StatusOK || !strings.Contains(page, "Bの記事") {
		t.Errorf("B's preview after A's deletes = %d", status)
	}

	// A's next ids continue after its own, not after B's.
	next := alice.newContent("post", "Aの次の記事", false)
	if next.ID != a.post.ID+1 {
		t.Errorf("A's next content id = %d, want %d", next.ID, a.post.ID+1)
	}
}
