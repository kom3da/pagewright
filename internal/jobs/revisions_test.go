package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// postWithRevisions creates a content with n revisions, the first created
// at start and each next one a second later. It returns the content id and
// the revision ids, oldest first.
func postWithRevisions(t *testing.T, env *dbtest.Env, tenant uuid.UUID, number, n int, start time.Time) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	var contentID int64
	if err := env.Admin.QueryRow(ctx, "INSERT INTO contents (tenant_id, number, type) VALUES ($1, $2, 'post') RETURNING id",
		tenant, number).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	rows, err := env.Admin.Query(ctx, `INSERT INTO content_revisions (tenant_id, content_id, body, body_schema_version, created_at)
		SELECT $1, $2, '{"type":"doc","content":[]}', 1, $3::timestamptz + g * interval '1 second'
		FROM generate_series(0, $4 - 1) g RETURNING id`, tenant, contentID, start, n)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return contentID, ids
}

// revisionIDs returns the ids of the content's revisions, oldest first.
func revisionIDs(t *testing.T, env *dbtest.Env, tenant uuid.UUID, contentID int64) []int64 {
	t.Helper()
	rows, err := env.Admin.Query(context.Background(),
		"SELECT id FROM content_revisions WHERE tenant_id = $1 AND content_id = $2 ORDER BY id", tenant, contentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestPruneRevisions(t *testing.T) {
	env := dbtest.New(t)
	ctx := context.Background()
	tenant := env.CreateTenant(t, "版株式会社")
	other := env.CreateTenant(t, "別の会社")
	now := time.Now()
	old := now.Add(-RevisionKeepFor - 24*time.Hour)

	// 300 old revisions (more than one batch); the first is published and
	// the second scheduled.
	busy, busyRevs := postWithRevisions(t, env, tenant, 1, 300, old)
	if _, err := env.Admin.Exec(ctx, `UPDATE contents SET published_revision_id = $1, published_at = now(),
		scheduled_revision_id = $2, scheduled_at = now() + interval '1 day' WHERE tenant_id = $3 AND id = $4`,
		busyRevs[0], busyRevs[1], tenant, busy); err != nil {
		t.Fatal(err)
	}
	// 80 revisions within the last 30 days: all stay.
	recent, recentRevs := postWithRevisions(t, env, tenant, 2, 80, now.Add(-time.Hour))
	// 60 revisions: the 5 oldest are old, the rest recent.
	mixed, mixedRevs := postWithRevisions(t, env, tenant, 3, 5, old)
	if _, err := env.Admin.Exec(ctx, `INSERT INTO content_revisions (tenant_id, content_id, body, body_schema_version)
		SELECT $1, $2, '{"type":"doc","content":[]}', 1 FROM generate_series(1, 55)`, tenant, mixed); err != nil {
		t.Fatal(err)
	}
	mixedAll := revisionIDs(t, env, tenant, mixed)
	// Another tenant with the same history is not touched.
	otherPost, otherRevs := postWithRevisions(t, env, other, 1, 300, old)

	deleted, more, err := PruneRevisions(ctx, env.DB, tenant, now)
	if err != nil {
		t.Fatal(err)
	}
	wantBusy := append([]int64{busyRevs[0], busyRevs[1]}, busyRevs[len(busyRevs)-RevisionKeepNewest:]...)
	if diff := cmp.Diff(wantBusy, revisionIDs(t, env, tenant, busy)); diff != "" {
		t.Errorf("old revisions kept (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(recentRevs, revisionIDs(t, env, tenant, recent)); diff != "" {
		t.Errorf("recent revisions kept (-want +got):\n%s", diff)
	}
	// Of the 60, the 10 beyond the newest 50 are the 5 old ones and 5
	// recent ones: only the old ones go.
	if diff := cmp.Diff(mixedAll[len(mixedRevs):], revisionIDs(t, env, tenant, mixed)); diff != "" {
		t.Errorf("mixed revisions kept (-want +got):\n%s", diff)
	}
	if want := int64(300 - len(wantBusy) + len(mixedRevs)); deleted != want {
		t.Errorf("deleted = %d, want %d", deleted, want)
	}
	if !more {
		t.Error("more = false, want true while contents have more than the newest revisions")
	}
	if diff := cmp.Diff(otherRevs, revisionIDs(t, env, other, otherPost)); diff != "" {
		t.Errorf("other tenant's revisions changed (-want +got):\n%s", diff)
	}

	// Running again deletes nothing.
	if deleted, _, err := PruneRevisions(ctx, env.DB, tenant, now); err != nil || deleted != 0 {
		t.Errorf("second run deleted %d, %v; want 0", deleted, err)
	}
}

func TestPruneRevisionsNothingMore(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "版株式会社")
	postWithRevisions(t, env, tenant, 1, 3, time.Now().Add(-RevisionKeepFor-time.Hour))
	deleted, more, err := PruneRevisions(context.Background(), env.DB, tenant, time.Now())
	if err != nil || deleted != 0 || more {
		t.Errorf("PruneRevisions = %d, %v, %v; want 0, false, nil", deleted, more, err)
	}
}

func TestPruneRevisionsJobSchedulesNext(t *testing.T) {
	env := dbtest.New(t)
	ctx := context.Background()
	tenant := env.CreateTenant(t, "版株式会社")
	post, revs := postWithRevisions(t, env, tenant, 1, RevisionKeepNewest+10, time.Now().Add(-RevisionKeepFor-time.Hour))
	client := newRiver(t, env, NewWorkers(env.DB, build.Options{OutDir: t.TempDir()}, nil))

	if _, err := client.Insert(ctx, PruneRevisionsArgs{TenantID: tenant, At: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	runUntil(t, client, 1)

	if diff := cmp.Diff(revs[10:], revisionIDs(t, env, tenant, post)); diff != "" {
		t.Errorf("revisions kept (-want +got):\n%s", diff)
	}
	// No content has more than the newest revisions left: no next run.
	res, err := client.JobList(ctx, river.NewJobListParams().Kinds(PruneRevisionsArgs{}.Kind()).States("scheduled"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Jobs) != 0 {
		t.Errorf("%d runs scheduled after pruning everything, want 0", len(res.Jobs))
	}

	// With a protected old revision beyond the newest, the next day's run
	// is scheduled, once.
	if _, err := env.Admin.Exec(ctx, `INSERT INTO content_revisions (tenant_id, content_id, body, body_schema_version)
		VALUES ($1, $2, '{"type":"doc","content":[]}', 1)`, tenant, post); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Admin.Exec(ctx, "UPDATE contents SET published_revision_id = $1, published_at = now() WHERE tenant_id = $2 AND id = $3",
		revs[10], tenant, post); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insert(ctx, PruneRevisionsArgs{TenantID: tenant, At: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	runUntil(t, client, 1)
	res, err = client.JobList(ctx, river.NewJobListParams().Kinds(PruneRevisionsArgs{}.Kind()).States("scheduled"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Jobs) != 1 {
		t.Fatalf("%d runs scheduled, want 1", len(res.Jobs))
	}
	if _, err := client.Insert(ctx, NextPruneRevisions(tenant, time.Now()), nil); err != nil {
		t.Fatal(err)
	}
	res, err = client.JobList(ctx, river.NewJobListParams().Kinds(PruneRevisionsArgs{}.Kind()).States("scheduled"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Jobs) != 1 {
		t.Errorf("%d runs scheduled after a save, want still 1", len(res.Jobs))
	}
}

func TestNextPruneRevisions(t *testing.T) {
	id := uuid.New()
	jst := time.FixedZone("JST", 9*3600)
	for _, tc := range []struct{ now, want time.Time }{
		{time.Date(2026, 11, 27, 10, 0, 0, 0, jst), time.Date(2026, 11, 28, 3, 0, 0, 0, jst)},
		{time.Date(2026, 11, 27, 2, 59, 0, 0, jst), time.Date(2026, 11, 27, 3, 0, 0, 0, jst)},
		{time.Date(2026, 11, 27, 3, 0, 0, 0, jst), time.Date(2026, 11, 28, 3, 0, 0, 0, jst)},
	} {
		got := NextPruneRevisions(id, tc.now)
		if !got.At.Equal(tc.want) || got.TenantID != id {
			t.Errorf("NextPruneRevisions(%v) = %v, want %v", tc.now, got.At, tc.want)
		}
	}
}
