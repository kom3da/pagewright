package build

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db"
)

// buildLockNamespace is the first key of the two-key advisory lock a
// build holds ("pwbl"). Other users of advisory locks must pick another
// namespace; the one-key (bigint) form never collides with the two-key
// form.
const buildLockNamespace int32 = 0x7077626c

var tenantLocks sync.Map // uuid.UUID → *sync.Mutex

// lockTenant serializes builds of one tenant across processes: an
// in-process mutex first (so one process keeps at most one connection
// waiting per tenant), then a session-level PostgreSQL advisory lock held
// on a connection taken from the pool for the whole build. Two processes
// would otherwise remove each other's staging directory. The returned
// function releases both; it must be called exactly once.
func lockTenant(ctx context.Context, d *db.DB, id uuid.UUID) (func(), error) {
	m, _ := tenantLocks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()

	conn, err := d.Pool().Acquire(ctx)
	if err != nil {
		mu.Unlock()
		return nil, fmt.Errorf("lock tenant: %w", err)
	}
	key := buildLockKey(id)
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1, $2)", buildLockNamespace, key); err != nil {
		// A cancelled wait may leave the connection in doubt: drop it.
		_ = conn.Conn().Close(context.WithoutCancel(ctx))
		conn.Release()
		mu.Unlock()
		return nil, fmt.Errorf("lock tenant: %w", err)
	}
	return func() {
		defer mu.Unlock()
		uctx := context.WithoutCancel(ctx)
		var ok bool
		if err := conn.QueryRow(uctx, "SELECT pg_advisory_unlock($1, $2)", buildLockNamespace, key).Scan(&ok); err != nil || !ok {
			// Closing the session releases its locks.
			_ = conn.Conn().Close(uctx)
		}
		conn.Release()
	}, nil
}

// buildLockKey folds a tenant id into the lock's second key. Two tenants
// sharing a key only build one after the other.
func buildLockKey(id uuid.UUID) int32 {
	h := fnv.New32a()
	_, _ = h.Write(id[:])
	return int32(h.Sum32()) //nolint:gosec // wrapping is fine for a hash
}
