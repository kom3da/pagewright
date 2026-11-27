package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kom3da/pagewright/internal/db/dbtest"
)

// TestBuildWaitsForOtherProcess holds the tenant's build lock from another
// session, as a second app process would, and checks that a build waits
// for it before touching the output directory.
func TestBuildWaitsForOtherProcess(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "ロック")
	// Cleanups run last-in first-out: on failure the waiting build is
	// cancelled and the lock's session closed before the pools close.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := t.TempDir()

	other := holdLock(t, env.Admin, buildLockKey(tenant))

	done := make(chan error, 1)
	go func() {
		_, err := Build(ctx, env.DB, tenant, Options{OutDir: out})
		done <- err
	}()

	// Wait until the build is queued on the lock.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		err := env.Admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND classid = $1::bigint::oid AND objid = $2::bigint::oid AND objsubid = 2)`,
			int64(uint32(buildLockNamespace)), int64(uint32(buildLockKey(tenant)))).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("build finished while another session held the lock (err %v)", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("build never waited for the lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(out, ".staging")); !os.IsNotExist(err) {
		t.Fatalf("build touched the output before getting the lock: %v", err)
	}

	if _, err := other.Exec(ctx, "SELECT pg_advisory_unlock($1, $2)", buildLockNamespace, buildLockKey(tenant)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("build did not finish after the lock was released")
	}
	if _, err := os.Stat(filepath.Join(out, tenant.String(), "index.html")); err != nil {
		t.Fatalf("no site written: %v", err)
	}

	// The build gave its lock back: another session can take it at once.
	var got bool
	if err := other.QueryRow(ctx, "SELECT pg_try_advisory_lock($1, $2)", buildLockNamespace, buildLockKey(tenant)).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("build kept the lock")
	}
	if _, err := other.Exec(ctx, "SELECT pg_advisory_unlock($1, $2)", buildLockNamespace, buildLockKey(tenant)); err != nil {
		t.Fatal(err)
	}
}

// TestBuildLockCancel checks that a build waiting for the lock gives up
// with its context.
func TestBuildLockCancel(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "ロック")
	ctx := context.Background()
	other := holdLock(t, env.Admin, buildLockKey(tenant))
	cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if _, err := Build(cctx, env.DB, tenant, Options{OutDir: t.TempDir()}); err == nil {
		t.Fatal("build ran while another session held the lock")
	}
	// The in-process lock was released too: a later build is not stuck.
	if _, err := other.Exec(ctx, "SELECT pg_advisory_unlock($1, $2)", buildLockNamespace, buildLockKey(tenant)); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(ctx, env.DB, tenant, Options{OutDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
}

// holdLock takes the build lock of key on a session of its own, as another
// app process would. The session is closed (releasing any lock it still
// holds) when the test ends, so a failing test never leaves a build waiting.
func holdLock(t *testing.T, pool *pgxpool.Pool, key int32) *pgxpool.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Conn().Close(ctx)
		conn.Release()
	})
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1, $2)", buildLockNamespace, key); err != nil {
		t.Fatal(err)
	}
	return conn
}
