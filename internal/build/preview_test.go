package build_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// TestPreviewFindsContactPostWithoutBodies checks that a preview, which
// leaves most post bodies out, still finds a contact form in a post: the
// menu links to it as in the built site.
func TestPreviewFindsContactPostWithoutBodies(t *testing.T) {
	env := dbtest.New(t)
	s := newSeeder(t, env, "投稿だけのサイト", "carol@example.com")
	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	contact := s.content("post", ptr("contact"), nil)
	rev := s.revision(contact.ID, "お問い合わせ窓口",
		`{"type":"doc","content":[{"type":"blockquote","content":[{"type":"contactForm"}]}]}`)
	s.publish(contact.ID, rev.ID, base)
	// In between, so that the contact post is not the previewed one's
	// older neighbour.
	middle := s.content("post", ptr("middle"), nil)
	rev = s.revision(middle.ID, "あいだの記事", paragraph("本文"))
	s.publish(middle.ID, rev.ID, base.Add(time.Hour))
	other := s.content("post", ptr("other"), nil)
	rev = s.revision(other.ID, "ふつうの記事", paragraph("本文"))
	s.publish(other.ID, rev.ID, base.Add(2*time.Hour))

	page, err := build.RenderPreview(context.Background(), env.DB, s.tenant(), other.ID, build.PreviewOptions{
		PublicBaseURL: "https://example.jp", SelfURL: "https://cms.example.jp/p/x/", Notice: "プレビュー",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `href="/posts/contact/"`) {
		t.Errorf("preview has no link to the contact post:\n%s", page)
	}
}
