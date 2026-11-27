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

	"github.com/kom3da/pagewright/internal/content"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/routing"
)

// PublishScheduledArgs publishes a scheduled revision at At. Insert it with
// InsertOpts.ScheduledAt = At. A job whose revision was rescheduled,
// cancelled or already published does nothing, so stale jobs are harmless
// and rescheduling just inserts another job.
type PublishScheduledArgs struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	ContentID  int64     `json:"content_id"`
	RevisionID int64     `json:"revision_id"`
	At         time.Time `json:"at"`
}

// Kind implements river.JobArgs.
func (PublishScheduledArgs) Kind() string { return "publish_scheduled" }

// InsertOpts runs the job at At and drops duplicates of the same schedule.
func (a PublishScheduledArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{ScheduledAt: a.At, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// PublishScheduledWorker publishes due revisions and schedules a build.
type PublishScheduledWorker struct {
	river.WorkerDefaults[PublishScheduledArgs]
	db *db.DB
}

// NewPublishScheduledWorker returns the worker.
func NewPublishScheduledWorker(d *db.DB) *PublishScheduledWorker {
	return &PublishScheduledWorker{db: d}
}

// Work implements river.Worker.
func (w *PublishScheduledWorker) Work(ctx context.Context, job *river.Job[PublishScheduledArgs]) error {
	a := job.Args
	ctx = db.WithTenant(ctx, a.TenantID)
	return w.db.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		_, err := q.PublishScheduled(ctx, dbgen.PublishScheduledParams{ContentID: a.ContentID, RevisionID: &a.RevisionID})
		if errors.Is(err, pgx.ErrNoRows) {
			// Still scheduled but not due by the database clock (River
			// wakes jobs by this process's clock): wait instead of losing it.
			c, err := q.GetContent(ctx, a.ContentID)
			if err == nil && c.ScheduledRevisionID != nil && *c.ScheduledRevisionID == a.RevisionID &&
				c.ScheduledAt != nil && c.ScheduledAt.After(time.Now().Add(-time.Minute)) {
				return river.JobSnooze(max(time.Until(*c.ScheduledAt), time.Second))
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("publish scheduled: %w", err)
		}
		// Settings saved while the content was live go out with the
		// changes. A URL that cannot be applied stays pending instead of
		// blocking the publishing.
		c, err := q.GetContent(ctx, a.ContentID)
		if err != nil {
			return err
		}
		skipped, err := content.ApplyPendingExceptURL(ctx, tx, q, c, true)
		if err != nil {
			return fmt.Errorf("apply pending settings: %w", err)
		}
		if skipped != nil {
			slog.WarnContext(ctx, "scheduled content published without its new URL", "tenant_id", a.TenantID, "content_id", a.ContentID, "err", skipped)
		}
		// A URL clash must not block publishing: the build falls back to
		// the default URL and reports it.
		if err := routing.SyncContent(ctx, q, a.ContentID); err != nil {
			if !errors.Is(err, routing.ErrURLTaken) && !errors.Is(err, routing.ErrReservedPath) {
				return fmt.Errorf("sync route: %w", err)
			}
			slog.WarnContext(ctx, "scheduled content published without its URL", "tenant_id", a.TenantID, "content_id", a.ContentID, "err", err)
		}
		client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
		if err != nil {
			return err
		}
		_, err = client.InsertTx(ctx, tx, BuildSiteArgs{TenantID: a.TenantID}, nil)
		return err
	})
}

// ScheduleExisting inserts publish jobs for every scheduled content of the
// tenant, e.g. after an import. Duplicates of existing jobs are dropped.
func ScheduleExisting(ctx context.Context, d *db.DB, client *river.Client[pgx.Tx], tenantID uuid.UUID) (int, error) {
	var rows []dbgen.ListScheduledRow
	err := d.InTenant(db.WithTenant(ctx, tenantID), func(q *dbgen.Queries) error {
		var err error
		rows, err = q.ListScheduled(ctx)
		return err
	})
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		if r.ScheduledRevisionID == nil || r.ScheduledAt == nil {
			continue
		}
		if _, err := client.Insert(ctx, PublishScheduledArgs{
			TenantID: tenantID, ContentID: r.ID, RevisionID: *r.ScheduledRevisionID, At: *r.ScheduledAt,
		}, nil); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}
