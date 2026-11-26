package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db/migrations"
)

// riverSchema holds River's tables (migrations/00003_river_schema.sql).
const riverSchema = "river"

// runMigrate applies goose migrations, then River's, as the table owner.
func runMigrate(ctx context.Context, cfg config.Config) error {
	pool, err := pgxpool.New(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return err
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("goose: %w", err)
	}
	for _, r := range results {
		slog.Info("マイグレーションを適用しました", "file", r.Source.Path, "duration", r.Duration)
	}

	m, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: riverSchema})
	if err != nil {
		return err
	}
	res, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("river: %w", err)
	}
	for _, v := range res.Versions {
		slog.Info("Riverのマイグレーションを適用しました", "version", v.Version)
	}
	return nil
}
