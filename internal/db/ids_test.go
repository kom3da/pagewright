package db_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
)

// perTenant is what TestPerTenantIDs creates in each of two fresh tenants.
type perTenant struct {
	ctx                                  context.Context
	tenant                               uuid.UUID
	content, rev, cat, tag, media, route int64
	submission, link, menu               int64
	token                                []byte
}

// newBareTenant inserts a tenant with no rows yet, so its ids start at 1.
func (e *testEnv) newBareTenant(t *testing.T, name string) (uuid.UUID, context.Context) {
	t.Helper()
	id := uuid.New()
	if _, err := e.admin.Exec(context.Background(), "INSERT INTO tenants (id, name) VALUES ($1, $2)", id, name); err != nil {
		t.Fatal(err)
	}
	return id, db.WithTenant(context.Background(), id)
}

func (e *testEnv) fillTenant(t *testing.T, name string) perTenant {
	t.Helper()
	tenant, ctx := e.newBareTenant(t, name)
	d := perTenant{ctx: ctx, tenant: tenant}
	sum := sha256.Sum256([]byte(name))
	d.token = sum[:]
	err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
		c, err := q.CreateContent(ctx, dbgen.CreateContentParams{Type: "post"})
		if err != nil {
			return err
		}
		r, err := q.CreateRevision(ctx, dbgen.CreateRevisionParams{
			ContentID: c.ID, Title: name, Body: []byte(`{"type":"doc","content":[]}`), BodySchemaVersion: 1, Fields: []byte(`{}`),
		})
		if err != nil {
			return err
		}
		cat, err := q.CreateCategory(ctx, dbgen.CreateCategoryParams{Name: name, Slug: "cat"})
		if err != nil {
			return err
		}
		tag, err := q.CreateTag(ctx, dbgen.CreateTagParams{Name: name, Slug: "tag"})
		if err != nil {
			return err
		}
		m, err := q.CreateMedia(ctx, dbgen.CreateMediaParams{StorageKey: "photo.jpg", Filename: name + ".jpg", ContentType: "image/jpeg", SizeBytes: 1})
		if err != nil {
			return err
		}
		rt, err := q.CreateRoute(ctx, dbgen.CreateRouteParams{Path: "/" + name + "/", Kind: "canonical", ContentID: &c.ID})
		if err != nil {
			return err
		}
		sub, err := q.CreateFormSubmission(ctx, dbgen.CreateFormSubmissionParams{Name: name, Email: "x@example.com", Message: name})
		if err != nil {
			return err
		}
		link, err := q.CreatePreviewLink(ctx, dbgen.CreatePreviewLinkParams{TokenHash: d.token, ContentID: c.ID, ExpiresAt: time.Now().Add(time.Hour)})
		if err != nil {
			return err
		}
		if err := q.AddMenuItem(ctx, dbgen.AddMenuItemParams{Position: 0, ContentID: &c.ID}); err != nil {
			return err
		}
		menu, err := q.ListMenuItems(ctx)
		if err != nil || len(menu) != 1 {
			return fmt.Errorf("menu: %v %v", menu, err)
		}
		if err := q.RecordBuild(ctx, dbgen.RecordBuildParams{StartedAt: time.Now(), Warnings: []byte(`[]`)}); err != nil {
			return err
		}
		d.content, d.rev, d.cat, d.tag, d.media, d.route = c.ID, r.ID, cat.ID, tag.ID, m.ID, rt.ID
		d.submission, d.link, d.menu = sub.ID, link.ID, menu[0].ID
		return nil
	})
	if err != nil {
		t.Fatalf("fill %s: %v", name, err)
	}
	return d
}

// Two tenants number their rows independently, so both have id 1 in every
// table. Each sees and changes only its own row 1.
func TestPerTenantIDs(t *testing.T) {
	e := newTestEnv(t)
	c := e.fillTenant(t, "c")
	d := e.fillTenant(t, "d")

	for _, x := range []perTenant{c, d} {
		got := []int64{x.content, x.rev, x.cat, x.tag, x.media, x.route, x.submission, x.link, x.menu}
		if diff := cmp.Diff([]int64{1, 1, 1, 1, 1, 1, 1, 1, 1}, got); diff != "" {
			t.Errorf("ids of tenant %v (-want +got):\n%s", x.tenant, diff)
		}
	}

	// Lookups by the same id return each tenant's own row.
	for _, x := range []struct {
		name string
		d    perTenant
	}{{"c", c}, {"d", d}} {
		err := e.db.InTenant(x.d.ctx, func(q *dbgen.Queries) error {
			ctx := x.d.ctx
			rev, err := q.GetLatestRevision(ctx, 1)
			if err != nil || rev.Title != x.name {
				t.Errorf("%s: latest revision of content 1 = %q (%v)", x.name, rev.Title, err)
			}
			cat, err := q.GetCategory(ctx, 1)
			if err != nil || cat.Name != x.name {
				t.Errorf("%s: category 1 = %q (%v)", x.name, cat.Name, err)
			}
			tag, err := q.GetTag(ctx, 1)
			if err != nil || tag.Name != x.name {
				t.Errorf("%s: tag 1 = %q (%v)", x.name, tag.Name, err)
			}
			m, err := q.GetMedia(ctx, 1)
			if err != nil || m.Filename != x.name+".jpg" {
				t.Errorf("%s: media 1 = %q (%v)", x.name, m.Filename, err)
			}
			sub, err := q.GetFormSubmission(ctx, 1)
			if err != nil || sub.Name != x.name {
				t.Errorf("%s: submission 1 = %q (%v)", x.name, sub.Name, err)
			}
			routes, err := q.ListRoutes(ctx)
			if err != nil || len(routes) != 1 || routes[0].Path != "/"+x.name+"/" {
				t.Errorf("%s: routes = %+v (%v)", x.name, routes, err)
			}
			subs, err := q.ListFormSubmissions(ctx, dbgen.ListFormSubmissionsParams{})
			if err != nil || len(subs) != 1 {
				t.Errorf("%s: %d submissions (%v), want 1", x.name, len(subs), err)
			}
			builds, err := q.ListBuilds(ctx, 10)
			if err != nil || len(builds) != 1 {
				t.Errorf("%s: %d builds (%v), want 1", x.name, len(builds), err)
			}
			links, err := q.ListPreviewLinks(ctx, 1)
			if err != nil || len(links) != 1 {
				t.Errorf("%s: %d preview links (%v), want 1", x.name, len(links), err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A preview token names its tenant along with the content id.
	for _, x := range []perTenant{c, d} {
		var link dbgen.LookupPreviewLinkRow
		err := pgx.BeginFunc(context.Background(), e.app, func(tx pgx.Tx) error {
			var err error
			link, err = dbgen.New(tx).LookupPreviewLink(context.Background(), x.token)
			return err
		})
		if err != nil || link.TenantID != x.tenant || link.ContentID != 1 {
			t.Errorf("lookup of %v's token = %+v (%v)", x.tenant, link, err)
		}
	}

	// Deleting row 1 in c leaves d's row 1 alone.
	err := e.db.InTenant(c.ctx, func(q *dbgen.Queries) error {
		for _, del := range []func() (int64, error){
			func() (int64, error) { return q.DeleteFormSubmission(c.ctx, 1) },
			func() (int64, error) { return q.DeletePreviewLink(c.ctx, 1) },
			func() (int64, error) { return q.DeleteTag(c.ctx, 1) },
			func() (int64, error) { return q.DeleteContent(c.ctx, 1) },
		} {
			if n, err := del(); err != nil || n != 1 {
				return fmt.Errorf("delete: n=%d err=%v", n, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"contents", "content_revisions", "tags", "routes", "form_submissions", "preview_links", "menu_items"} {
		var left []uuid.UUID
		rows, err := e.admin.Query(context.Background(), "SELECT tenant_id FROM "+table+" WHERE id = 1 AND tenant_id IN ($1, $2)", c.tenant, d.tenant)
		if err == nil {
			left, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		}
		if err != nil || !slices.Equal(left, []uuid.UUID{d.tenant}) {
			t.Errorf("%s row 1 left in %v (%v), want only d's", table, left, err)
		}
	}
}

// Concurrent inserts into one tenant get distinct, gapless ids; another
// tenant's inserts at the same time do not take numbers from it.
func TestPerTenantIDsConcurrent(t *testing.T) {
	e := newTestEnv(t)
	_, ctxC := e.newBareTenant(t, "c")
	_, ctxD := e.newBareTenant(t, "d")

	const n = 20
	var mu sync.Mutex
	got := map[context.Context][]int64{}
	var wg sync.WaitGroup
	errs := make(chan error, 2*n)
	for i := range 2 * n {
		ctx := ctxC
		if i%2 == 1 {
			ctx = ctxD
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
				s, err := q.CreateFormSubmission(ctx, dbgen.CreateFormSubmissionParams{Name: "x", Email: "x@example.com", Message: "x"})
				if err != nil {
					return err
				}
				mu.Lock()
				got[ctx] = append(got[ctx], s.ID)
				mu.Unlock()
				return nil
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	want := make([]int64, n)
	for i := range want {
		want[i] = int64(i + 1)
	}
	for _, ctx := range []context.Context{ctxC, ctxD} {
		ids := got[ctx]
		slices.Sort(ids)
		if diff := cmp.Diff(want, ids); diff != "" {
			t.Errorf("ids (-want +got):\n%s", diff)
		}
	}
}

// An explicit id (an import, or the owner fixing data) moves the counter
// past it; a lower explicit id leaves the counter alone. A rolled-back
// insert gives its id back.
func TestExplicitIDMovesCounter(t *testing.T) {
	e := newTestEnv(t)
	tenant, ctx := e.newBareTenant(t, "c")

	if _, err := e.admin.Exec(context.Background(), "INSERT INTO tags (tenant_id, id, name, slug) VALUES ($1, 50, 'x', 'x')", tenant); err != nil {
		t.Fatal(err)
	}
	create := func(slug string) int64 {
		t.Helper()
		var id int64
		err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
			tag, err := q.CreateTag(ctx, dbgen.CreateTagParams{Name: slug, Slug: slug})
			id = tag.ID
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if id := create("a"); id != 51 {
		t.Errorf("after explicit 50: id = %d, want 51", id)
	}
	if _, err := e.admin.Exec(context.Background(), "INSERT INTO tags (tenant_id, id, name, slug) VALUES ($1, 10, 'y', 'y')", tenant); err != nil {
		t.Fatal(err)
	}
	if id := create("b"); id != 52 {
		t.Errorf("after explicit 10: id = %d, want 52", id)
	}

	rollback := errors.New("rollback")
	err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
		if _, err := q.CreateTag(ctx, dbgen.CreateTagParams{Name: "c", Slug: "c"}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if id := create("d"); id != 53 {
		t.Errorf("after a rolled-back insert: id = %d, want 53", id)
	}

	// An explicit id that is already taken in the tenant is a key conflict.
	err = e.execInTenant(tenant, "INSERT INTO tags (id, name, slug) VALUES (51, 'z', 'z')")
	if code := pgCode(err); code != "23505" {
		t.Errorf("explicit taken id: err = %v, want SQLSTATE 23505", err)
	}
}

// The counters are under RLS like everything else: the app sees and moves
// only its own tenant's. A row for another tenant, or with no tenant set,
// is refused with the RLS error of the table written to.
func TestIDCountersAreIsolated(t *testing.T) {
	e := newTestEnv(t)
	if n := e.countInTenant(t, e.tenantA, "SELECT count(*) FROM tenant_id_counters WHERE tenant_id <> $1", e.tenantA); n != 0 {
		t.Errorf("A sees %d counters of other tenants", n)
	}
	wantRLSError := func(what string, err error) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" ||
			pgErr.Message != `new row violates row-level security policy for table "tags"` {
			t.Errorf("%s: err = %v, want the RLS error of tags", what, err)
		}
	}
	empty, _ := e.newBareTenant(t, "empty")
	for _, other := range []uuid.UUID{e.tenantB, empty} {
		for _, q := range []string{
			"INSERT INTO tags (tenant_id, name, slug) VALUES ($1, 'x', 'x')",
			"INSERT INTO tags (tenant_id, id, name, slug) VALUES ($1, 5000, 'x', 'x')",
		} {
			wantRLSError(fmt.Sprintf("%q for %v", q, other), e.execInTenant(e.tenantA, q, other))
		}
	}
	_, err := e.app.Exec(context.Background(), "INSERT INTO tags (name, slug) VALUES ('x', 'x')")
	wantRLSError("insert with no tenant set", err)

	var last int64
	if err := e.admin.QueryRow(context.Background(), "SELECT last_id FROM tenant_id_counters WHERE tenant_id = $1 AND name = 'tags'", e.tenantB).Scan(&last); err != nil || last != e.tagB.ID {
		t.Errorf("B's tag counter = %d (%v), want %d", last, err, e.tagB.ID)
	}
	if err := e.execInTenant(e.tenantA, "UPDATE tenant_id_counters SET last_id = 0"); err != nil {
		t.Fatal(err)
	}
	if err := e.admin.QueryRow(context.Background(), "SELECT last_id FROM tenant_id_counters WHERE tenant_id = $1 AND name = 'tags'", e.tenantB).Scan(&last); err != nil || last != e.tagB.ID {
		t.Errorf("B's tag counter after A's UPDATE = %d (%v), want %d", last, err, e.tagB.ID)
	}
}
