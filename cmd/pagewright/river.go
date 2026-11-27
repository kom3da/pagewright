package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/jobs"
	"github.com/kom3da/pagewright/internal/mail"
)

// newRiver returns a River client that works the build jobs. It uses the
// app's pool, so jobs run as pagewright_app like the API.
func newRiver(d *db.DB, cfg config.Config, sender mail.Sender) (*river.Client[pgx.Tx], error) {
	workers := jobs.NewWorkers(d, build.Options{
		OutDir: cfg.SiteDir(), MediaRoot: cfg.MediaDir(), BaseURL: cfg.PublicBaseURL, Logger: slog.Default(),
		Pagefind: cfg.Pagefind, FormBaseURL: cfg.BaseURL,
	}, sender)
	if cfg.Pagefind == "" {
		slog.Info("Pagefind が見つからないため、公開サイトの検索は作りません（mise install で入ります）")
	}
	c, err := river.NewClient(riverpgxv5.New(d.Pool()), &river.Config{
		Schema:       riverSchema,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}},
		Workers:      workers,
		PeriodicJobs: jobs.PeriodicJobs(),
		// Mail jobs carry addresses and login links: keep finished jobs
		// briefly (River's defaults are a day, and a week for failed ones).
		CompletedJobRetentionPeriod: time.Hour,
		CancelledJobRetentionPeriod: 24 * time.Hour,
		DiscardedJobRetentionPeriod: 72 * time.Hour,
		Logger:                      slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}
	return c, nil
}

// riverJobs implements api.Jobs with River.
type riverJobs struct {
	client *river.Client[pgx.Tx]
}

func (j riverJobs) EnqueueBuild(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) error {
	_, err := j.client.InsertTx(ctx, tx, jobs.BuildSiteArgs{TenantID: tenantID}, nil)
	return err
}

func (j riverJobs) EnqueueMail(ctx context.Context, tx pgx.Tx, m mail.Message) error {
	args := jobs.SendMailArgs{To: m.To, Subject: m.Subject, Body: m.Body}
	opts := &river.InsertOpts{MaxAttempts: 10}
	var err error
	if tx == nil {
		_, err = j.client.Insert(ctx, args, opts)
	} else {
		_, err = j.client.InsertTx(ctx, tx, args, opts)
	}
	return err
}

func (j riverJobs) EnqueueContactNotify(ctx context.Context, tx pgx.Tx, args jobs.ContactNotifyArgs) error {
	_, err := j.client.InsertTx(ctx, tx, args, &river.InsertOpts{MaxAttempts: 10})
	return err
}

func (j riverJobs) EnqueuePublish(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, contentID, revisionID int64, at time.Time) error {
	_, err := j.client.InsertTx(ctx, tx, jobs.PublishScheduledArgs{
		TenantID: tenantID, ContentID: contentID, RevisionID: revisionID, At: at,
	}, nil)
	return err
}

func (j riverJobs) EnqueuePruneRevisions(ctx context.Context, tenantID uuid.UUID) error {
	_, err := j.client.Insert(ctx, jobs.NextPruneRevisions(tenantID, time.Now()), nil)
	return err
}
