package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

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
	if _, err := env.Admin.Exec(ctx, "UPDATE contents SET scheduled_revision_id = $1, scheduled_at = $2 WHERE id = $3",
		revisionID, at, contentID); err != nil {
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
		FROM contents c JOIN routes r ON r.content_id = c.id AND r.kind = 'canonical'
		WHERE c.id = $1 AND c.scheduled_revision_id IS NULL`, contentID).Scan(&published, &publishedAt, &path)
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

	stale := []PublishScheduledArgs{
		// The schedule was moved an hour later: the old job finds it not due.
		{TenantID: tenant, ContentID: contentID, RevisionID: revisionID, At: time.Now().Add(-time.Minute)},
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
	if err := env.Admin.QueryRow(context.Background(), "SELECT published_revision_id FROM contents WHERE id = $1", contentID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != nil {
		t.Errorf("stale jobs published revision %d", *published)
	}
}
