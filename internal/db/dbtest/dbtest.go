// Package dbtest gives tests a fresh, migrated PostgreSQL database.
//
// It connects to PAGEWRIGHT_TEST_DATABASE_URL (default: the dev database
// from deploy/compose.dev.yaml) as a superuser, creates a throwaway
// database, runs the migrations and drops it when the test ends.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/migrations"
)

// DefaultAdminURL is the dev database from deploy/compose.dev.yaml.
const DefaultAdminURL = config.DevDatabaseURL

// Env is a migrated test database.
type Env struct {
	// Admin is a superuser pool. It bypasses RLS: use it only to seed what
	// the app cannot create (tenants, users, memberships) and for checks.
	Admin *pgxpool.Pool
	// DB is the application's access layer (runs as pagewright_app).
	DB *db.DB
	// URL is the test database as the admin user.
	URL string
}

// New creates the database. It fails the test when PostgreSQL is down.
func New(t testing.TB) *Env {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("PAGEWRIGHT_TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = DefaultAdminURL
	}
	server, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL (start it with `mise run dev`): %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(ctx); err != nil {
			t.Errorf("close server connection: %v", err)
		}
	})

	name := "pw_test_" + randomHex(t)
	if _, err := server.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := server.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	migrate(t, cfg.ConnConfig)

	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("PAGEWRIGHT_TEST_DATABASE_URL must be a URL: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()
	d, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return &Env{Admin: admin, DB: d, URL: dbURL}
}

// CreateTenant inserts a tenant (the app has no grant for that).
func (e *Env) CreateTenant(t testing.TB, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.Admin.Exec(context.Background(), "INSERT INTO tenants (id, name) VALUES ($1, $2)", id, name); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return id
}

func migrate(t testing.TB, cfg *pgx.ConnConfig) {
	t.Helper()
	sqlDB := stdlib.OpenDB(*cfg)
	defer func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close migration connection: %v", err)
		}
	}()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func randomHex(t testing.TB) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
