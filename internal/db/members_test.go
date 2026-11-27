package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

var errRollback = errors.New("rollback")

// Role changes and removals by the app reach only the current site.
func TestMembershipChangesStayInSite(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	// bob is B's admin; carol is an editor of both.
	for _, q := range []string{
		"UPDATE memberships SET role = 'admin' WHERE user_id = $1",
		"DELETE FROM memberships WHERE user_id = $1",
	} {
		for _, user := range []uuid.UUID{e.bob, e.carol} {
			n := 0
			err := pgx.BeginFunc(ctx, e.app, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", e.tenantA.String()); err != nil {
					return err
				}
				tag, err := tx.Exec(ctx, q, user)
				if err != nil {
					return err
				}
				n = int(tag.RowsAffected())
				return errRollback
			})
			if !errors.Is(err, errRollback) {
				t.Fatalf("%q for %s: %v", q, user, err)
			}
			want := 0
			if user == e.carol {
				want = 1 // her membership of A only
			}
			if n != want {
				t.Errorf("%q in A for %s: %d rows, want %d", q, user, n, want)
			}
		}
	}
	var role string
	if err := e.admin.QueryRow(ctx, "SELECT role FROM memberships WHERE tenant_id = $1 AND user_id = $2", e.tenantB, e.bob).Scan(&role); err != nil || role != "admin" {
		t.Errorf("bob in B: role %q err %v, want admin", role, err)
	}
}

func lastAdmin(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23514" && pe.ConstraintName == "memberships_keep_admin"
}

func TestSiteKeepsAnAdmin(t *testing.T) {
	e := newTestEnv(t)
	// alice is A's only admin.
	for _, q := range []string{
		"UPDATE memberships SET role = 'editor' WHERE user_id = $1",
		"DELETE FROM memberships WHERE user_id = $1",
	} {
		if err := e.execInTenant(e.tenantA, q, e.alice); !lastAdmin(err) {
			t.Errorf("%q for A's last admin: err = %v, want memberships_keep_admin", q, err)
		}
	}
	// Removing everyone at once is refused too.
	if err := e.execInTenant(e.tenantA, "DELETE FROM memberships"); !lastAdmin(err) {
		t.Errorf("removing all of A's members: err = %v, want memberships_keep_admin", err)
	}
	// Editors come and go freely.
	if err := e.execInTenant(e.tenantA, "UPDATE memberships SET role = 'editor' WHERE user_id = $1", e.carol); err != nil {
		t.Errorf("editor stays editor: %v", err)
	}
	// With a second admin, the first may step down.
	if err := e.execInTenant(e.tenantA, "UPDATE memberships SET role = 'admin' WHERE user_id = $1", e.carol); err != nil {
		t.Fatal(err)
	}
	if err := e.execInTenant(e.tenantA, "UPDATE memberships SET role = 'editor' WHERE user_id = $1", e.alice); err != nil {
		t.Errorf("demoting alice with carol an admin: %v", err)
	}
}

// Two admins demoting each other at the same moment: only one succeeds.
func TestSiteKeepsAnAdminConcurrently(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if err := e.execInTenant(e.tenantA, "UPDATE memberships SET role = 'admin' WHERE user_id = $1", e.carol); err != nil {
		t.Fatal(err)
	}
	begin := func() pgx.Tx {
		tx, err := e.app.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback(ctx) })
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", e.tenantA.String()); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	first, second := begin(), begin()
	if _, err := first.Exec(ctx, "UPDATE memberships SET role = 'editor' WHERE user_id = $1", e.alice); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := second.Exec(ctx, "UPDATE memberships SET role = 'editor' WHERE user_id = $1", e.carol)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("second demotion did not wait for the first: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !lastAdmin(err) {
		t.Errorf("second demotion: err = %v, want memberships_keep_admin", err)
	}
}

// invite stores an invitation in tenant's site with the token tok.
func (e *testEnv) invite(t *testing.T, ctx context.Context, email, role, tok string, ttl time.Duration) dbgen.Invitation {
	t.Helper()
	var inv dbgen.Invitation
	err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
		var err error
		inv, err = q.UpsertInvitation(ctx, dbgen.UpsertInvitationParams{
			Email: email, Role: role, TokenHash: hash(tok), ExpiresAt: time.Now().Add(ttl),
		})
		return err
	})
	if err != nil {
		t.Fatalf("invite %s: %v", email, err)
	}
	return inv
}

func (e *testEnv) accept(t *testing.T, tok, session, newSession, name string) string {
	t.Helper()
	var current []byte
	if session != "" {
		current = hash(session)
	}
	res, err := e.db.Queries().AuthAcceptInvitation(context.Background(), dbgen.AuthAcceptInvitationParams{
		TokenHash: hash(tok), SessionHash: current, NewSessionHash: hash(newSession), DisplayName: name, TtlSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("accept %s: %v", tok, err)
	}
	return res
}

func TestAcceptInvitation(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()

	t.Run("new user", func(t *testing.T) {
		e.invite(t, e.ctxA, "Dave@Example.com", "editor", "dave-invite", time.Hour)
		got, err := q.InvitationLookup(ctx, hash("dave-invite"))
		if err != nil || got.SiteName != "A" || got.Email != "dave@example.com" || got.Role != "editor" || got.HasAccount {
			t.Fatalf("lookup = %+v, %v", got, err)
		}
		if res := e.accept(t, "dave-invite", "", "dave-session", "デイブ"); res != "joined" {
			t.Fatalf("accept = %q, want joined", res)
		}
		s, err := q.AuthSession(ctx, hash("dave-session"))
		if err != nil || s.Email != "dave@example.com" || s.DisplayName != "デイブ" || s.TenantID != e.tenantA || s.Role != "editor" {
			t.Errorf("session = %+v, %v; want dave, editor of A", s, err)
		}
		// The link works once.
		if res := e.accept(t, "dave-invite", "", "dave-session2", ""); res != "invalid" {
			t.Errorf("second accept = %q, want invalid", res)
		}
		if _, err := q.InvitationLookup(ctx, hash("dave-invite")); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("lookup after use: err = %v, want no rows", err)
		}
	})

	t.Run("existing user", func(t *testing.T) {
		// bob (B's admin) is invited to A, and is logged out.
		e.invite(t, e.ctxA, "bob@example.com", "admin", "bob-invite", time.Hour)
		if got, _ := q.InvitationLookup(ctx, hash("bob-invite")); !got.HasAccount {
			t.Error("lookup: has_account = false for bob")
		}
		if res := e.accept(t, "bob-invite", "", "bob-session", "ignored"); res != "joined" {
			t.Fatalf("accept = %q, want joined", res)
		}
		s, err := q.AuthSession(ctx, hash("bob-session"))
		if err != nil || s.UserID != e.bob || s.DisplayName != "" || s.TenantID != e.tenantA || s.Role != "admin" {
			t.Errorf("session = %+v, %v; want bob, admin of A, name unchanged", s, err)
		}
	})

	t.Run("logged in as the invited address", func(t *testing.T) {
		e.invite(t, e.ctxB, "alice@example.com", "editor", "alice-invite", time.Hour)
		login(t, q, "alice@example.com", "alice-session")
		if res := e.accept(t, "alice-invite", "alice-session", "unused", ""); res != "joined_current" {
			t.Fatalf("accept = %q, want joined_current", res)
		}
		s, err := q.AuthSession(ctx, hash("alice-session"))
		if err != nil || s.TenantID != e.tenantB || s.Role != "editor" {
			t.Errorf("session = %+v, %v; want B selected", s, err)
		}
		if _, err := q.AuthSession(ctx, hash("unused")); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("a second session was opened: %v", err)
		}
	})

	t.Run("wrong account", func(t *testing.T) {
		e.invite(t, e.ctxB, "erin@example.com", "editor", "erin-invite", time.Hour)
		login(t, q, "carol@example.com", "carol-session")
		if res := e.accept(t, "erin-invite", "carol-session", "erin-session", ""); res != "wrong_account" {
			t.Fatalf("accept = %q, want wrong_account", res)
		}
		// Nothing changed: the invitation still works.
		if _, err := q.InvitationLookup(ctx, hash("erin-invite")); err != nil {
			t.Errorf("invitation used up by the wrong account: %v", err)
		}
		if _, err := e.db.FindUserByEmail(ctx, "erin@example.com"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("erin created: %v", err)
		}
		// An unknown session counts as logged out.
		if res := e.accept(t, "erin-invite", "no-such-session", "erin-session", ""); res != "joined" {
			t.Errorf("accept with an unknown session = %q, want joined", res)
		}
	})

	t.Run("already a member", func(t *testing.T) {
		// carol is A's editor; an invitation as admin leaves her role.
		e.invite(t, e.ctxA, "carol@example.com", "admin", "carol-invite", time.Hour)
		if res := e.accept(t, "carol-invite", "", "carol-session2", ""); res != "joined" {
			t.Fatalf("accept = %q, want joined", res)
		}
		s, err := q.AuthSession(ctx, hash("carol-session2"))
		if err != nil || s.TenantID != e.tenantA || s.Role != "editor" {
			t.Errorf("session = %+v, %v; want A selected, still editor", s, err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		e.invite(t, e.ctxA, "frank@example.com", "editor", "frank-invite", time.Hour)
		if _, err := e.admin.Exec(ctx, "UPDATE invitations SET expires_at = now() - interval '1 second' WHERE token_hash = $1", hash("frank-invite")); err != nil {
			t.Fatal(err)
		}
		if got, err := q.InvitationLookup(ctx, hash("frank-invite")); err != nil || got.ExpiresAt.After(time.Now()) {
			t.Errorf("lookup of expired = %+v, %v; want it, expired", got, err)
		}
		if res := e.accept(t, "frank-invite", "", "frank-session", ""); res != "expired" {
			t.Errorf("accept = %q, want expired", res)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		inv := e.invite(t, e.ctxA, "gina@example.com", "editor", "gina-invite", time.Hour)
		e.inA(t, func(q *dbgen.Queries) error {
			_, err := q.DeleteInvitation(e.ctxA, inv.ID)
			return err
		})
		if res := e.accept(t, "gina-invite", "", "gina-session", ""); res != "invalid" {
			t.Errorf("accept = %q, want invalid", res)
		}
	})

	t.Run("replaced", func(t *testing.T) {
		e.invite(t, e.ctxA, "hana@example.com", "editor", "hana-old", time.Hour)
		e.invite(t, e.ctxA, "hana@example.com", "admin", "hana-new", time.Hour)
		if res := e.accept(t, "hana-old", "", "hana-session", ""); res != "invalid" {
			t.Errorf("old link = %q, want invalid", res)
		}
		if res := e.accept(t, "hana-new", "", "hana-session", ""); res != "joined" {
			t.Errorf("new link = %q, want joined", res)
		}
		if s, _ := q.AuthSession(ctx, hash("hana-session")); s.Role != "admin" {
			t.Errorf("hana's role = %q, want admin (the newer invitation)", s.Role)
		}
	})
}

func TestInvitationsAreIsolated(t *testing.T) {
	e := newTestEnv(t)
	a := e.invite(t, e.ctxA, "x@example.com", "editor", "a-invite", time.Hour)
	// The same address may be invited by each site.
	b := e.invite(t, e.ctxB, "x@example.com", "admin", "b-invite", time.Hour)

	err := e.db.InTenant(e.ctxA, func(q *dbgen.Queries) error {
		list, err := q.ListInvitations(e.ctxA)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != a.ID || list[0].Role != "editor" {
			t.Errorf("A's invitations = %+v, want only A's", list)
		}
		if _, err := q.GetInvitation(e.ctxA, b.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("A reads B's invitation: %v", err)
		}
		if _, err := q.RenewInvitation(e.ctxA, dbgen.RenewInvitationParams{ID: b.ID, TokenHash: hash("stolen"), ExpiresAt: time.Now().Add(time.Hour)}); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("A renews B's invitation: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.execInTenant(e.tenantA, "DELETE FROM invitations WHERE tenant_id = $1", e.tenantB); err != nil {
		t.Fatal(err)
	}
	if err := e.execInTenant(e.tenantA, "INSERT INTO invitations (tenant_id, email, role, token_hash, expires_at) VALUES ($1, 'y@example.com', 'admin', $2, now())", e.tenantB, hash("y")); pgCode(err) != "42501" {
		t.Errorf("A invites to B: err = %v, want SQLSTATE 42501", err)
	}
	if got, err := e.db.Queries().InvitationLookup(context.Background(), hash("b-invite")); err != nil || got.SiteName != "B" {
		t.Errorf("B's invitation after A's attempts = %+v, %v", got, err)
	}
}

func TestSetDisplayName(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	q := e.db.Queries()
	login(t, q, "carol@example.com", "carol")
	if ok, err := q.AuthSetDisplayName(ctx, dbgen.AuthSetDisplayNameParams{TokenHash: hash("carol"), DisplayName: "山田 花子"}); err != nil || !ok {
		t.Fatalf("set name: ok=%v err=%v", ok, err)
	}
	// Both of carol's sites see it.
	for _, ctx := range []context.Context{e.ctxA, e.ctxB} {
		err := e.db.InTenant(ctx, func(q *dbgen.Queries) error {
			u, err := q.GetUser(ctx, e.carol)
			if err == nil && u.DisplayName != "山田 花子" {
				t.Errorf("name = %q", u.DisplayName)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := q.AuthSetDisplayName(ctx, dbgen.AuthSetDisplayNameParams{TokenHash: hash("nobody"), DisplayName: "x"}); err != nil || ok {
		t.Errorf("unknown session: ok=%v err=%v, want false", ok, err)
	}
	if _, err := q.AuthSetDisplayName(ctx, dbgen.AuthSetDisplayNameParams{TokenHash: hash("carol"), DisplayName: strings.Repeat("あ", 101)}); pgCode(err) != "23514" {
		t.Errorf("101 characters: err = %v, want SQLSTATE 23514", err)
	}
}

func TestInvitationCleanup(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.invite(t, e.ctxA, "old@example.com", "editor", "old", time.Hour)
	e.invite(t, e.ctxA, "recent@example.com", "editor", "recent", time.Hour)
	for tok, ago := range map[string]string{"old": "31 days", "recent": "29 days"} {
		if _, err := e.admin.Exec(ctx, "UPDATE invitations SET expires_at = now() - $2::interval WHERE token_hash = $1", hash(tok), ago); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := e.db.Queries().AuthCleanup(ctx); err != nil || n != 1 {
		t.Fatalf("AuthCleanup = %d, %v; want 1 removed", n, err)
	}
	if _, err := e.db.Queries().InvitationLookup(ctx, hash("recent")); err != nil {
		t.Errorf("recently expired invitation removed: %v", err)
	}
}
