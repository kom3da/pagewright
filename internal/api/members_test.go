package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type memberOut struct {
	UserID      string
	Email       string
	DisplayName string
	Role        string
	AddedAt     time.Time
	You         bool
}

type invitationOut struct {
	ID        int64
	Email     string
	Role      string
	InvitedBy *string
	SentAt    time.Time
	ExpiresAt time.Time
	Expired   bool
}

// invite sends an invitation as c and returns it with the mailed token.
func (c *client) invite(email, role string) (invitationOut, string) {
	c.e.t.Helper()
	var inv invitationOut
	c.mustDo(http.MethodPost, "/api/invitations", map[string]string{"email": email, "role": role}, &inv, http.StatusCreated)
	return inv, c.e.mailedToken(strings.ToLower(email))
}

// mailedToken is the token in the last mail, which must be to `to`.
func (e *env) mailedToken(to string) string {
	e.t.Helper()
	m, ok := e.mail.last()
	if !ok || m.To != to {
		e.t.Fatalf("last mail = %+v, want one to %s", m, to)
	}
	match := tokenRE.FindStringSubmatch(m.Body)
	if match == nil {
		e.t.Fatalf("no token in mail: %s", m.Body)
	}
	token, _ := url.QueryUnescape(match[1])
	return token
}

func (c *client) members() map[string]memberOut {
	c.e.t.Helper()
	var list []memberOut
	c.mustDo(http.MethodGet, "/api/members", nil, &list, http.StatusOK)
	out := map[string]memberOut{}
	for _, m := range list {
		out[m.Email] = m
	}
	return out
}

func (e *env) userID(email string) string {
	e.t.Helper()
	var id string
	if err := e.db.Admin.QueryRow(context.Background(), "SELECT id::text FROM users WHERE email = $1", email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

type meOut struct {
	User struct {
		Email       string
		DisplayName string
	}
	Sites         []struct{ ID, Name, Role string }
	CurrentSiteID *string
}

func TestInviteNewUser(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	if err := alice.do(http.MethodPatch, "/api/me", map[string]string{"displayName": "山田 花子"}, nil); err != http.StatusOK {
		t.Fatalf("set alice's name = %d", err)
	}

	inv, token := alice.invite("New@Example.com", "editor")
	if inv.Email != "new@example.com" || inv.Role != "editor" || inv.Expired || inv.InvitedBy == nil || *inv.InvitedBy != "山田 花子" {
		t.Errorf("invitation = %+v", inv)
	}
	if d := time.Until(inv.ExpiresAt); d < 6*24*time.Hour || d > 7*24*time.Hour {
		t.Errorf("expires in %v, want 7 days", d)
	}
	m, _ := e.mail.last()
	for _, want := range []string{"山田 花子（alice@example.com）さん", "「サイトA」", "編集者として招待", "/admin/invite/accept?token=", "有効期限は", "心当たりがない場合"} {
		if !strings.Contains(m.Subject+m.Body, want) {
			t.Errorf("mail lacks %q:\n%s\n%s", want, m.Subject, m.Body)
		}
	}

	// The link's page shows what the invitation is for, without using it.
	anon := e.anon()
	var preview struct {
		SiteName, Email, Role string
		Expired, HasAccount   bool
	}
	for range 2 {
		anon.mustDo(http.MethodPost, "/api/invitations/lookup", map[string]string{"token": token}, &preview, http.StatusOK)
	}
	if preview.SiteName != "サイトA" || preview.Email != "new@example.com" || preview.Role != "editor" || preview.Expired || preview.HasAccount {
		t.Errorf("lookup = %+v", preview)
	}

	if got, code := anon.errorCode(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token, "displayName": strings.Repeat("あ", 51)}); got != http.StatusUnprocessableEntity || code != "invalid_display_name" {
		t.Errorf("accept with a long name = %d %s", got, code)
	}
	var me meOut
	anon.mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token, "displayName": " 新人 "}, &me, http.StatusOK)
	if me.User.Email != "new@example.com" || me.User.DisplayName != "新人" || me.CurrentSiteID == nil || *me.CurrentSiteID != e.tenantA.String() {
		t.Errorf("me after accepting = %+v", me)
	}
	// Logged in: the cookie works.
	anon.mustDo(http.MethodGet, "/api/contents?type=post", nil, nil, http.StatusOK)

	if got := alice.members()["new@example.com"]; got.Role != "editor" || got.DisplayName != "新人" {
		t.Errorf("new member = %+v", got)
	}
	var list []invitationOut
	alice.mustDo(http.MethodGet, "/api/invitations", nil, &list, http.StatusOK)
	if len(list) != 0 {
		t.Errorf("invitations after accepting = %+v, want none", list)
	}
	if got, code := e.anon().errorCode(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}); got != http.StatusBadRequest || code != "invalid_token" {
		t.Errorf("second accept = %d %s, want 400 invalid_token", got, code)
	}
}

func TestAcceptInvitationFlows(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")

	t.Run("existing user, logged out", func(t *testing.T) {
		_, token := alice.invite("bob@example.com", "admin")
		c := e.anon()
		var me meOut
		c.mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}, &me, http.StatusOK)
		if me.CurrentSiteID == nil || *me.CurrentSiteID != e.tenantA.String() || len(me.Sites) != 2 {
			t.Errorf("bob's me = %+v, want A selected of A and B", me)
		}
	})

	t.Run("logged in as the invited address", func(t *testing.T) {
		bob := e.login("bob@example.com")
		var b invitationOut
		bob.mustDo(http.MethodPost, "/api/me/site", map[string]string{"siteId": e.tenantB.String()}, nil, http.StatusOK)
		bob.mustDo(http.MethodPost, "/api/invitations", map[string]string{"email": "alice@example.com", "role": "editor"}, &b, http.StatusCreated)
		token := e.mailedToken("alice@example.com")
		var me meOut
		alice2 := e.login("alice@example.com")
		alice2.mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}, &me, http.StatusOK)
		if me.CurrentSiteID == nil || *me.CurrentSiteID != e.tenantB.String() {
			t.Errorf("alice's me = %+v, want B selected", me)
		}
	})

	t.Run("wrong account", func(t *testing.T) {
		_, token := alice.invite("erin@example.com", "editor")
		dave := e.login("dave@example.com")
		var out struct{ Error struct{ Code, Message string } }
		if got := dave.do(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}, &out); got != http.StatusConflict || out.Error.Code != "wrong_account" || !strings.Contains(out.Error.Message, "dave@example.com") {
			t.Errorf("accept as dave = %d %+v", got, out)
		}
		// Still usable by erin.
		e.anon().mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}, nil, http.StatusOK)
	})

	t.Run("already a member", func(t *testing.T) {
		if got, code := alice.errorCode(http.MethodPost, "/api/invitations", map[string]string{"email": "Dave@example.com", "role": "admin"}); got != http.StatusConflict || code != "already_member" {
			t.Errorf("inviting dave = %d %s, want 409 already_member", got, code)
		}
	})

	t.Run("expired", func(t *testing.T) {
		inv, token := alice.invite("frank@example.com", "editor")
		if _, err := e.db.Admin.Exec(context.Background(), "UPDATE invitations SET expires_at = now() - interval '1 minute' WHERE tenant_id = $1 AND id = $2", e.tenantA, inv.ID); err != nil {
			t.Fatal(err)
		}
		var list []invitationOut
		alice.mustDo(http.MethodGet, "/api/invitations", nil, &list, http.StatusOK)
		if len(list) != 1 || !list[0].Expired {
			t.Errorf("list = %+v, want frank's, expired", list)
		}
		var preview struct{ Expired bool }
		e.anon().mustDo(http.MethodPost, "/api/invitations/lookup", map[string]string{"token": token}, &preview, http.StatusOK)
		if !preview.Expired {
			t.Error("lookup: not expired")
		}
		if got, code := e.anon().errorCode(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}); got != http.StatusBadRequest || code != "invitation_expired" {
			t.Errorf("accept = %d %s, want 400 invitation_expired", got, code)
		}

		// Sending again makes a new link that works; the old one does not.
		var again invitationOut
		alice.mustDo(http.MethodPost, fmt.Sprintf("/api/invitations/%d/resend", inv.ID), nil, &again, http.StatusOK)
		if again.Expired || again.ID != inv.ID {
			t.Errorf("resent = %+v", again)
		}
		fresh := e.mailedToken("frank@example.com")
		if got, code := e.anon().errorCode(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}); got != http.StatusBadRequest || code != "invalid_token" {
			t.Errorf("old link after resend = %d %s, want invalid_token", got, code)
		}
		e.anon().mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": fresh}, nil, http.StatusOK)
	})

	t.Run("cancelled", func(t *testing.T) {
		inv, token := alice.invite("gina@example.com", "editor")
		alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/invitations/%d", inv.ID), nil, nil, http.StatusNoContent)
		if got, code := e.anon().errorCode(http.MethodPost, "/api/invitations/lookup", map[string]string{"token": token}); got != http.StatusBadRequest || code != "invalid_token" {
			t.Errorf("lookup = %d %s, want invalid_token", got, code)
		}
		if got, code := e.anon().errorCode(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}); got != http.StatusBadRequest || code != "invalid_token" {
			t.Errorf("accept = %d %s, want invalid_token", got, code)
		}
		if got := alice.do(http.MethodDelete, fmt.Sprintf("/api/invitations/%d", inv.ID), nil, nil); got != http.StatusNotFound {
			t.Errorf("cancel again = %d, want 404", got)
		}
	})

	t.Run("invite again replaces", func(t *testing.T) {
		_, old := alice.invite("hana@example.com", "editor")
		inv, token := alice.invite("hana@example.com", "admin")
		var list []invitationOut
		alice.mustDo(http.MethodGet, "/api/invitations", nil, &list, http.StatusOK)
		if len(list) != 1 || list[0].ID != inv.ID || list[0].Role != "admin" {
			t.Errorf("list = %+v, want one, as admin", list)
		}
		if got, _ := e.anon().errorCode(http.MethodPost, "/api/invitations/accept", map[string]string{"token": old}); got != http.StatusBadRequest {
			t.Errorf("old link = %d, want 400", got)
		}
		e.anon().mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}, nil, http.StatusOK)
	})
}

func TestMembersAreAdminOnly(t *testing.T) {
	e := newEnv(t)
	dave := e.login("dave@example.com") // editor of A
	alice := e.userID("alice@example.com")
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/members", nil},
		{http.MethodPatch, "/api/members/" + alice, map[string]string{"role": "editor"}},
		{http.MethodDelete, "/api/members/" + alice, nil},
		{http.MethodGet, "/api/invitations", nil},
		{http.MethodPost, "/api/invitations", map[string]string{"email": "x@example.com", "role": "admin"}},
		{http.MethodPost, "/api/invitations/1/resend", nil},
		{http.MethodDelete, "/api/invitations/1", nil},
	} {
		if got, code := dave.errorCode(r.method, r.path, r.body); got != http.StatusForbidden || code != "forbidden" {
			t.Errorf("editor %s %s = %d %s, want 403 forbidden", r.method, r.path, got, code)
		}
	}
	if got, _ := e.anon().errorCode(http.MethodGet, "/api/members", nil); got != http.StatusUnauthorized {
		t.Errorf("anonymous GET /api/members = %d, want 401", got)
	}
}

func TestMemberRolesAndRemoval(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	dave := e.login("dave@example.com")
	aliceID, daveID := e.userID("alice@example.com"), e.userID("dave@example.com")

	members := alice.members()
	if len(members) != 3 || !members["alice@example.com"].You || members["dave@example.com"].You || members["dave@example.com"].Role != "editor" {
		t.Errorf("members = %+v", members)
	}

	// alice is the only admin.
	if got, code := alice.errorCode(http.MethodPatch, "/api/members/"+aliceID, map[string]string{"role": "editor"}); got != http.StatusConflict || code != "last_admin" {
		t.Errorf("demote the last admin = %d %s, want 409 last_admin", got, code)
	}
	if got, code := alice.errorCode(http.MethodDelete, "/api/members/"+aliceID, nil); got != http.StatusConflict || code != "last_admin" {
		t.Errorf("remove the last admin = %d %s, want 409 last_admin", got, code)
	}
	if got, code := alice.errorCode(http.MethodPatch, "/api/members/"+daveID, map[string]string{"role": "owner"}); got != http.StatusUnprocessableEntity || code != "invalid_role" {
		t.Errorf("unknown role = %d %s", got, code)
	}

	// dave writes a post, becomes an admin, and alice steps down.
	post := dave.newContent("post", "デイブの記事", true)
	var m memberOut
	alice.mustDo(http.MethodPatch, "/api/members/"+daveID, map[string]string{"role": "admin"}, &m, http.StatusOK)
	if m.Role != "admin" || m.Email != "dave@example.com" {
		t.Errorf("dave = %+v", m)
	}
	alice.mustDo(http.MethodPatch, "/api/members/"+aliceID, map[string]string{"role": "editor"}, nil, http.StatusOK)
	if got, _ := alice.errorCode(http.MethodGet, "/api/members", nil); got != http.StatusForbidden {
		t.Errorf("alice as editor lists members = %d, want 403", got)
	}

	// dave removes alice, then alice re-joins; dave's own removal keeps his post.
	dave.mustDo(http.MethodDelete, "/api/members/"+aliceID, nil, nil, http.StatusNoContent)
	if got, code := alice.errorCode(http.MethodGet, "/api/contents?type=post", nil); got != http.StatusForbidden || code != "no_site" {
		t.Errorf("removed alice reads contents = %d %s, want 403 no_site", got, code)
	}
	_, token := dave.invite("alice@example.com", "admin")
	alice.mustDo(http.MethodPost, "/api/invitations/accept", map[string]string{"token": token}, nil, http.StatusOK)
	alice.mustDo(http.MethodDelete, "/api/members/"+daveID, nil, nil, http.StatusNoContent)
	var d detail
	alice.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", post.ID), nil, &d, http.StatusOK)
	if d.Title != "デイブの記事" || d.Status != "published" {
		t.Errorf("dave's post after removing him = %+v", d)
	}
	if got := alice.do(http.MethodDelete, "/api/members/"+daveID, nil, nil); got != http.StatusNotFound {
		t.Errorf("remove dave again = %d, want 404", got)
	}
	if got := alice.do(http.MethodDelete, "/api/members/not-a-uuid", nil, nil); got != http.StatusNotFound {
		t.Errorf("remove a bad id = %d, want 404", got)
	}
}

func TestMembersTenantIsolation(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com") // admin of A
	bob := e.login("bob@example.com")     // admin of B
	bobID := e.userID("bob@example.com")

	if _, ok := alice.members()["bob@example.com"]; ok {
		t.Error("A's members include bob of B")
	}
	if got := alice.do(http.MethodPatch, "/api/members/"+bobID, map[string]string{"role": "editor"}, nil); got != http.StatusNotFound {
		t.Errorf("A changes B's admin = %d, want 404", got)
	}
	if got := alice.do(http.MethodDelete, "/api/members/"+bobID, nil, nil); got != http.StatusNotFound {
		t.Errorf("A removes B's admin = %d, want 404", got)
	}
	// carol is in both; A removing her leaves B alone.
	alice.mustDo(http.MethodDelete, "/api/members/"+e.userID("carol@example.com"), nil, nil, http.StatusNoContent)
	if _, ok := bob.members()["carol@example.com"]; !ok {
		t.Error("carol left B too")
	}
	if b := bob.members()["bob@example.com"]; b.Role != "admin" {
		t.Errorf("bob in B = %+v", b)
	}

	inv, _ := bob.invite("x@example.com", "editor")
	var list []invitationOut
	alice.mustDo(http.MethodGet, "/api/invitations", nil, &list, http.StatusOK)
	if len(list) != 0 {
		t.Errorf("A sees invitations %+v", list)
	}
	if got := alice.do(http.MethodPost, fmt.Sprintf("/api/invitations/%d/resend", inv.ID), nil, nil); got != http.StatusNotFound {
		t.Errorf("A resends B's invitation = %d, want 404", got)
	}
	if got := alice.do(http.MethodDelete, fmt.Sprintf("/api/invitations/%d", inv.ID), nil, nil); got != http.StatusNotFound {
		t.Errorf("A cancels B's invitation = %d, want 404", got)
	}
	bob.mustDo(http.MethodGet, "/api/invitations", nil, &list, http.StatusOK)
	if len(list) != 1 || list[0].ID != inv.ID {
		t.Errorf("B's invitations = %+v", list)
	}
	// Each site may invite the same address on its own.
	alice.invite("x@example.com", "admin")
	bob.mustDo(http.MethodGet, "/api/invitations", nil, &list, http.StatusOK)
	if len(list) != 1 || list[0].Role != "editor" {
		t.Errorf("B's invitation after A's = %+v", list)
	}
}

func TestInvitationValidationAndRateLimit(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	for _, in := range []map[string]string{
		{"email": "not an address", "role": "editor"},
		{"email": "Taro <taro@example.jp>", "role": "editor"},
		{"email": "taro@example.jp", "role": "owner"},
	} {
		if got, _ := alice.errorCode(http.MethodPost, "/api/invitations", in); got != http.StatusUnprocessableEntity {
			t.Errorf("invite %v = %d, want 422", in, got)
		}
	}
	for i := range 5 {
		if got := alice.do(http.MethodPost, "/api/invitations", map[string]string{"email": "spam@example.com", "role": "editor"}, nil); got != http.StatusCreated {
			t.Fatalf("invite %d = %d, want 201", i+1, got)
		}
	}
	if got, code := alice.errorCode(http.MethodPost, "/api/invitations", map[string]string{"email": "spam@example.com", "role": "editor"}); got != http.StatusTooManyRequests || code != "too_many_requests" {
		t.Errorf("6th invitation to one address = %d %s, want 429", got, code)
	}
	// The limit is per address across sites.
	bob := e.login("bob@example.com")
	if got, _ := bob.errorCode(http.MethodPost, "/api/invitations", map[string]string{"email": "spam@example.com", "role": "editor"}); got != http.StatusTooManyRequests {
		t.Errorf("from another site = %d, want 429", got)
	}
}

func TestDisplayName(t *testing.T) {
	e := newEnv(t)
	carol := e.login("carol@example.com")
	var me meOut
	carol.mustDo(http.MethodPatch, "/api/me", map[string]string{"displayName": "  鈴木 花子 "}, &me, http.StatusOK)
	if me.User.DisplayName != "鈴木 花子" {
		t.Errorf("name = %q", me.User.DisplayName)
	}
	carol.mustDo(http.MethodGet, "/api/me", nil, &me, http.StatusOK)
	if me.User.DisplayName != "鈴木 花子" {
		t.Errorf("name after reload = %q", me.User.DisplayName)
	}
	for _, bad := range []string{strings.Repeat("あ", 51), "改行\nあり"} {
		if got, code := carol.errorCode(http.MethodPatch, "/api/me", map[string]string{"displayName": bad}); got != http.StatusUnprocessableEntity || code != "invalid_display_name" {
			t.Errorf("name %q = %d %s, want 422 invalid_display_name", bad, got, code)
		}
	}
	// Clearing it is allowed (the email is shown instead).
	carol.mustDo(http.MethodPatch, "/api/me", map[string]string{"displayName": ""}, &me, http.StatusOK)
	if me.User.DisplayName != "" {
		t.Errorf("cleared name = %q", me.User.DisplayName)
	}
	if got, _ := e.anon().errorCode(http.MethodPatch, "/api/me", map[string]string{"displayName": "x"}); got != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", got)
	}
}
