package build_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// BenchmarkRenderPreview previews one post of a site with 2,000 published
// posts and a few pages.
func BenchmarkRenderPreview(b *testing.B) {
	env := dbtest.New(b)
	tenant := env.CreateTenant(b, "大きなサイト")
	ctx := db.WithTenant(context.Background(), tenant)
	body := `{"type":"doc","content":[` + strings.Repeat(
		`{"type":"paragraph","content":[{"type":"text","text":"本文の段落です。いくつかの文が続きます。"}]},`, 8) +
		`{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"見出し"}]}]}`
	var target int64
	err := env.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		add := func(typ, slug string) (int64, error) {
			c, err := q.CreateContent(ctx, dbgen.CreateContentParams{Type: typ, Slug: &slug})
			if err != nil {
				return 0, err
			}
			r, err := q.CreateRevision(ctx, dbgen.CreateRevisionParams{
				ContentID: c.ID, Title: slug, Body: []byte(body), BodySchemaVersion: 1, Fields: []byte("{}"),
			})
			if err != nil {
				return 0, err
			}
			_, err = q.PublishRevision(ctx, dbgen.PublishRevisionParams{ContentID: c.ID, RevisionID: &r.ID})
			return c.ID, err
		}
		for i := range 5 {
			if _, err := add("page", fmt.Sprintf("page-%d", i)); err != nil {
				return err
			}
		}
		for i := range 2000 {
			id, err := add("post", fmt.Sprintf("post-%04d", i))
			if err != nil {
				return err
			}
			if i == 1000 {
				target = id
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	opts := build.PreviewOptions{PublicBaseURL: "https://example.jp", SelfURL: "https://cms.example.jp/p/x/", Notice: "プレビュー"}
	b.ResetTimer()
	for b.Loop() {
		if _, err := build.RenderPreview(context.Background(), env.DB, tenant, target, opts); err != nil {
			b.Fatal(err)
		}
	}
}
