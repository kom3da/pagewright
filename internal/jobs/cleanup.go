package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/kom3da/pagewright/internal/db"
)

// CleanupAuthArgs removes expired login links and sessions.
type CleanupAuthArgs struct{}

// Kind implements river.JobArgs.
func (CleanupAuthArgs) Kind() string { return "cleanup_auth" }

// CleanupAuthWorker runs auth_cleanup().
type CleanupAuthWorker struct {
	river.WorkerDefaults[CleanupAuthArgs]
	db *db.DB
}

// Work implements river.Worker.
func (w *CleanupAuthWorker) Work(ctx context.Context, _ *river.Job[CleanupAuthArgs]) error {
	n, err := w.db.Queries().AuthCleanup(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		slog.InfoContext(ctx, "removed expired login links and sessions", "count", n)
	}
	return nil
}

// PeriodicJobs are the jobs River runs on a schedule.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupAuthArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),
	}
}
