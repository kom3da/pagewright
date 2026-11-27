// Package migrations embeds the goose SQL migrations.
package migrations

import (
	"database/sql"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS

// NewProvider returns the goose provider for the migrations in fsys (FS,
// except in tests). `pagewright migrate` and the tests share it so both
// apply migrations the same way.
//
// It keeps goose's default of refusing out-of-order migrations: when the
// database already has a version above a pending one (a file with a lower
// number merged after a higher one was deployed), Up fails before changing
// anything instead of applying it in the wrong order. The fix is to
// renumber the file above the newest migration (see TestMigrationNumbers).
func NewProvider(db *sql.DB, fsys fs.FS) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithAllowOutofOrder(false))
}
