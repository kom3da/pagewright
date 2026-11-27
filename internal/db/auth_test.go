package db_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
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
	want := dbgen.AuthSessionRow{UserID: e.alice, Email: "alice@example.com", TenantID: e.tenantA, Role: "admin", TouchDue: true}
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

// login opens a session for email with the session token tok.
func login(t *testing.T, q *dbgen.Queries, email, tok string) {
	t.Helper()
	ctx := context.Background()
	if _, err := q.AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{Email: email, TokenHash: hash("link-" + tok), TtlSeconds: 900}); err != nil {
		t.Fatal(err)
	}
	if user, err := q.AuthLogin(ctx, dbgen.AuthLoginParams{LoginTokenHash: hash("link-" + tok), SessionTokenHash: hash(tok), TtlSeconds: 3600}); err != nil || user == uuid.Nil {
		t.Fatalf("login %s: user=%v err=%v", email, user, err)
	}
}

func TestSessionList(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()
	login(t, q, "alice@example.com", "alice-pc")
	login(t, q, "alice@example.com", "alice-phone")
	login(t, q, "bob@example.com", "bob-pc")

	// A use is recorded once, then not again within the hour.
	s, err := q.AuthSession(ctx, hash("alice-pc"))
	if err != nil || !s.TouchDue {
		t.Fatalf("new session: touch_due=%v err=%v, want true", s.TouchDue, err)
	}
	if err := q.AuthTouchSession(ctx, dbgen.AuthTouchSessionParams{TokenHash: hash("alice-pc"), Device: "Windows の Chrome"}); err != nil {
		t.Fatal(err)
	}
	if s, err := q.AuthSession(ctx, hash("alice-pc")); err != nil || s.TouchDue {
		t.Errorf("just used: touch_due=%v err=%v, want false", s.TouchDue, err)
	}
	if err := q.AuthTouchSession(ctx, dbgen.AuthTouchSessionParams{TokenHash: hash("alice-pc"), Device: "other"}); err != nil {
		t.Fatal(err)
	}

	sessions, err := q.AuthListSessions(ctx, hash("alice-pc"))
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID      []byte
		Device  string
		Current bool
	}
	var got []row
	for _, s := range sessions {
		got = append(got, row{s.ID, s.Device, s.IsCurrent})
	}
	want := []row{{hash(string(hash("alice-pc"))), "Windows の Chrome", true}, {hash(string(hash("alice-phone"))), "", false}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("alice's sessions (-want +got):\n%s", diff)
	}
	if sessions, err := q.AuthListSessions(ctx, hash("unknown")); err != nil || len(sessions) != 0 {
		t.Errorf("unknown session lists %d sessions (%v), want none", len(sessions), err)
	}

	// alice cannot end bob's session, nor her own through this.
	for _, id := range [][]byte{hash(string(hash("bob-pc"))), hash(string(hash("alice-pc"))), hash("bob-pc")} {
		if ok, err := q.AuthRevokeSession(ctx, dbgen.AuthRevokeSessionParams{TokenHash: hash("alice-pc"), SessionID: id}); err != nil || ok {
			t.Errorf("revoke %x: ok=%v err=%v, want false", id, ok, err)
		}
	}
	for _, tok := range []string{"alice-pc", "bob-pc"} {
		if _, err := q.AuthSession(ctx, hash(tok)); err != nil {
			t.Errorf("%s ended: %v", tok, err)
		}
	}
	if ok, err := q.AuthRevokeSession(ctx, dbgen.AuthRevokeSessionParams{TokenHash: hash("alice-pc"), SessionID: hash(string(hash("alice-phone")))}); err != nil || !ok {
		t.Errorf("revoke alice-phone: ok=%v err=%v, want true", ok, err)
	}
	if _, err := q.AuthSession(ctx, hash("alice-phone")); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("alice-phone after revoke: err=%v, want ErrNoRows", err)
	}

	// Ending the others keeps the caller's session and other users'.
	login(t, q, "alice@example.com", "alice-tablet")
	login(t, q, "alice@example.com", "alice-old")
	if n, err := q.AuthRevokeOtherSessions(ctx, hash("alice-pc")); err != nil || n != 2 {
		t.Errorf("revoke others = %d (%v), want 2", n, err)
	}
	for tok, alive := range map[string]bool{"alice-pc": true, "bob-pc": true, "alice-tablet": false, "alice-old": false} {
		_, err := q.AuthSession(ctx, hash(tok))
		if alive != (err == nil) {
			t.Errorf("%s: err=%v, want alive=%v", tok, err, alive)
		}
	}
	// An unknown or expired session ends nothing.
	if n, err := q.AuthRevokeOtherSessions(ctx, hash("unknown")); err != nil || n != 0 {
		t.Errorf("revoke others of unknown = %d (%v), want 0", n, err)
	}
	if _, err := e.admin.Exec(ctx, "UPDATE sessions SET expires_at = now() - interval '1 second' WHERE token_hash = sha256('bob-pc')"); err != nil {
		t.Fatal(err)
	}
	login(t, q, "bob@example.com", "bob-phone")
	if n, err := q.AuthRevokeOtherSessions(ctx, hash("bob-pc")); err != nil || n != 0 {
		t.Errorf("revoke others from an expired session = %d (%v), want 0", n, err)
	}
}

func TestSessionUseIsRecordedHourly(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()
	login(t, q, "alice@example.com", "s")
	touch := func(device string) {
		t.Helper()
		if err := q.AuthTouchSession(ctx, dbgen.AuthTouchSessionParams{TokenHash: hash("s"), Device: device}); err != nil {
			t.Fatal(err)
		}
	}
	device := func() string {
		t.Helper()
		var d string
		if err := e.admin.QueryRow(ctx, "SELECT device FROM sessions WHERE token_hash = sha256('s')").Scan(&d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	touch("first")
	touch("second")
	if got := device(); got != "first" {
		t.Errorf("device after two uses in an hour = %q, want first", got)
	}
	if _, err := e.admin.Exec(ctx, "UPDATE sessions SET last_used_at = now() - interval '61 minutes'"); err != nil {
		t.Fatal(err)
	}
	if s, err := q.AuthSession(ctx, hash("s")); err != nil || !s.TouchDue {
		t.Errorf("an hour later: touch_due=%v err=%v, want true", s.TouchDue, err)
	}
	touch(strings.Repeat("あ", 150))
	if got := device(); got != strings.Repeat("あ", 100) {
		t.Errorf("device = %q, want cut to 100 characters", got)
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
