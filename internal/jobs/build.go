// Package jobs holds the River job workers.
//
// River's tables have no tenant_id (the documented exception): each job
// carries its tenant in its args, and the worker scopes every database
// access to that tenant (db.WithTenant / InTenant).
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/mail"
)

// BuildSiteArgs asks for a full build of one tenant's public site. Insert
// one whenever content is published, unpublished or deleted.
type BuildSiteArgs struct {
	TenantID uuid.UUID `json:"tenant_id"`
}

// Kind implements river.JobArgs.
func (BuildSiteArgs) Kind() string { return "build_site" }

// BuildTimeout bounds one build. A full build of a large site (10,000
// pages) must fit comfortably.
const BuildTimeout = 10 * time.Minute

// NewWorkers registers every worker of this package.
func NewWorkers(d *db.DB, opts build.Options, sender mail.Sender) *river.Workers {
	w := river.NewWorkers()
	river.AddWorker(w, NewBuildSiteWorker(d, opts))
	river.AddWorker(w, NewPublishScheduledWorker(d))
	river.AddWorker(w, &CleanupAuthWorker{db: d})
	river.AddWorker(w, &SendMailWorker{sender: sender})
	return w
}

// BuildSiteWorker runs build.Build.
//
// Bursts are coalesced here rather than with River's unique jobs: River
// requires a unique job to also be unique while running, so a publish
// during a running build would be dropped and never built. Instead, a job
// that sees another build of the same tenant still waiting (available)
// skips itself: the waiting one starts later and so sees at least as
// much. Builds of one tenant never overlap (build.Build locks per tenant).
type BuildSiteWorker struct {
	river.WorkerDefaults[BuildSiteArgs]
	db   *db.DB
	opts build.Options
	// build and laterBuildWaiting are replaced in tests.
	build             func(ctx context.Context, d *db.DB, tenantID uuid.UUID, opts build.Options) (build.Result, error)
	laterBuildWaiting func(ctx context.Context, job *river.Job[BuildSiteArgs]) (bool, error)
}

// NewBuildSiteWorker returns the worker for BuildSiteArgs.
func NewBuildSiteWorker(d *db.DB, opts build.Options) *BuildSiteWorker {
	return &BuildSiteWorker{db: d, opts: opts, build: build.Build, laterBuildWaiting: laterBuildWaiting}
}

// Timeout implements river.Worker.
func (w *BuildSiteWorker) Timeout(*river.Job[BuildSiteArgs]) time.Duration { return BuildTimeout }

// Work implements river.Worker.
func (w *BuildSiteWorker) Work(ctx context.Context, job *river.Job[BuildSiteArgs]) error {
	tenantID := job.Args.TenantID
	if tenantID == uuid.Nil {
		return river.JobCancel(errors.New("build_site: no tenant_id"))
	}
	log := slog.With("job_id", job.ID, "tenant_id", tenantID)
	waiting, err := w.laterBuildWaiting(ctx, job)
	if err != nil {
		log.Warn("build_site: check for waiting builds", "err", err) // build anyway
	}
	if waiting {
		log.Info("build_site: skipped, a later build is waiting")
		return nil
	}
	started := time.Now()
	res, err := w.build(ctx, w.db, tenantID, w.opts)
	w.record(ctx, tenantID, started, res, err)
	if err != nil {
		return fmt.Errorf("build_site: %w", err)
	}
	log.Info("build_site: done", "changed", len(res.Changed), "removed", len(res.Removed))
	return nil
}

// laterBuildWaiting reports whether another build of the job's tenant is
// available to run. It needs the River client in ctx (always there when
// River runs the job).
func laterBuildWaiting(ctx context.Context, job *river.Job[BuildSiteArgs]) (bool, error) {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return false, nil //nolint:nilerr // not run by River: nothing to coalesce with
	}
	params := river.NewJobListParams().
		Kinds(BuildSiteArgs{}.Kind()).
		States(rivertype.JobStateAvailable).
		Where("args->>'tenant_id' = @tenant_id AND id <> @job_id",
			river.NamedArgs{"tenant_id": job.Args.TenantID.String(), "job_id": job.ID}).
		First(1)
	res, err := client.JobList(ctx, params)
	if err != nil {
		return false, err
	}
	return len(res.Jobs) > 0, nil
}

// record keeps the outcome of a build for the admin. Failing to record
// does not fail the build.
func (w *BuildSiteWorker) record(ctx context.Context, tenantID uuid.UUID, started time.Time, res build.Result, buildErr error) {
	warnings, _ := json.Marshal(nonNilStrings(res.Warnings))
	var msg *string
	if buildErr != nil {
		m := buildErr.Error()
		msg = &m
	}
	err := w.db.InTenant(db.WithTenant(ctx, tenantID), func(q *dbgen.Queries) error {
		if err := q.RecordBuild(ctx, dbgen.RecordBuildParams{
			StartedAt: started, Files: int64(res.Files), Changed: int64(len(res.Changed)),
			Removed: int64(len(res.Removed)), Warnings: warnings, Error: msg,
		}); err != nil {
			return err
		}
		return q.PruneBuilds(ctx)
	})
	if err != nil {
		slog.WarnContext(ctx, "build_site: record result", "tenant_id", tenantID, "err", err)
	}
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
