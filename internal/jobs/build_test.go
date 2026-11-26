package jobs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// newRiver migrates River's tables into the river schema of the test
// database (as the owner, like pagewright migrate) and returns a client
// (not started) that runs as the app role, like pagewright serve.
func newRiver(t *testing.T, env *dbtest.Env, workers *river.Workers) *river.Client[pgx.Tx] {
	t.Helper()
	ctx := context.Background()
	migrator, err := rivermigrate.New(riverpgxv5.New(env.Admin), &rivermigrate.Config{Schema: "river"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("migrate river: %v", err)
	}
	client, err := river.NewClient(riverpgxv5.New(env.DB.Pool()), &river.Config{
		Schema:   "river",
		Queues:   map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers:  workers,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		TestOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// runUntil starts the client and waits for n jobs to complete.
func runUntil(t *testing.T, client *river.Client[pgx.Tx], n int) {
	t.Helper()
	ctx := context.Background()
	events, cancel := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobFailed)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Errorf("stop river: %v", err)
		}
	}()
	timeout := time.After(30 * time.Second)
	for range n {
		select {
		case ev := <-events:
			if ev.Kind != river.EventKindJobCompleted {
				t.Fatalf("job %d failed: %+v", ev.Job.ID, ev.Job.Errors)
			}
		case <-timeout:
			t.Fatal("timed out waiting for jobs")
		}
	}
}

func TestBuildSiteJob(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "ジョブ株式会社")
	opts := build.Options{OutDir: t.TempDir()}
	client := newRiver(t, env, NewWorkers(env.DB, opts))

	if _, err := client.Insert(context.Background(), BuildSiteArgs{TenantID: tenant}, nil); err != nil {
		t.Fatal(err)
	}
	runUntil(t, client, 1)

	b, err := os.ReadFile(filepath.Join(opts.OutDir, tenant.String(), "index.html"))
	if err != nil {
		t.Fatalf("site not built: %v", err)
	}
	if !strings.Contains(string(b), "ジョブ株式会社") {
		t.Errorf("index.html lacks the site name:\n%s", b)
	}
}

func TestBuildSiteJobCoalescesWaitingBuilds(t *testing.T) {
	env := dbtest.New(t)
	a, b := env.CreateTenant(t, "A"), env.CreateTenant(t, "B")

	var mu sync.Mutex
	builds := map[uuid.UUID]int{}
	w := NewBuildSiteWorker(env.DB, build.Options{})
	w.build = func(_ context.Context, _ *db.DB, tenantID uuid.UUID, _ build.Options) (build.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		builds[tenantID]++
		return build.Result{}, nil
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, w)
	client := newRiver(t, env, workers)

	ctx := context.Background()
	for _, tenant := range []uuid.UUID{a, a, b, a} {
		if _, err := client.Insert(ctx, BuildSiteArgs{TenantID: tenant}, nil); err != nil {
			t.Fatal(err)
		}
	}
	runUntil(t, client, 4)

	want := map[uuid.UUID]int{a: 1, b: 1}
	if diff := cmp.Diff(want, builds); diff != "" {
		t.Errorf("builds per tenant (-want +got):\n%s", diff)
	}
}

func TestBuildSiteJobWithoutTenantIsCancelled(t *testing.T) {
	w := NewBuildSiteWorker(nil, build.Options{})
	err := w.Work(context.Background(), &river.Job[BuildSiteArgs]{Args: BuildSiteArgs{}})
	if err == nil {
		t.Fatal("Work without tenant succeeded")
	}
}
