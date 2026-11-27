package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// scheduledPost creates a post with one revision scheduled at at.
func scheduledPost(t *testing.T, env *dbtest.Env, tenant uuid.UUID, at time.Time) (contentID, revisionID int64) {
	t.Helper()
	ctx := context.Background()
	if err := env.Admin.QueryRow(ctx,
		"INSERT INTO contents (tenant_id, number, type, slug) VALUES ($1, 7, 'post', 'later') RETURNING id",
		tenant).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	if err := env.Admin.QueryRow(ctx, `INSERT INTO content_revisions (tenant_id, content_id, title, body, body_schema_version)
		VALUES ($1, $2, '予約の記事', '{"type":"doc","content":[]}', 1) RETURNING id`, tenant, contentID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Admin.Exec(ctx, "UPDATE contents SET scheduled_revision_id = $1, scheduled_at = $2 WHERE tenant_id = $3 AND id = $4",
		revisionID, at, tenant, contentID); err != nil {
		t.Fatal(err)
	}
	return contentID, revisionID
}

func TestPublishScheduled(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "予約株式会社")
	at := time.Now().Add(-time.Second).UTC().Truncate(time.Second)
	contentID, revisionID := scheduledPost(t, env, tenant, at)
	client := newRiver(t, env, NewWorkers(env.DB, build.Options{OutDir: t.TempDir()}, nil))

	if _, err := client.Insert(context.Background(), PublishScheduledArgs{
		TenantID: tenant, ContentID: contentID, RevisionID: revisionID, At: at,
	}, nil); err != nil {
		t.Fatal(err)
	}
	runUntil(t, client, 2) // the publish, then the build it schedules

	var published *int64
	var publishedAt *time.Time
	var path string
	err := env.Admin.QueryRow(context.Background(), `
		SELECT c.published_revision_id, c.published_at, r.path
		FROM contents c JOIN routes r ON r.tenant_id = c.tenant_id AND r.content_id = c.id AND r.kind = 'canonical'
		WHERE c.tenant_id = $1 AND c.id = $2 AND c.scheduled_revision_id IS NULL`, tenant, contentID).Scan(&published, &publishedAt, &path)
	if err != nil {
		t.Fatalf("published content with route: %v", err)
	}
	if published == nil || *published != revisionID || publishedAt == nil || !publishedAt.Equal(at) || path != "/posts/later/" {
		t.Errorf("published=%v at=%v path=%q, want revision %d at %v on /posts/later/", published, publishedAt, path, revisionID, at)
	}
}

func TestPublishScheduledIgnoresStaleJobs(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "予約株式会社")
	contentID, revisionID := scheduledPost(t, env, tenant, time.Now().Add(time.Hour))
	client := newRiver(t, env, NewWorkers(env.DB, build.Options{OutDir: t.TempDir()}, nil))

	// (A job for the same revision that fires early snoozes until the
	// schedule is due; see TestPublishScheduledWaitsWhenEarly.)
	stale := []PublishScheduledArgs{
		// A newer revision was scheduled instead.
		{TenantID: tenant, ContentID: contentID, RevisionID: revisionID + 1000, At: time.Now().Add(-time.Minute)},
		// Another tenant's job cannot touch this content.
		{TenantID: uuid.New(), ContentID: contentID, RevisionID: revisionID, At: time.Now().Add(-time.Minute)},
	}
	for _, a := range stale {
		if _, err := client.Insert(context.Background(), a, nil); err != nil {
			t.Fatal(err)
		}
	}
	runUntil(t, client, len(stale))

	var published *int64
	if err := env.Admin.QueryRow(context.Background(), "SELECT published_revision_id FROM contents WHERE tenant_id = $1 AND id = $2", tenant, contentID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != nil {
		t.Errorf("stale jobs published revision %d", *published)
	}
}

func TestPublishScheduledWaitsWhenEarly(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "予約株式会社")
	at := time.Now().Add(3 * time.Second).UTC().Truncate(time.Second)
	contentID, revisionID := scheduledPost(t, env, tenant, at)
	client := newRiver(t, env, NewWorkers(env.DB, build.Options{OutDir: t.TempDir()}, nil))

	// The job fires early, as with a clock ahead of the database's.
	if _, err := client.Insert(context.Background(), PublishScheduledArgs{
		TenantID: tenant, ContentID: contentID, RevisionID: revisionID, At: at,
	}, &river.InsertOpts{ScheduledAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	runUntil(t, client, 2) // the publish after its snooze, then the build

	var published *int64
	if err := env.Admin.QueryRow(context.Background(), "SELECT published_revision_id FROM contents WHERE tenant_id = $1 AND id = $2", tenant, contentID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published == nil || *published != revisionID {
		t.Errorf("published = %v, want %d after waiting", published, revisionID)
	}
}

// TestPublishScheduledAppliesPending schedules changes of a live post
// whose settings wait: they go live with the scheduled revision. A URL
// taken meanwhile stays pending instead of blocking the publishing.
func TestPublishScheduledAppliesPending(t *testing.T) {
	for _, taken := range []bool{false, true} {
		env := dbtest.New(t)
		tenant := env.CreateTenant(t, "予約株式会社")
		ctx := context.Background()
		at := time.Now().Add(-time.Second).UTC().Truncate(time.Second)
		contentID, revisionID := scheduledPost(t, env, tenant, at)
		var catID, oldRevisionID int64
		if err := env.Admin.QueryRow(ctx, "INSERT INTO categories (tenant_id, name, slug) VALUES ($1, 'お知らせ', 'news') RETURNING id", tenant).Scan(&catID); err != nil {
			t.Fatal(err)
		}
		if err := env.Admin.QueryRow(ctx, `INSERT INTO content_revisions (tenant_id, content_id, title, body, body_schema_version)
			VALUES ($1, $2, '前の版', '{"type":"doc","content":[]}', 1) RETURNING id`, tenant, contentID).Scan(&oldRevisionID); err != nil {
			t.Fatal(err)
		}
		// Live with an older revision, and a slug and category waiting.
		pending := fmt.Sprintf(`{"slug": "moved", "categoryIds": [%d]}`, catID)
		setup := []struct {
			sql  string
			args []any
		}{
			{"UPDATE contents SET published_revision_id = $1, published_at = now() - interval '1 day', pending_settings = $2 WHERE tenant_id = $3 AND id = $4",
				[]any{oldRevisionID, pending, tenant, contentID}},
			{"INSERT INTO routes (tenant_id, path, kind, content_id) VALUES ($1, '/posts/later/', 'canonical', $2)", []any{tenant, contentID}},
		}
		if taken {
			setup = append(setup, struct {
				sql  string
				args []any
			}{"INSERT INTO routes (tenant_id, path, kind, category_id) VALUES ($1, '/posts/moved/', 'canonical', $2)", []any{tenant, catID}})
		}
		for _, s := range setup {
			if _, err := env.Admin.Exec(ctx, s.sql, s.args...); err != nil {
				t.Fatal(err)
			}
		}
		client := newRiver(t, env, NewWorkers(env.DB, build.Options{OutDir: t.TempDir()}, nil))
		if _, err := client.Insert(ctx, PublishScheduledArgs{TenantID: tenant, ContentID: contentID, RevisionID: revisionID, At: at}, nil); err != nil {
			t.Fatal(err)
		}
		runUntil(t, client, 2)

		var published int64
		var slug, left *string
		var path string
		var cats []int64
		if err := env.Admin.QueryRow(ctx, `
			SELECT c.published_revision_id, c.slug, c.pending_settings::text, r.path,
			  coalesce((SELECT array_agg(category_id) FROM content_categories WHERE tenant_id = c.tenant_id AND content_id = c.id), '{}')
			FROM contents c JOIN routes r ON r.tenant_id = c.tenant_id AND r.content_id = c.id AND r.kind = 'canonical'
			WHERE c.tenant_id = $1 AND c.id = $2`, tenant, contentID).Scan(&published, &slug, &left, &path, &cats); err != nil {
			t.Fatal(err)
		}
		got := fmt.Sprintf("rev=%t slug=%v pending=%v path=%s cats=%v", published == revisionID, deref(slug), deref(left), path, cats)
		want := fmt.Sprintf("rev=true slug=moved pending=<nil> path=/posts/moved/ cats=[%d]", catID)
		if taken {
			want = fmt.Sprintf(`rev=true slug=later pending={"slug": "moved"} path=/posts/later/ cats=[%d]`, catID)
		}
		if got != want {
			t.Errorf("taken=%t: %s, want %s", taken, got, want)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
