// Package db gives tenant-scoped access to PostgreSQL.
//
// Every query on tenant data runs inside InTenant, which opens a
// transaction and sets app.tenant_id for that transaction only. Row level
// security in the database then hides other tenants' rows, so a missing
// WHERE clause cannot leak data.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

// ErrNoTenant is returned when the context carries no tenant.
var ErrNoTenant = errors.New("db: no tenant in context")

type tenantKey struct{}

// WithTenant returns a context that carries the tenant for the request.
func WithTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

// TenantFrom returns the tenant stored by WithTenant.
func TenantFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

// AppRole is the database role every connection switches to. It is not a
// superuser and has no BYPASSRLS, so row level security always applies.
const AppRole = "pagewright_app"

// DB is a connection pool whose connections all run as AppRole.
type DB struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL. Each connection runs SET ROLE to AppRole
// right after connecting, so RLS applies even when connString names a
// superuser; the login user only needs to be a member of AppRole.
func Open(ctx context.Context, connString string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse connection string: %w", err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if _, err := c.Exec(ctx, "SET ROLE "+AppRole); err != nil {
			return fmt.Errorf("set role %s: %w", AppRole, err)
		}
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close closes the pool.
func (d *DB) Close() {
	d.pool.Close()
}

// Pool returns the underlying pool, for River and tests. Queries on tenant
// data still belong in InTenant; without a tenant set, RLS hides all rows.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// Queries returns queries that run without a tenant, for the auth
// functions that are looked up before a site is chosen.
func (d *DB) Queries() *dbgen.Queries {
	return dbgen.New(d.pool)
}

// InTenant runs fn in a transaction scoped to the tenant in ctx. The
// setting is transaction-local, so it never leaks to the next user of the
// pooled connection. fn's error rolls the transaction back.
func (d *DB) InTenant(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	return d.InTenantTx(ctx, func(_ pgx.Tx, q *dbgen.Queries) error { return fn(q) })
}

// InTenantTx is InTenant that also hands fn the transaction, for work that
// must commit together with the queries (e.g. inserting a River job).
func (d *DB) InTenantTx(ctx context.Context, fn func(tx pgx.Tx, q *dbgen.Queries) error) error {
	tenantID, ok := TenantFrom(ctx)
	if !ok {
		return ErrNoTenant
	}
	return pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
			return fmt.Errorf("set tenant: %w", err)
		}
		return fn(tx, dbgen.New(tx))
	})
}

// FindUserByEmail looks a user up across tenants. It is meant for login,
// before a tenant is chosen, and returns pgx.ErrNoRows when not found.
func (d *DB) FindUserByEmail(ctx context.Context, email string) (dbgen.FindUserByEmailRow, error) {
	return dbgen.New(d.pool).FindUserByEmail(ctx, email)
}
