package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type sessionOut struct {
	ID         string
	Device     string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
	Current    bool
}

const (
	chromeOnWindows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	safariOnIPhone  = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
)

func TestSessions(t *testing.T) {
	e := newEnv(t)
	pc := e.loginFrom("alice@example.com", chromeOnWindows)
	phone := e.loginFrom("alice@example.com", safariOnIPhone)
	tablet := e.login("alice@example.com")
	bob := e.loginFrom("bob@example.com", chromeOnWindows)

	var list []sessionOut
	pc.mustDo(http.MethodGet, "/api/me/sessions", nil, &list, http.StatusOK)
	type row struct {
		Device  string
		Current bool
	}
	var got []row
	for _, s := range list {
		got = append(got, row{s.Device, s.Current})
		if len(s.ID) != 64 || s.LastUsedAt.IsZero() || !s.ExpiresAt.After(s.CreatedAt) {
			t.Errorf("session %+v: want a 64-character id, a last use and a later expiry", s)
		}
	}
	// The caller's own first, then the most recently used.
	want := []row{{"Windows の Chrome", true}, {"", false}, {"iPhone の Safari", false}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("alice's sessions from the PC (-want +got):\n%s", diff)
	}
	pcID, tabletID := list[0].ID, list[1].ID

	var bobList []sessionOut
	bob.mustDo(http.MethodGet, "/api/me/sessions", nil, &bobList, http.StatusOK)
	if len(bobList) != 1 || !bobList[0].Current {
		t.Fatalf("bob's sessions = %+v, want only his own", bobList)
	}
	bobID := bobList[0].ID

	// Another user's session, the caller's own and made-up ids are not found.
	for _, id := range []string{bobID, pcID, strings.Repeat("0", 64), "x", strings.ToUpper(bobID)} {
		if got, code := pc.errorCode(http.MethodDelete, "/api/me/sessions/"+id, nil); got != http.StatusNotFound || code != "not_found" {
			t.Errorf("DELETE session %q = %d %s, want 404", id, got, code)
		}
	}
	bob.mustDo(http.MethodGet, "/api/me", nil, nil, http.StatusOK)
	pc.mustDo(http.MethodGet, "/api/me", nil, nil, http.StatusOK)

	pc.mustDo(http.MethodDelete, "/api/me/sessions/"+tabletID, nil, nil, http.StatusNoContent)
	if got, _ := tablet.errorCode(http.MethodGet, "/api/me", nil); got != http.StatusUnauthorized {
		t.Errorf("tablet after being logged out = %d, want 401", got)
	}
	if got, _ := pc.errorCode(http.MethodDelete, "/api/me/sessions/"+tabletID, nil); got != http.StatusNotFound {
		t.Errorf("DELETE an ended session = %d, want 404", got)
	}

	// 「ほかの端末からログアウト」 keeps this one and other users'.
	var res struct{ Revoked int }
	pc.mustDo(http.MethodPost, "/api/me/sessions/logout-others", nil, &res, http.StatusOK)
	if res.Revoked != 1 {
		t.Errorf("logged out %d others, want 1 (the phone)", res.Revoked)
	}
	if got, _ := phone.errorCode(http.MethodGet, "/api/me", nil); got != http.StatusUnauthorized {
		t.Errorf("phone after logout-others = %d, want 401", got)
	}
	pc.mustDo(http.MethodGet, "/api/me/sessions", nil, &list, http.StatusOK)
	if len(list) != 1 || list[0].ID != pcID {
		t.Errorf("sessions after logout-others = %+v, want only the PC", list)
	}
	bob.mustDo(http.MethodGet, "/api/me/sessions", nil, &bobList, http.StatusOK)
	if len(bobList) != 1 {
		t.Errorf("bob has %d sessions after alice's logout-others, want 1", len(bobList))
	}

	anon := e.anon()
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/me/sessions"},
		{http.MethodPost, "/api/me/sessions/logout-others"},
		{http.MethodDelete, "/api/me/sessions/" + bobID},
	} {
		if got, _ := anon.errorCode(r.method, r.path, nil); got != http.StatusUnauthorized {
			t.Errorf("%s %s without login = %d, want 401", r.method, r.path, got)
		}
	}
	// Changes need the CSRF header like every other.
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/me/sessions/logout-others", nil)
	if got := pc.send(req, nil); got != http.StatusForbidden {
		t.Errorf("logout-others without X-Pagewright = %d, want 403", got)
	}
}

// A user picking no site yet can still manage sessions (they are not per site).
func TestSessionsBeforeChoosingSite(t *testing.T) {
	e := newEnv(t)
	carol := e.login("carol@example.com")
	var list []sessionOut
	carol.mustDo(http.MethodGet, "/api/me/sessions", nil, &list, http.StatusOK)
	if len(list) != 1 {
		t.Errorf("carol's sessions = %+v, want 1", list)
	}
}
