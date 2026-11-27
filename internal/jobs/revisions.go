package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
)

// Revision retention: every revision of the last RevisionKeepFor stays,
// and beyond that the RevisionKeepNewest newest of each content. The
// published, scheduled and latest revisions always stay.
const (
	RevisionKeepFor    = 30 * 24 * time.Hour
	RevisionKeepNewest = 50
	// revisionPruneBatch bounds one delete, so each transaction (and the
	// row locks of up to 4 MB revisions) stays short.
	revisionPruneBatch = 200
	// revisionPruneHourUTC is when the daily run starts: 03:00 in Japan.
	revisionPruneHourUTC = 18
)

// PruneRevisionsArgs deletes the revisions of one tenant that the
// retention policy lets go. At is the run time (a day at
// revisionPruneHourUTC), so there is at most one waiting job per tenant
// and day; build it with NextPruneRevisions.
//
// River's periodic jobs cannot be per tenant, and the app cannot list
// tenants, so the job is self-scheduling instead: saving a draft inserts
// the next run, and a run inserts the following day's while some content
// has more than RevisionKeepNewest revisions (otherwise nothing can become
// deletable until the next save).
type PruneRevisionsArgs struct {
	TenantID uuid.UUID `json:"tenant_id"`
	At       time.Time `json:"at"`
}

// Kind implements river.JobArgs.
func (PruneRevisionsArgs) Kind() string { return "prune_revisions" }

// InsertOpts runs the job at At and drops duplicates of the same run.
func (a PruneRevisionsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{ScheduledAt: a.At, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// NextPruneRevisions returns the tenant's first daily run after now.
func NextPruneRevisions(tenantID uuid.UUID, now time.Time) PruneRevisionsArgs {
	now = now.UTC()
	at := time.Date(now.Year(), now.Month(), now.Day(), revisionPruneHourUTC, 0, 0, 0, time.UTC)
	if !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	return PruneRevisionsArgs{TenantID: tenantID, At: at}
}

// PruneRevisionsWorker runs PruneRevisions and schedules the next run.
type PruneRevisionsWorker struct {
	river.WorkerDefaults[PruneRevisionsArgs]
	db *db.DB
}

// Work implements river.Worker.
func (w *PruneRevisionsWorker) Work(ctx context.Context, job *river.Job[PruneRevisionsArgs]) error {
	tenantID := job.Args.TenantID
	if tenantID == uuid.Nil {
		return river.JobCancel(errors.New("prune_revisions: no tenant_id"))
	}
	n, more, err := PruneRevisions(ctx, w.db, tenantID, time.Now())
	if n > 0 {
		slog.InfoContext(ctx, "removed old revisions", "tenant_id", tenantID, "count", n)
	}
	if err != nil {
		return fmt.Errorf("prune_revisions: %w", err)
	}
	if !more {
		return nil
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return err
	}
	// Count from the later of now and the schedule: a run woken a little
	// early must not pick its own slot (a duplicate of itself is dropped).
	from := time.Now()
	if job.ScheduledAt.After(from) {
		from = job.ScheduledAt
	}
	from = from.Add(time.Minute)
	_, err = client.Insert(ctx, NextPruneRevisions(tenantID, from), nil)
	return err
}

// PruneRevisions deletes the tenant's revisions older than RevisionKeepFor
// (counted from now) that are not among the RevisionKeepNewest newest of
// their content, nor published or scheduled. It deletes in batches, one
// transaction each, and reports how many it deleted and whether a later
// run may find more to delete.
func PruneRevisions(ctx context.Context, d *db.DB, tenantID uuid.UUID, now time.Time) (deleted int64, more bool, err error) {
	ctx = db.WithTenant(ctx, tenantID)
	params := dbgen.DeleteOldRevisionsParams{
		KeepNewest: RevisionKeepNewest, Cutoff: now.Add(-RevisionKeepFor), BatchSize: revisionPruneBatch,
	}
	for {
		var n int64
		err := d.InTenant(ctx, func(q *dbgen.Queries) error {
			var err error
			n, err = q.DeleteOldRevisions(ctx, params)
			return err
		})
		if err != nil {
			return deleted, false, err
		}
		deleted += n
		if n < revisionPruneBatch {
			break
		}
	}
	err = d.InTenant(ctx, func(q *dbgen.Queries) error {
		var err error
		more, err = q.HasRevisionsBeyond(ctx, RevisionKeepNewest)
		return err
	})
	return deleted, more, err
}
