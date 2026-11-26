package db

import "github.com/jackc/pgx/v5/pgxpool"

// Pool exposes the pool to tests that need raw SQL as the app role.
func (d *DB) Pool() *pgxpool.Pool { return d.pool }
