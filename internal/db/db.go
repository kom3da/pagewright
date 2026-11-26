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

// DB wraps a pool whose connections log in as the pagewright_app role.
type DB struct {
	pool *pgxpool.Pool
}

// New returns a DB using pool.
func New(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// InTenant runs fn in a transaction scoped to the tenant in ctx. The
// setting is transaction-local, so it never leaks to the next user of the
// pooled connection. fn's error rolls the transaction back.
func (d *DB) InTenant(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	tenantID, ok := TenantFrom(ctx)
	if !ok {
		return ErrNoTenant
	}
	return pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
			return fmt.Errorf("set tenant: %w", err)
		}
		return fn(dbgen.New(tx))
	})
}

// FindUserByEmail looks a user up across tenants. It is meant for login,
// before a tenant is chosen, and returns pgx.ErrNoRows when not found.
func (d *DB) FindUserByEmail(ctx context.Context, email string) (dbgen.FindUserByEmailRow, error) {
	return dbgen.New(d.pool).FindUserByEmail(ctx, email)
}
