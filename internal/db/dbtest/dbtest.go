// Package dbtest creates migrated databases on a real PostgreSQL for tests.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/migrations"
)

// DB is a fresh database, dropped when the test ends.
type DB struct {
	Admin *pgxpool.Pool // superuser: bypasses RLS; for seeding and checks only
	App   *db.DB        // runs as pagewright_app, like the server
	URL   string        // the database, as the admin user
}

// New creates and migrates a database on PAGEWRIGHT_TEST_DATABASE_URL
// (default: the dev database server).
func New(t testing.TB) *DB {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("PAGEWRIGHT_TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = config.DevDatabaseURL
	}
	server, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL (start it with `mise run dev`): %v", err)
	}
	t.Cleanup(func() { _ = server.Close(ctx) })

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "pw_test_" + hex.EncodeToString(b)
	if _, err := server.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := server.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dbURL := u.String()

	cfg, err := pgx.ParseConfig(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDB(*cfg)
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = sqlDB.Close()

	admin, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	app, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return &DB{Admin: admin, App: app, URL: dbURL}
}
