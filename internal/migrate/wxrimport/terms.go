package wxrimport

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/migrate/model"
)

// terms returns the declared terms plus terms that items use but the
// export does not declare (partial exports), parents first.
func terms(declared []model.Term, items []model.Item, used func(model.Item) []string) []model.Term {
	out := slices.Clone(declared)
	have := map[string]bool{}
	for _, t := range declared {
		have[t.Slug] = true
	}
	for _, it := range items {
		for _, slug := range used(it) {
			if have[slug] || skipReason(it) != "" {
				continue
			}
			have[slug] = true
			name := it.TermNames[slug]
			if name == "" {
				name = slug
			}
			out = append(out, model.Term{Slug: slug, RawSlug: slug, Name: name})
		}
	}
	bySlug := map[string]model.Term{}
	for _, t := range out {
		bySlug[t.Slug] = t
	}
	depth := func(t model.Term) int {
		d := 0
		for seen := map[string]bool{}; t.ParentSlug != "" && !seen[t.ParentSlug]; d++ {
			seen[t.ParentSlug] = true
			p, ok := bySlug[t.ParentSlug]
			if !ok {
				break
			}
			t = p
		}
		return d
	}
	slices.SortStableFunc(out, func(a, b model.Term) int { return depth(a) - depth(b) })
	return out
}

func (im *importer) importCategories(ctx context.Context) {
	cats := terms(im.exp.Categories, im.exp.Items, func(it model.Item) []string { return it.Categories })
	bySlug := map[string]model.Term{}
	for _, t := range cats {
		bySlug[t.Slug] = t
	}
	// WordPress's default category URL: /category/parent/child/.
	wpPath := func(t model.Term) string {
		segs := []string{t.Slug}
		seen := map[string]bool{t.Slug: true}
		for p, ok := bySlug[t.ParentSlug]; ok && !seen[p.Slug]; p, ok = bySlug[p.ParentSlug] {
			seen[p.Slug] = true
			segs = append([]string{p.Slug}, segs...)
		}
		return sitePrefix(im.exp.Site.URL) + pathOf(append([]string{"category"}, segs...)...)
	}
	for i, t := range cats {
		im.importTerm(ctx, KindCategory, t, int64(i), wpPath(t))
	}
	if len(cats) > 0 {
		im.rep.Notes = append(im.rep.Notes, "カテゴリとタグの旧URLは、WordPress の初期設定（/category/…/、/tag/…/）だったものとして扱いました")
	}
}

func (im *importer) importTags(ctx context.Context) {
	tags := terms(im.exp.Tags, im.exp.Items, func(it model.Item) []string { return it.Tags })
	for _, t := range tags {
		im.importTerm(ctx, KindTag, t, 0, sitePrefix(im.exp.Site.URL)+pathOf("tag", t.Slug))
	}
}

// importTerm creates a category or tag with its URL. With KeepURLs the
// WordPress URL is canonical; otherwise /posts/category/{slug}/ (or
// /posts/tag/{slug}/) is, and the WordPress URL redirects to it.
func (im *importer) importTerm(ctx context.Context, kind string, t model.Term, position int64, wpPath string) {
	e := Entry{Kind: kind, SourceID: t.SourceID, Title: t.Name}
	ids := im.categories
	pwBase := "category"
	if kind == KindTag {
		ids, pwBase = im.tags, "tag"
	}
	if t.Slug == "" || strings.Contains(t.Slug, "/") {
		e.Outcome, e.Reason = Failed, "スラッグ「"+t.RawSlug+"」は使えません"
		im.rep.add(e)
		return
	}
	var redirects int
	err := im.unit(func(q *dbgen.Queries) error {
		e.Warnings, redirects = nil, 0
		var target dbgen.CreateRouteParams
		switch kind {
		case KindCategory:
			if c, err := q.GetCategoryBySlug(ctx, t.Slug); err == nil {
				ids[t.Slug] = c.ID
				e.Outcome, e.Reason = Existing, "同じスラッグ（"+t.Slug+"）のカテゴリがあります"
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			var parentID *int64
			if t.ParentSlug != "" {
				if id, ok := ids[t.ParentSlug]; ok {
					parentID = &id
				} else {
					e.warn(WarnParent, "親カテゴリ「"+t.ParentSlug+"」が取り込まれていないため、最上位に置きました", "")
				}
			}
			c, err := q.CreateCategory(ctx, dbgen.CreateCategoryParams{ParentID: parentID, Name: t.Name, Slug: t.Slug, Position: position})
			if err != nil {
				return fmt.Errorf("create category: %w", err)
			}
			ids[t.Slug] = c.ID
			target.CategoryID = &c.ID
		case KindTag:
			if tg, err := q.GetTagBySlug(ctx, t.Slug); err == nil {
				ids[t.Slug] = tg.ID
				e.Outcome, e.Reason = Existing, "同じスラッグ（"+t.Slug+"）のタグがあります"
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			tg, err := q.CreateTag(ctx, dbgen.CreateTagParams{Name: t.Name, Slug: t.Slug})
			if err != nil {
				return fmt.Errorf("create tag: %w", err)
			}
			ids[t.Slug] = tg.ID
			target.TagID = &tg.ID
		}

		pw := pathOf("posts", pwBase, t.Slug)
		candidates := []string{pw}
		if im.opts.KeepURLs {
			candidates = []string{wpPath, pw}
		}
		canonical, err := im.addRoute(ctx, q, "canonical", candidates, target, &e)
		if err != nil {
			return err
		}
		e.Path = canonical
		if !im.opts.KeepURLs && wpPath != canonical {
			r, err := im.addRoute(ctx, q, "redirect", []string{wpPath}, target, &e)
			if err != nil {
				return err
			}
			if r != "" {
				redirects++
			}
		}
		e.Outcome = Created
		return nil
	})
	if err != nil {
		delete(ids, t.Slug)
		e.Outcome, e.Reason, e.Path = Failed, err.Error(), ""
	} else if e.Outcome == Created {
		if e.Path != "" {
			im.rep.Routes++
		}
		im.rep.Redirects += redirects
	}
	im.rep.add(e)
}
