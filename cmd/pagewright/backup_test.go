package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kom3da/pagewright/internal/backup"
	"github.com/kom3da/pagewright/internal/config"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
	"github.com/kom3da/pagewright/internal/db/migrations"
)

// seedSite fills one tenant with a bit of everything: a member, media
// (also as the logo), a published content with a scheduled revision, a
// category, a route, a menu entry, an inquiry, and a session and login
// token (which a backup leaves out).
func seedSite(t *testing.T, admin *pgxpool.Pool, mediaDir string, tenant uuid.UUID, email string) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
		var user uuid.UUID
		var media, content, rev1, rev2, cat int64
		steps := []func() error{
			func() error {
				return tx.QueryRow(ctx, "INSERT INTO users (email, display_name) VALUES ($1, '担当者') RETURNING id", email).Scan(&user)
			},
			func() error {
				_, err := tx.Exec(ctx, "INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, 'admin')", tenant, user)
				return err
			},
			func() error {
				return tx.QueryRow(ctx, `INSERT INTO media (tenant_id, storage_key, filename, content_type, size_bytes, alt_text, uploaded_by)
					VALUES ($1, '2026/11/logo.png', 'ロゴ.png', 'image/png', 4, '会社のロゴ', $2) RETURNING id`, tenant, user).Scan(&media)
			},
			func() error {
				_, err := tx.Exec(ctx, `UPDATE tenants SET settings = '{"accent": "#336699"}', logo_media_id = $2,
					public_base_url = 'https://' || left(id::text, 8) || '.example' WHERE id = $1`, tenant, media)
				return err
			},
			func() error {
				return tx.QueryRow(ctx, "INSERT INTO contents (tenant_id, type, slug, author_id) VALUES ($1, 'post', 'hello', $2) RETURNING id", tenant, user).Scan(&content)
			},
			func() error {
				return tx.QueryRow(ctx, `INSERT INTO content_revisions (tenant_id, content_id, title, body, body_schema_version, created_by)
					VALUES ($1, $2, 'こんにちは', '{"type": "doc", "content": []}', 1, $3) RETURNING id`, tenant, content, user).Scan(&rev1)
			},
			func() error {
				return tx.QueryRow(ctx, `INSERT INTO content_revisions (tenant_id, content_id, title, body, body_schema_version, created_by)
					VALUES ($1, $2, 'こんにちは（改）', '{"type": "doc", "content": []}', 1, $3) RETURNING id`, tenant, content, user).Scan(&rev2)
			},
			func() error {
				_, err := tx.Exec(ctx, `UPDATE contents SET published_revision_id = $3, published_at = now(),
					scheduled_revision_id = $4, scheduled_at = now() + interval '1 day' WHERE tenant_id = $1 AND id = $2`,
					tenant, content, rev1, rev2)
				return err
			},
			func() error {
				return tx.QueryRow(ctx, "INSERT INTO categories (tenant_id, name, slug) VALUES ($1, 'お知らせ', 'news') RETURNING id", tenant).Scan(&cat)
			},
			func() error {
				_, err := tx.Exec(ctx, "INSERT INTO content_categories (tenant_id, content_id, category_id) VALUES ($1, $2, $3)", tenant, content, cat)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, "INSERT INTO routes (tenant_id, path, kind, content_id) VALUES ($1, '/posts/hello/', 'canonical', $2)", tenant, content)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, "INSERT INTO menu_items (tenant_id, position, content_id) VALUES ($1, 1, $2)", tenant, content)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO form_submissions (tenant_id, name, email, message, page_path)
					VALUES ($1, '山田 太郎', 'taro@example.jp', '見積もりをお願いします', '/contact/')`, tenant)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, tenant_id, expires_at)
					VALUES (sha256(gen_random_uuid()::text::bytea), $1, $2, now() + interval '1 day')`, user, tenant)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO login_tokens (token_hash, user_id, expires_at)
					VALUES (sha256(gen_random_uuid()::text::bytea), $1, now() + interval '1 hour')`, user)
				return err
			},
		}
		for _, s := range steps {
			if err := s(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed %s: %v", tenant, err)
	}
	p := filepath.Join(mediaDir, tenant.String(), "2026", "11", "logo.png")
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("png"+tenant.String()[:1]), 0o640); err != nil {
		t.Fatal(err)
	}
}

// tableRows returns every row of every table in public as JSON.
func tableRows(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT relname FROM pg_class
		WHERE relnamespace = 'public'::regnamespace AND relkind = 'r' AND relname <> 'goose_db_version'`)
	if err != nil {
		t.Fatal(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, n := range names {
		var s string
		q := fmt.Sprintf("SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text), '[]')::text FROM %s t", pgx.Identifier{n}.Sanitize())
		if err := pool.QueryRow(ctx, q).Scan(&s); err != nil {
			t.Fatal(err)
		}
		out[n] = s
	}
	return out
}

// schemaOf describes grants, row level security, constraints, triggers and
// policies of public and river, to compare a restored database with one
// migrated as usual.
func schemaOf(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT format('rel %s.%s %s acl=%s rls=%s', n.nspname, c.relname, c.relkind, c.relacl, c.relrowsecurity)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname IN ('public', 'river')
		UNION ALL
		SELECT format('con %s %s %s valid=%s', conrelid::regclass, conname, pg_get_constraintdef(oid), convalidated)
		FROM pg_constraint WHERE connamespace = 'public'::regnamespace
		UNION ALL
		SELECT format('trg %s %s %s', tgrelid::regclass, tgname, tgenabled)
		FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
		WHERE NOT tgisinternal AND c.relnamespace = 'public'::regnamespace
		UNION ALL
		SELECT format('pol %s %s %s', tablename, policyname, qual) FROM pg_policies WHERE schemaname = 'public'
		UNION ALL
		SELECT format('fn %s acl=%s', p.oid::regprocedure, p.proacl) FROM pg_proc p WHERE p.pronamespace = 'public'::regnamespace`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func readFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = fmt.Sprintf("%s %v", b, info.Mode().Perm())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func connect(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func onlyArchive(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		found = append(found, e.Name())
	}
	if len(found) != 1 || !archiveRe.MatchString(found[0]) {
		t.Fatalf("backup dir has %v, want one archive", found)
	}
	return filepath.Join(dir, found[0])
}

func TestBackupRestore(t *testing.T) {
	ctx := context.Background()
	src := dbtest.New(t)
	srcCfg := config.Config{MigrateDatabaseURL: src.URL, DataDir: t.TempDir()}
	if err := runMigrate(ctx, srcCfg); err != nil {
		t.Fatal(err)
	}
	a, b := src.CreateTenant(t, "サイトA"), src.CreateTenant(t, "サイトB")
	dbtest.SkipIDs(t, src.Admin, b, 1000)
	seedSite(t, src.Admin, srcCfg.MediaDir(), a, "a@example.jp")
	seedSite(t, src.Admin, srcCfg.MediaDir(), b, "b@example.jp")

	out := t.TempDir()
	if err := runBackup(ctx, srcCfg, []string{"--out", out}); err != nil {
		t.Fatal(err)
	}
	archive := onlyArchive(t, out)
	if st, err := os.Stat(archive); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("archive mode = %v (%v), want 0600", st.Mode().Perm(), err)
	}

	dstURL := dbtest.NewEmpty(t).URL
	dstCfg := config.Config{MigrateDatabaseURL: dstURL, DataDir: t.TempDir()}
	if err := runRestore(ctx, dstCfg, []string{archive}); err != nil {
		t.Fatal(err)
	}
	dst := connect(t, dstURL)

	want := tableRows(t, src.Admin)
	want["sessions"], want["login_tokens"] = "[]", "[]"
	if diff := cmp.Diff(want, tableRows(t, dst)); diff != "" {
		t.Errorf("rows differ (-backed up +restored):\n%s", diff)
	}
	if diff := cmp.Diff(schemaOf(t, src.Admin), schemaOf(t, dst)); diff != "" {
		t.Errorf("schema differs (-migrated +restored):\n%s", diff)
	}
	if diff := cmp.Diff(readFiles(t, srcCfg.MediaDir()), readFiles(t, dstCfg.MediaDir())); diff != "" {
		t.Errorf("media differ (-backed up +restored):\n%s", diff)
	}

	// The jobs the archive does not carry are queued again.
	var jobs []string
	rows, err := dst.Query(ctx, "SELECT kind || ' ' || (args->>'tenant_id') FROM river.river_job ORDER BY 1")
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		t.Fatal(err)
	}
	var wantJobs []string
	for _, id := range []uuid.UUID{a, b} {
		wantJobs = append(wantJobs, "build_site "+id.String(), "prune_revisions "+id.String(), "publish_scheduled "+id.String())
	}
	slices.Sort(wantJobs)
	if diff := cmp.Diff(wantJobs, jobs); diff != "" {
		t.Errorf("jobs (-want +got):\n%s", diff)
	}

	// The app works on the restored database and sees only its own site;
	// a new row continues the site's own ids.
	app, err := db.Open(ctx, dstURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	for _, tc := range []struct {
		tenant uuid.UUID
		wantID int64
	}{{a, 2}, {b, 1002}} {
		err := app.InTenantTx(db.WithTenant(ctx, tc.tenant), func(tx pgx.Tx, _ *dbgen.Queries) error {
			var sites, inquiries int
			if err := tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM tenants), (SELECT count(*) FROM form_submissions)").Scan(&sites, &inquiries); err != nil {
				return err
			}
			if sites != 1 || inquiries != 1 {
				t.Errorf("tenant %s sees %d sites and %d inquiries, want 1 and 1", tc.tenant, sites, inquiries)
			}
			var id, number int64
			if err := tx.QueryRow(ctx, "INSERT INTO contents (type, slug) VALUES ('page', 'about') RETURNING id, number").Scan(&id, &number); err != nil {
				return err
			}
			if id != tc.wantID || number != 2 {
				t.Errorf("tenant %s: new content got id %d number %d, want %d and 2", tc.tenant, id, number, tc.wantID)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A second restore needs --force, then replaces the data and media.
	stray := filepath.Join(dstCfg.MediaDir(), a.String(), "stray.png")
	if err := os.WriteFile(stray, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := runRestore(ctx, dstCfg, []string{archive}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("restore over data: err = %v, want a hint at --force", err)
	}
	before := t.TempDir()
	if err := runRestore(ctx, dstCfg, []string{"--force", "--out", before, archive}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, tableRows(t, dst)); diff != "" {
		t.Errorf("rows after --force differ (-backed up +restored):\n%s", diff)
	}
	if diff := cmp.Diff(readFiles(t, srcCfg.MediaDir()), readFiles(t, dstCfg.MediaDir())); diff != "" {
		t.Errorf("media after --force differ (-backed up +restored):\n%s", diff)
	}
	// --force first backed up what it replaced, stray file included.
	m, err := backup.Verify(onlyArchive(t, before))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(m.Files, func(f backup.File) bool { return strings.HasSuffix(f.Path, "/stray.png") }) {
		t.Errorf("backup before --force lacks the stray file: %+v", m.Files)
	}
}

// forgeArchive rewrites src with its manifest and entries changed by edit,
// with the manifest's file list recomputed to match (as a crafted archive
// would have).
func forgeArchive(t *testing.T, src string, edit func(m *backup.Manifest, entries map[string][]byte)) string {
	t.Helper()
	m, err := backup.Verify(src)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	rewriteArchive(t, src, func(name string, b []byte) []byte {
		if name != backup.ManifestName {
			entries[name] = b
		}
		return b
	})
	edit(&m, entries)
	m.Files = nil
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		sum := sha256.Sum256(entries[name])
		m.Files = append(m.Files, backup.File{Path: name, Size: int64(len(entries[name])), SHA256: hex.EncodeToString(sum[:])})
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	write := func(name string, b []byte) {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Size: int64(len(b)), Mode: 0o600}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range m.Files {
		write(f.Path, entries[f.Path])
	}
	mb, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	write(backup.ManifestName, mb)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "forged.tar.gz")
	if err := os.WriteFile(dst, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// Restore refuses crafted archives, and --force checks what it can
// before it drops anything: a refused restore leaves the data as it was,
// and takes no backup.
func TestRestoreForceChecksFirst(t *testing.T) {
	ctx := context.Background()
	src := dbtest.New(t)
	cfg := config.Config{MigrateDatabaseURL: src.URL, DataDir: t.TempDir()}
	if err := runMigrate(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	seedSite(t, src.Admin, cfg.MediaDir(), src.CreateTenant(t, "サイト"), "a@example.jp")
	out := t.TempDir()
	if err := runBackup(ctx, cfg, []string{"--out", out}); err != nil {
		t.Fatal(err)
	}
	archive := onlyArchive(t, out)
	addTable := func(name string) func(*backup.Manifest, map[string][]byte) {
		return func(m *backup.Manifest, entries map[string][]byte) {
			m.Tables = append(m.Tables, backup.Table{Name: name, Columns: []string{"tenant_id"}})
			entries["db/"+name+".copy"] = nil
		}
	}
	tests := []struct {
		name string
		edit func(*backup.Manifest, map[string][]byte)
		want string
	}{
		{"sessions", addTable("sessions"), "must not be in a backup"},
		{"login tokens", addTable("login_tokens"), "must not be in a backup"},
		{"goose version", addTable("goose_db_version"), "must not be in a backup"},
		{"stray table entry", func(_ *backup.Manifest, entries map[string][]byte) { entries["db/extra.copy"] = []byte("x") }, "unexpected entry"},
		{"missing column", func(m *backup.Manifest, _ map[string][]byte) {
			m.Tables[0].Columns = append(m.Tables[0].Columns, "no_such_column")
		}, "no_such_column"},
	}
	rows := tableRows(t, src.Admin)
	media := readFiles(t, cfg.MediaDir())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := t.TempDir()
			err := runRestore(ctx, cfg, []string{"--force", "--out", before, forgeArchive(t, archive, tt.edit)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
			if diff := cmp.Diff(rows, tableRows(t, src.Admin)); diff != "" {
				t.Errorf("rows changed by a refused restore (-before +after):\n%s", diff)
			}
			if diff := cmp.Diff(media, readFiles(t, cfg.MediaDir())); diff != "" {
				t.Errorf("media changed by a refused restore (-before +after):\n%s", diff)
			}
			if entries, _ := os.ReadDir(before); len(entries) != 0 {
				t.Errorf("a refused restore took a backup: %v", entries)
			}
		})
	}
}

// rewriteArchive copies src to a new archive, passing each entry through fn.
func rewriteArchive(t *testing.T, src string, fn func(name string, b []byte) []byte) string {
	t.Helper()
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		b = fn(hdr.Name, b)
		hdr.Size = int64(len(b))
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.WriteFile(dst, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestRestoreRefuses(t *testing.T) {
	ctx := context.Background()
	src := dbtest.New(t)
	srcCfg := config.Config{MigrateDatabaseURL: src.URL, DataDir: t.TempDir()}
	seedSite(t, src.Admin, srcCfg.MediaDir(), src.CreateTenant(t, "サイト"), "a@example.jp")
	out := t.TempDir()
	if err := runBackup(ctx, srcCfg, []string{"--out", out}); err != nil {
		t.Fatal(err)
	}
	archive := onlyArchive(t, out)

	damaged := rewriteArchive(t, archive, func(name string, b []byte) []byte {
		if strings.HasPrefix(name, "media/") {
			b[0] ^= 1
		}
		return b
	})
	newer := rewriteArchive(t, archive, func(name string, b []byte) []byte {
		if name != backup.ManifestName {
			return b
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		m["migration_version"] = 99999
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	})
	notArchive := filepath.Join(t.TempDir(), "x.tar.gz")
	if err := os.WriteFile(notArchive, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, url, archive, want string
	}{
		{"damaged", "", damaged, "checksum"},
		{"newer", "", newer, "新しい版"},
		{"not an archive", "", notArchive, "not a Pagewright backup"},
		{"over data", src.URL, archive, "--force"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.url
			if url == "" {
				url = dbtest.NewEmpty(t).URL
			}
			cfg := config.Config{MigrateDatabaseURL: url, DataDir: t.TempDir()}
			err := runRestore(ctx, cfg, []string{tt.archive})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}

	// Media left in the data dir also need --force.
	cfg := config.Config{MigrateDatabaseURL: dbtest.NewEmpty(t).URL, DataDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(cfg.MediaDir(), "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.MediaDir(), "x", "y.png"), nil, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := runRestore(ctx, cfg, []string{archive}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("restore over media: err = %v, want a hint at --force", err)
	}
}

func TestPruneBackups(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"pagewright-20261101T030000Z.tar.gz", "pagewright-20261102T030000Z.tar.gz",
		"pagewright-20261103T030000Z.tar.gz", "pagewright-20261104T030000Z.tar.gz",
		"notes.txt", "pagewright-latest.tar.gz",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneBackups(dir, 2); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{"notes.txt", "pagewright-20261103T030000Z.tar.gz", "pagewright-20261104T030000Z.tar.gz", "pagewright-latest.tar.gz"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("left (-want +got):\n%s", diff)
	}
}

func TestRemoveStaleTemp(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-13 * time.Hour)
	files := map[string]bool{ // name: old
		".pagewright-123.partial":            true,
		".pagewright-table-456":              true,
		".pagewright-789.partial":            false, // a backup still running
		"pagewright-20261101T030000Z.tar.gz": true,
		".pagewright-notes":                  true,
	}
	for name, isOld := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if isOld {
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	removeStaleTemp(dir, time.Now().Add(-staleTempAge))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{".pagewright-789.partial", ".pagewright-notes", "pagewright-20261101T030000Z.tar.gz"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("left (-want +got):\n%s", diff)
	}
}

// A backup from an older version is restored at its version, then
// migrated: here from before ids were numbered per tenant (00013).
func TestRestoreOlderBackup(t *testing.T) {
	ctx := context.Background()
	srcURL := dbtest.NewEmpty(t).URL
	src := connect(t, srcURL)
	sqlDB := stdlib.OpenDBFromPool(src)
	defer func() { _ = sqlDB.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(ctx, `
		WITH t AS (INSERT INTO tenants (name) VALUES ('古いサイト') RETURNING id),
		  u AS (INSERT INTO users (email) VALUES ('old@example.jp') RETURNING id),
		  m AS (INSERT INTO memberships (tenant_id, user_id, role) SELECT t.id, u.id, 'admin' FROM t, u RETURNING tenant_id),
		  c AS (INSERT INTO contents (tenant_id, type, slug, number) SELECT tenant_id, 'post', 'old', 1 FROM m RETURNING tenant_id, id)
		INSERT INTO content_revisions (tenant_id, content_id, body, body_schema_version) SELECT tenant_id, id, '{}', 1 FROM c`)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	srcCfg := config.Config{MigrateDatabaseURL: srcURL, DataDir: t.TempDir()}
	if err := runBackup(ctx, srcCfg, []string{"--out", out}); err != nil {
		t.Fatal(err)
	}
	dstURL := dbtest.NewEmpty(t).URL
	if err := runRestore(ctx, config.Config{MigrateDatabaseURL: dstURL, DataDir: t.TempDir()}, []string{onlyArchive(t, out)}); err != nil {
		t.Fatal(err)
	}
	if err := runMigrate(ctx, srcCfg); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(tableRows(t, src), tableRows(t, connect(t, dstURL))); diff != "" {
		t.Errorf("rows differ (-migrated in place +restored and migrated):\n%s", diff)
	}
}
