package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// RestoreOptions are the targets of a restore.
type RestoreOptions struct {
	// DatabaseURL connects as the role that runs the migrations; it
	// becomes the owner of the tables.
	DatabaseURL string
	MediaDir    string
	// Force overwrites a database with data and existing media. Built
	// sites are left alone: the rebuild after restore replaces them.
	Force bool
}

// ErrNotEmpty is returned when the target has data and Force is not set.
var ErrNotEmpty = errors.New("復元先にデータがあります")

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Verify reads the whole archive and checks it against its manifest.
func Verify(archive string) (Manifest, error) {
	var m Manifest
	seen := map[string]File{}
	var haveManifest bool
	err := walkArchive(archive, func(hdr *tar.Header, r io.Reader) error {
		if haveManifest {
			return fmt.Errorf("unexpected entry after the manifest: %q", hdr.Name)
		}
		if hdr.Name == ManifestName {
			haveManifest = true
			return json.NewDecoder(io.LimitReader(r, 256<<20)).Decode(&m)
		}
		if err := checkEntryName(hdr.Name); err != nil {
			return err
		}
		if _, dup := seen[hdr.Name]; dup {
			return fmt.Errorf("duplicate entry %q", hdr.Name)
		}
		h := sha256.New()
		n, err := io.Copy(h, r)
		if err != nil {
			return err
		}
		seen[hdr.Name] = File{Path: hdr.Name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
		return nil
	})
	if err != nil {
		return m, err
	}
	if !haveManifest {
		return m, errors.New("no manifest: not a Pagewright backup, or cut short")
	}
	if m.Format != Format {
		return m, fmt.Errorf("unsupported backup format %d", m.Format)
	}
	if len(m.Files) != len(seen) {
		return m, fmt.Errorf("the manifest lists %d files, the archive has %d", len(m.Files), len(seen))
	}
	for _, f := range m.Files {
		if seen[f.Path] != f {
			return m, fmt.Errorf("%s: checksum or size does not match the manifest (damaged archive)", f.Path)
		}
	}
	for _, t := range m.Tables {
		if !identRe.MatchString(t.Name) {
			return m, fmt.Errorf("bad table name %q", t.Name)
		}
		if _, ok := seen[tableEntry(t.Name)]; !ok {
			return m, fmt.Errorf("no rows for table %s", t.Name)
		}
		for _, c := range t.Columns {
			if !identRe.MatchString(c) {
				return m, fmt.Errorf("bad column name %q", c)
			}
		}
	}
	return m, nil
}

func checkEntryName(name string) error {
	ok := (strings.HasPrefix(name, "db/") || strings.HasPrefix(name, "media/")) &&
		path.Clean(name) == name && !strings.Contains(name, "\\") &&
		!strings.HasPrefix(name, "/") && !strings.Contains("/"+name+"/", "/../")
	if !ok {
		return fmt.Errorf("unexpected entry %q", name)
	}
	return nil
}

func walkArchive(archive string, fn func(*tar.Header, io.Reader) error) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("not a Pagewright backup: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("unexpected entry %q", hdr.Name)
		}
		if err := fn(hdr, tr); err != nil {
			return err
		}
	}
}

// Restore puts an archive into an empty database and media directory (or,
// with Force, replaces what is there). It leaves the database at the
// archive's migration version: the caller migrates up and queues the jobs
// (builds, scheduled publishing) afterwards.
func Restore(ctx context.Context, opts RestoreOptions, archive string) (Manifest, error) {
	m, err := Verify(archive)
	if err != nil {
		return m, err
	}
	conn, err := pgx.Connect(ctx, opts.DatabaseURL)
	if err != nil {
		return m, fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if err := checkMigrationVersion(ctx, conn, m.MigrationVersion); err != nil {
		return m, err
	}

	empty, err := databaseEmpty(ctx, conn)
	if err != nil {
		return m, err
	}
	if !empty && !opts.Force {
		return m, fmt.Errorf("データベース: %w", ErrNotEmpty)
	}
	if !opts.Force {
		if empty, err := dirEmpty(opts.MediaDir); err != nil {
			return m, err
		} else if !empty {
			return m, fmt.Errorf("%s: %w", opts.MediaDir, ErrNotEmpty)
		}
	}

	if err := resetDatabase(ctx, conn, m.MigrationVersion); err != nil {
		return m, err
	}
	if err := loadRows(ctx, conn, m, archive); err != nil {
		return m, err
	}
	if opts.Force {
		if err := os.RemoveAll(opts.MediaDir); err != nil {
			return m, err
		}
	}
	if err := extractMedia(archive, opts.MediaDir); err != nil {
		return m, err
	}
	return m, nil
}

func checkMigrationVersion(ctx context.Context, conn *pgx.Conn, v int64) error {
	sqlDB := stdlib.OpenDB(*conn.Config())
	defer func() { _ = sqlDB.Close() }()
	p, err := newGoose(sqlDB)
	if err != nil {
		return err
	}
	for _, s := range p.ListSources() {
		if s.Version == v {
			return nil
		}
	}
	return fmt.Errorf("このバックアップはより新しい版の Pagewright（データベースの版 %d）で取られました。同じか新しい版の Pagewright で復元してください", v)
}

// databaseEmpty reports whether the database has no tables, or has the
// schema but no site and no user (e.g. just migrated).
func databaseEmpty(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_class
		WHERE relnamespace = 'public'::regnamespace AND relkind IN ('r', 'p')`).Scan(&tables); err != nil {
		return false, err
	}
	if tables == 0 {
		return true, nil
	}
	var ours bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.tenants') IS NOT NULL AND to_regclass('public.users') IS NOT NULL`).Scan(&ours); err != nil {
		return false, err
	}
	if !ours {
		return false, nil
	}
	var used bool
	err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT FROM public.tenants) OR EXISTS (SELECT FROM public.users)`).Scan(&used)
	return !used, err
}

func dirEmpty(dir string) (bool, error) {
	empty := true
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if !d.IsDir() {
			empty = false
			return fs.SkipAll
		}
		return nil
	})
	return empty, err
}

// dropAll removes what the migrations created, so they can run again.
const dropAll = `
DO $$
DECLARE r record;
BEGIN
  DROP SCHEMA IF EXISTS river CASCADE;
  FOR r IN
    SELECT CASE c.relkind WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW'
             WHEN 'S' THEN 'SEQUENCE' WHEN 'f' THEN 'FOREIGN TABLE' ELSE 'TABLE' END AS kind,
           c.oid::regclass::text AS name
    FROM pg_class c
    WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
      AND NOT EXISTS (SELECT FROM pg_depend d
        WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype IN ('e', 'a', 'i'))
  LOOP
    EXECUTE format('DROP %s IF EXISTS %s CASCADE', r.kind, r.name);
  END LOOP;
  FOR r IN
    SELECT p.oid::regprocedure::text AS name FROM pg_proc p
    WHERE p.pronamespace = 'public'::regnamespace
      AND NOT EXISTS (SELECT FROM pg_depend d
        WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e')
  LOOP
    EXECUTE format('DROP ROUTINE IF EXISTS %s CASCADE', r.name);
  END LOOP;
  FOR r IN
    SELECT t.oid::regtype::text AS name FROM pg_type t
    WHERE t.typnamespace = 'public'::regnamespace AND t.typtype IN ('e', 'd', 'r')
      AND NOT EXISTS (SELECT FROM pg_depend d
        WHERE d.classid = 'pg_type'::regclass AND d.objid = t.oid AND d.deptype = 'e')
  LOOP
    EXECUTE format('DROP TYPE IF EXISTS %s CASCADE', r.name);
  END LOOP;
END
$$`

// resetDatabase empties the database and migrates it to version.
func resetDatabase(ctx context.Context, conn *pgx.Conn, version int64) error {
	if _, err := conn.Exec(ctx, dropAll); err != nil {
		return fmt.Errorf("empty the database: %w", err)
	}
	sqlDB := stdlib.OpenDB(*conn.Config())
	defer func() { _ = sqlDB.Close() }()
	p, err := newGoose(sqlDB)
	if err != nil {
		return err
	}
	if _, err := p.UpTo(ctx, version); err != nil {
		return fmt.Errorf("migrate to %d: %w", version, err)
	}
	return nil
}

// loadRows loads every table in one transaction. Foreign keys are dropped
// and added back (validated) around the load, since some reference each
// other (contents and content_revisions), and triggers are disabled, since
// the rows already carry the numbers they would assign.
func loadRows(ctx context.Context, conn *pgx.Conn, m Manifest, archive string) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		fks, err := collect(ctx, tx, `
			SELECT conrelid::regclass::text, quote_ident(conname), pg_get_constraintdef(oid)
			FROM pg_constraint WHERE contype = 'f' AND connamespace = 'public'::regnamespace`)
		if err != nil {
			return err
		}
		triggers, err := collect(ctx, tx, `
			SELECT t.tgrelid::regclass::text, quote_ident(t.tgname)
			FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
			WHERE NOT t.tgisinternal AND t.tgenabled <> 'D' AND c.relnamespace = 'public'::regnamespace`)
		if err != nil {
			return err
		}
		all, err := collect(ctx, tx, `
			SELECT oid::regclass::text FROM pg_class
			WHERE relnamespace = 'public'::regnamespace AND relkind = 'r' AND relname <> 'goose_db_version'`)
		if err != nil {
			return err
		}
		for _, fk := range fks {
			if _, err := tx.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", fk[0], fk[1])); err != nil {
				return err
			}
		}
		for _, tg := range triggers {
			if _, err := tx.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER %s", tg[0], tg[1])); err != nil {
				return err
			}
		}
		names := make([]string, len(all))
		for i, t := range all {
			names[i] = t[0]
		}
		if len(names) > 0 {
			if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")); err != nil {
				return err
			}
		}

		tables := map[string]Table{}
		for _, t := range m.Tables {
			if err := checkColumns(ctx, tx, t); err != nil {
				return err
			}
			tables[tableEntry(t.Name)] = t
		}
		err = walkArchive(archive, func(hdr *tar.Header, r io.Reader) error {
			t, ok := tables[hdr.Name]
			if !ok {
				return nil
			}
			if _, err := tx.Conn().PgConn().CopyFrom(ctx, r, copySQL(t, "FROM STDIN")); err != nil {
				return fmt.Errorf("load %s: %w", t.Name, err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		for _, fk := range fks {
			if _, err := tx.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s", fk[0], fk[1], fk[2])); err != nil {
				return fmt.Errorf("restore constraint %s on %s: %w", fk[1], fk[0], err)
			}
		}
		for _, tg := range triggers {
			if _, err := tx.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER %s", tg[0], tg[1])); err != nil {
				return err
			}
		}
		return resetSequences(ctx, tx)
	})
}

// checkColumns makes sure the table has every saved column.
func checkColumns(ctx context.Context, tx pgx.Tx, t Table) error {
	var exists bool
	var missing []string
	err := tx.QueryRow(ctx, `
		WITH t AS (SELECT to_regclass(format('public.%I', $1::text)) AS oid)
		SELECT (SELECT oid FROM t) IS NOT NULL, coalesce(array_agg(c), '{}')
		FROM unnest($2::text[]) c
		WHERE NOT EXISTS (SELECT FROM pg_attribute a, t
			WHERE a.attrelid = t.oid AND a.attname = c
			  AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = '')`,
		t.Name, t.Columns).Scan(&exists, &missing)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("no table %s at migration version of the backup", t.Name)
	}
	if len(missing) > 0 {
		return fmt.Errorf("table %s has no column %s at migration version of the backup", t.Name, strings.Join(missing, ", "))
	}
	return nil
}

// resetSequences moves each column-owned sequence (identity columns of old
// migration versions) past the loaded ids.
func resetSequences(ctx context.Context, tx pgx.Tx) error {
	seqs, err := collect(ctx, tx, `
		SELECT s.oid::regclass::text, t.oid::regclass::text, quote_ident(a.attname)
		FROM pg_class s
		JOIN pg_depend d ON d.classid = 'pg_class'::regclass AND d.objid = s.oid
		  AND d.refclassid = 'pg_class'::regclass AND d.deptype IN ('a', 'i')
		JOIN pg_class t ON t.oid = d.refobjid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
		WHERE s.relkind = 'S' AND s.relnamespace = 'public'::regnamespace`)
	if err != nil {
		return err
	}
	for _, s := range seqs {
		q := fmt.Sprintf("SELECT setval($1::regclass, coalesce(max(%s), 0) + 1, false) FROM %s", s[2], s[1])
		if _, err := tx.Exec(ctx, q, s[0]); err != nil {
			return err
		}
	}
	return nil
}

// collect returns the rows of a query of text columns.
func collect(ctx context.Context, tx pgx.Tx, query string) ([][]string, error) {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) ([]string, error) {
		vals := make([]string, len(r.FieldDescriptions()))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		return vals, r.Scan(ptrs...)
	})
}

// extractMedia writes the media entries under dir, with the modes the
// media store uses.
func extractMedia(archive, dir string) error {
	return walkArchive(archive, func(hdr *tar.Header, r io.Reader) error {
		rel, ok := strings.CutPrefix(hdr.Name, "media/")
		if !ok {
			return nil
		}
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Chtimes(p, hdr.ModTime, hdr.ModTime)
	})
}
