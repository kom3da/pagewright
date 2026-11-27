package api

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

type sessionJSON struct {
	// ID names the session for revoking it. It is not the token, nor the
	// hash the server keeps (see migrations/00015_session_list.sql).
	ID         string    `json:"id"`
	Device     string    `json:"device"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Current    bool      `json:"current"`
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, sess *session) error {
	rows, err := s.DB.Queries().AuthListSessions(r.Context(), sess.tokenHash)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	out := make([]sessionJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionJSON{
			ID: hex.EncodeToString(row.ID), Device: row.Device, CreatedAt: row.CreatedAt,
			LastUsedAt: row.LastUsedAt, ExpiresAt: row.ExpiresAt, Current: row.IsCurrent,
		})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// revokeSession logs another device out. The current session is not
// found here: it ends with POST /api/auth/logout.
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := hex.DecodeString(r.PathValue("id"))
	if err != nil || len(id) != 32 {
		return errNotFound
	}
	ok, err := s.DB.Queries().AuthRevokeSession(r.Context(), dbgen.AuthRevokeSessionParams{TokenHash: sess.tokenHash, SessionID: id})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if !ok {
		return errNotFound
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// revokeOtherSessions logs every other device out (「ほかの端末からログアウト」).
func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request, sess *session) error {
	n, err := s.DB.Queries().AuthRevokeOtherSessions(r.Context(), sess.tokenHash)
	if err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}
	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
	return nil
}

// recordUse notes that the session is in use, from which device. The
// database keeps it to the hour, so this writes at most once an hour per
// session; a failure only leaves the time older.
func (s *Server) recordUse(r *http.Request, sess *session) {
	if !sess.touchDue {
		return
	}
	err := s.DB.Queries().AuthTouchSession(r.Context(), dbgen.AuthTouchSessionParams{
		TokenHash: sess.tokenHash, Device: deviceLabel(r.UserAgent()),
	})
	if err != nil {
		s.Logger.Warn("record session use", "err", err)
	}
	sess.touchDue = false
}

// deviceLabel names the browser and OS of a User-Agent in plain words
// ("Windows の Chrome"), so users can tell their sessions apart. Only this
// label is stored, not the User-Agent: it is all the list needs. Empty
// when neither is recognized.
func deviceLabel(ua string) string {
	browser := ""
	for _, b := range []struct{ token, name string }{
		// Order matters: Edge and Opera also say Chrome, Chrome says Safari.
		{"Edg/", "Edge"}, {"EdgiOS/", "Edge"}, {"EdgA/", "Edge"},
		{"OPR/", "Opera"}, {"SamsungBrowser/", "Samsung Internet"},
		{"Firefox/", "Firefox"}, {"FxiOS/", "Firefox"},
		{"Chrome/", "Chrome"}, {"CriOS/", "Chrome"},
		{"Safari/", "Safari"},
	} {
		if strings.Contains(ua, b.token) {
			browser = b.name
			break
		}
	}
	system := ""
	for _, o := range []struct{ token, name string }{
		// Android says Linux, iPhone says Mac OS X, ChromeOS says Linux.
		{"Android", "Android"}, {"iPhone", "iPhone"}, {"iPad", "iPad"},
		{"CrOS", "ChromeOS"}, {"Windows", "Windows"}, {"Macintosh", "Mac"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, o.token) {
			system = o.name
			break
		}
	}
	switch {
	case browser != "" && system != "":
		return system + " の " + browser
	case browser != "":
		return browser
	case system != "":
		return system + " のブラウザ"
	}
	return ""
}
