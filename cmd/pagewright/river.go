package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/jobs"
)

// newRiver returns a River client that works the build jobs. It uses the
// app's pool, so jobs run as pagewright_app like the API.
func newRiver(d *db.DB, cfg config.Config) (*river.Client[pgx.Tx], error) {
	workers := jobs.NewWorkers(d, build.Options{
		OutDir: cfg.SiteDir(), MediaRoot: cfg.MediaDir(), BaseURL: cfg.PublicBaseURL, Logger: slog.Default(),
	})
	c, err := river.NewClient(riverpgxv5.New(d.Pool()), &river.Config{
		Schema:  riverSchema,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}},
		Workers: workers,
		Logger:  slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}
	return c, nil
}

// riverBuilds implements api.BuildQueue with River.
type riverBuilds struct {
	client *river.Client[pgx.Tx]
}

func (b riverBuilds) EnqueueBuild(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) error {
	_, err := b.client.InsertTx(ctx, tx, jobs.BuildSiteArgs{TenantID: tenantID}, nil)
	return err
}
