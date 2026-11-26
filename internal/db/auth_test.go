package db_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

func hash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func TestLoginFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()

	found, err := q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: "nobody@example.com", TokenHash: hash("x"), TtlSeconds: 900})
	if err != nil || found {
		t.Fatalf("unknown email: found=%v err=%v, want false", found, err)
	}
	found, err = q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: "Alice@Example.com", TokenHash: hash("link"), TtlSeconds: 900})
	if err != nil || !found {
		t.Fatalf("alice: found=%v err=%v, want true", found, err)
	}

	user, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("link"), SessionTokenHash: hash("session"), TtlSeconds: 3600})
	if err != nil || user != e.alice {
		t.Fatalf("login: user=%v err=%v, want alice", user, err)
	}
	// A login link works once.
	if user, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("link"), SessionTokenHash: hash("session2"), TtlSeconds: 3600}); err != nil || user != uuid.Nil {
		t.Errorf("reused link: user=%v err=%v, want none", user, err)
	}

	// alice belongs only to A, so A is selected.
	s, err := q.AuthSession(ctx, hash("session"))
	if err != nil {
		t.Fatal(err)
	}
	want := dbgen.AuthSessionRow{UserID: e.alice, Email: "alice@example.com", TenantID: e.tenantA, Role: "admin"}
	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("session (-want +got):\n%s", diff)
	}

	// She cannot switch to B, which she is not a member of.
	if ok, err := q.AuthSelectSite(ctx, dbgen.AuthSelectSiteParams{TokenHash: hash("session"), TenantID: e.tenantB}); err != nil || ok {
		t.Errorf("select B: ok=%v err=%v, want false", ok, err)
	}

	if err := q.AuthLogout(ctx, hash("session")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AuthSession(ctx, hash("session")); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("session after logout: err=%v, want ErrNoRows", err)
	}
}

func TestLoginWithSeveralSites(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()

	if _, err := q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: "carol@example.com", TokenHash: hash("link"), TtlSeconds: 900}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("link"), SessionTokenHash: hash("session"), TtlSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	s, err := q.AuthSession(ctx, hash("session"))
	if err != nil {
		t.Fatal(err)
	}
	if s.TenantID != uuid.Nil || s.Role != "" {
		t.Errorf("carol has two sites; tenant=%v role=%q, want none selected", s.TenantID, s.Role)
	}
	sites, err := q.AuthSessionSites(ctx, hash("session"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 {
		t.Errorf("carol's sites = %v, want A and B", sites)
	}
	// An unknown session sees no sites.
	if sites, err := q.AuthSessionSites(ctx, hash("other")); err != nil || len(sites) != 0 {
		t.Errorf("unknown session sites = %v err=%v, want none", sites, err)
	}
	if ok, err := q.AuthSelectSite(ctx, dbgen.AuthSelectSiteParams{TokenHash: hash("session"), TenantID: e.tenantB}); err != nil || !ok {
		t.Errorf("select B: ok=%v err=%v, want true", ok, err)
	}
}

func TestExpiredLoginLink(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()
	if _, err := q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: "alice@example.com", TokenHash: hash("link"), TtlSeconds: 900}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, "UPDATE login_tokens SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if user, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("link"), SessionTokenHash: hash("session"), TtlSeconds: 3600}); err != nil || user != uuid.Nil {
		t.Errorf("expired link: user=%v err=%v, want none", user, err)
	}
}

func TestAppCannotReadAuthTables(t *testing.T) {
	e := newTestEnv(t)
	for _, q := range []string{"SELECT count(*) FROM login_tokens", "SELECT count(*) FROM sessions"} {
		err := e.execInTenant(e.tenantA, q)
		if code := pgCode(err); code != "42501" {
			t.Errorf("%q: err = %v, want SQLSTATE 42501", q, err)
		}
	}
}

func TestAuthCleanup(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()
	for _, tok := range []string{"old-link", "new-link"} {
		if _, err := q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: "alice@example.com", TokenHash: hash(tok), TtlSeconds: 900}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("new-link"), SessionTokenHash: hash("live"), TtlSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: "alice@example.com", TokenHash: hash("link3"), TtlSeconds: 900}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("link3"), SessionTokenHash: hash("dead"), TtlSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"UPDATE login_tokens SET expires_at = now() - interval '2 days' WHERE token_hash = sha256('old-link')",
		"UPDATE sessions SET expires_at = now() - interval '1 second' WHERE token_hash = sha256('dead')",
	} {
		if _, err := e.admin.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	n, err := q.AuthCleanup(ctx)
	if err != nil || n != 2 {
		t.Fatalf("AuthCleanup = %d, %v; want 2 removed", n, err)
	}
	if _, err := q.AuthSession(ctx, hash("live")); err != nil {
		t.Errorf("live session removed: %v", err)
	}
}
