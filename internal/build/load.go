package build

import (
	"context"
	"fmt"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
)

// snapshot is everything a build reads from the database.
type snapshot struct {
	tenant      dbgen.Tenant
	contents    []dbgen.ListPublishedContentsRow // newest first
	tree        []dbgen.ListContentTreeRow
	routes      []dbgen.Route
	categories  []dbgen.Category
	tags        []dbgen.Tag
	contentCats []dbgen.ListAllContentCategoriesRow
	contentTags []dbgen.ListAllContentTagsRow
	media       []dbgen.Medium
	menu        []dbgen.ListMenuItemsRow
}

// load reads the tenant's site in one transaction. RLS limits every query
// to the tenant in ctx. For a preview, post bodies a preview does not
// need are left out (nil; see ListPublishedContentsForPreview).
func load(ctx context.Context, d *db.DB, forPreview bool) (*snapshot, error) {
	var s snapshot
	err := d.InTenant(ctx, func(q *dbgen.Queries) error {
		var err error
		if s.tenant, err = q.GetCurrentTenant(ctx); err != nil {
			return fmt.Errorf("tenant: %w", err)
		}
		if s.menu, err = q.ListMenuItems(ctx); err != nil {
			return fmt.Errorf("menu: %w", err)
		}
		if forPreview {
			rows, err := q.ListPublishedContentsForPreview(ctx)
			if err != nil {
				return fmt.Errorf("contents: %w", err)
			}
			s.contents = make([]dbgen.ListPublishedContentsRow, len(rows))
			for i, r := range rows {
				s.contents[i] = dbgen.ListPublishedContentsRow(r)
			}
		} else if s.contents, err = q.ListPublishedContents(ctx); err != nil {
			return fmt.Errorf("contents: %w", err)
		}
		if s.tree, err = q.ListContentTree(ctx); err != nil {
			return fmt.Errorf("content tree: %w", err)
		}
		if s.routes, err = q.ListRoutes(ctx); err != nil {
			return fmt.Errorf("routes: %w", err)
		}
		if s.categories, err = q.ListCategories(ctx); err != nil {
			return fmt.Errorf("categories: %w", err)
		}
		if s.tags, err = q.ListTags(ctx); err != nil {
			return fmt.Errorf("tags: %w", err)
		}
		if s.contentCats, err = q.ListAllContentCategories(ctx); err != nil {
			return fmt.Errorf("content categories: %w", err)
		}
		if s.contentTags, err = q.ListAllContentTags(ctx); err != nil {
			return fmt.Errorf("content tags: %w", err)
		}
		if s.media, err = q.ListMedia(ctx); err != nil {
			return fmt.Errorf("media: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load site: %w", err)
	}
	return &s, nil
}
