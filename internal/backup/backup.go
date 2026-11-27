// Package backup writes a whole Pagewright installation (every tenant's
// database rows and media files) to one archive, and restores it.
//
// The archive is a gzipped tar:
//
//	db/<table>.copy   rows of each table in public, in COPY text format
//	media/<tenant>/…  the media files
//	manifest.json     versions, tables and the SHA-256 of every file (last)
//
// Only rows are saved, not the schema: the schema is what the migrations
// make. Restore migrates an empty database up to the archive's migration
// version, loads the rows, then the caller migrates up to the latest. So
// no pg_dump (which must be at least the server's major version) is
// needed, and roles, grants and row level security come from the
// migrations exactly as on a fresh install.
//
// Not saved: built sites (rebuilt after restore), River's jobs (the
// restore queues what must run again), and the rows of login_tokens,
// sessions and invitations (secrets for logging in; everyone logs in
// again, and pending invitations are sent again).
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kom3da/pagewright/internal/db/migrations"
)

// Format is the archive layout version. Restore refuses other formats.
const Format = 1

// ManifestName is the manifest's entry, the last one of the archive.
const ManifestName = "manifest.json"

// ExcludedTables keep their schema but not their rows.
var ExcludedTables = []string{"login_tokens", "sessions", "invitations"}

// Manifest describes an archive.
type Manifest struct {
	Format            int       `json:"format"`
	PagewrightVersion string    `json:"pagewright_version"`
	MigrationVersion  int64     `json:"migration_version"`
	PostgresVersion   string    `json:"postgres_version"`
	CreatedAt         time.Time `json:"created_at"`
	Tables            []Table   `json:"tables"`
	Files             []File    `json:"files"`
}

// Table is one table's rows, stored at db/<Name>.copy.
type Table struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Rows    int64    `json:"rows"`
}

// File is an archive entry other than the manifest.
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func tableEntry(name string) string { return "db/" + name + ".copy" }

// Options are the sources of a backup.
type Options struct {
	// DatabaseURL connects as the owner of the tables (not subject to row
	// level security), i.e. PAGEWRIGHT_MIGRATE_DATABASE_URL.
	DatabaseURL string
	// MediaDir holds the media files (<data dir>/media).
	MediaDir string
	// TempDir stages each table before it goes into the archive.
	TempDir string
	// Version is the running Pagewright's version, for the manifest.
	Version string
}

// Write writes a backup archive to w. The service may keep running: the
// rows come from one snapshot, and the media are copied after it, so every
// file the rows refer to is there (unless deleted in between). Files newer
// than the snapshot are orphans and harmless.
func Write(ctx context.Context, opts Options, w io.Writer) (Manifest, error) {
	m := Manifest{Format: Format, PagewrightVersion: opts.Version, CreatedAt: time.Now().UTC()}
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed) // media are compressed already
	if err != nil {
		return m, err
	}
	tw := tar.NewWriter(gz)

	if err := writeDatabase(ctx, opts, &m, tw); err != nil {
		return m, err
	}
	if err := writeMedia(opts.MediaDir, &m, tw); err != nil {
		return m, err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	if err := writeEntry(tw, ManifestName, int64(len(b)), m.CreatedAt, strings.NewReader(string(b)), nil); err != nil {
		return m, err
	}
	if err := tw.Close(); err != nil {
		return m, err
	}
	return m, gz.Close()
}

func writeDatabase(ctx context.Context, opts Options, m *Manifest, tw *tar.Writer) error {
	conn, err := pgx.Connect(ctx, opts.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if m.MigrationVersion, err = migrationVersion(ctx, conn); err != nil {
		return err
	}

	// One snapshot for every table.
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Fail instead of silently saving one tenant's rows when connected as
	// a role subject to row level security (like pg_dump does).
	if _, err := tx.Exec(ctx, "SET LOCAL row_security = off"); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, "SHOW server_version").Scan(&m.PostgresVersion); err != nil {
		return err
	}
	tables, err := listTables(ctx, tx)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(opts.TempDir, ".pagewright-table-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	for _, t := range tables {
		if err := tmp.Truncate(0); err != nil {
			return err
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		tag, err := tx.Conn().PgConn().CopyTo(ctx, tmp, copySQL(t, "TO STDOUT"))
		if err != nil {
			if strings.Contains(err.Error(), "row-level security") {
				return errors.New("行ごとのアクセス制限を受けるユーザーで接続しています。PAGEWRIGHT_MIGRATE_DATABASE_URL にテーブルの所有者を指定してください")
			}
			return fmt.Errorf("copy %s: %w", t.Name, err)
		}
		t.Rows = tag.RowsAffected()
		size, err := tmp.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		f := File{Path: tableEntry(t.Name), Size: size}
		if err := writeEntry(tw, f.Path, size, m.CreatedAt, tmp, &f); err != nil {
			return err
		}
		m.Tables = append(m.Tables, t)
		m.Files = append(m.Files, f)
	}
	return tx.Commit(ctx)
}

// migrationVersion is the database's goose version.
func migrationVersion(ctx context.Context, conn *pgx.Conn) (int64, error) {
	sqlDB := stdlib.OpenDB(*conn.Config())
	defer func() { _ = sqlDB.Close() }()
	p, err := newGoose(sqlDB)
	if err != nil {
		return 0, err
	}
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("migration version: %w", err)
	}
	return v, nil
}

func newGoose(sqlDB *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
}

// listTables returns the tables of public to save, with their columns
// (generated columns cannot be loaded, so they are left out).
func listTables(ctx context.Context, q pgx.Tx) ([]Table, error) {
	rows, err := q.Query(ctx, `
		SELECT c.relname, array_agg(a.attname ORDER BY a.attnum)
		FROM pg_class c
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = ''
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind = 'r'
		  AND c.relname <> 'goose_db_version' AND c.relname <> ALL ($1)
		GROUP BY c.relname ORDER BY c.relname`, ExcludedTables)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Table, error) {
		var t Table
		err := r.Scan(&t.Name, &t.Columns)
		return t, err
	})
}

func copySQL(t Table, dir string) string {
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = pgx.Identifier{c}.Sanitize()
	}
	return fmt.Sprintf("COPY %s (%s) %s", pgx.Identifier{"public", t.Name}.Sanitize(), strings.Join(cols, ", "), dir)
}

func writeMedia(dir string, m *Manifest, tw *tar.Writer) error {
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll // no media yet
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil // directories are implied; nothing else is stored there
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil // deleted since the walk listed it
		}
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		e := File{Path: "media/" + filepath.ToSlash(rel), Size: st.Size()}
		if err := writeEntry(tw, e.Path, e.Size, st.ModTime(), f, &e); err != nil {
			return err
		}
		m.Files = append(m.Files, e)
		return nil
	})
	if err != nil {
		return fmt.Errorf("media: %w", err)
	}
	return nil
}

// writeEntry writes size bytes of r as a file entry. With f set, it
// records the entry's checksum there.
func writeEntry(tw *tar.Writer, name string, size int64, mod time.Time, r io.Reader, f *File) error {
	hdr := &tar.Header{Typeflag: tar.TypeReg, Name: name, Size: size, Mode: 0o600, ModTime: mod}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.CopyN(io.MultiWriter(tw, h), r, size); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if f != nil {
		f.SHA256 = hex.EncodeToString(h.Sum(nil))
	}
	return nil
}
