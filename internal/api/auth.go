package api

import (
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	pwmail "github.com/kom3da/pagewright/internal/mail"
)

type meJSON struct {
	User          userJSON   `json:"user"`
	Sites         []siteJSON `json:"sites"`
	CurrentSiteID *string    `json:"currentSiteId"`
}

type userJSON struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type siteJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// maxAuthBytes caps the bodies of the endpoints anyone can call without a
// login (an address or a token fit with room to spare).
const maxAuthBytes = 64 << 10

func (s *Server) loginLink(w http.ResponseWriter, r *http.Request, _ *session) error {
	var in struct {
		Email string `json:"email"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBytes)
	if err := decode(w, r, &in); err != nil {
		return err
	}
	email := strings.TrimSpace(in.Email)
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		return invalid("invalid_email", "メールアドレスの形式が正しくありません")
	}

	if !s.allowByIP(s.loginByIP, r) {
		return &apiError{http.StatusTooManyRequests, "too_many_requests", "ログイン用のリンクの送信が続いています。しばらく待ってからお試しください"}
	}
	// Past the per-address limit, answer as usual but send nothing: the
	// limit must not reveal registration, nor let others lock someone out
	// with errors.
	if !s.loginByEmail.allow(strings.ToLower(email)) {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}

	// Both registered and unknown addresses take the same steps (the token
	// insert and the mail job, in one transaction), so the response time
	// does not tell them apart. An unknown address rolls it all back.
	ctx := r.Context()
	tx, err := s.DB.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	token := newToken()
	found, err := dbgen.New(tx).AuthRequestLogin(ctx, dbgen.AuthRequestLoginParams{
		Email: email, TokenHash: hashToken(token), TtlSeconds: int64(loginTTL.Seconds()),
	})
	if err != nil {
		return fmt.Errorf("request login: %w", err)
	}
	link := s.BaseURL + "/admin/login/verify?token=" + url.QueryEscape(token)
	msg := pwmail.Message{
		To:      email,
		Subject: "Pagewright ログイン用のリンク",
		Body: "Pagewright の管理画面にログインするには、次のリンクを開いて「ログイン」ボタンを押してください。\n\n" +
			link + "\n\n" +
			"このリンクの有効期限は15分で、1回だけ使えます。\n" +
			"心当たりがない場合は、このメールを無視してください。\n",
	}
	if s.Jobs == nil {
		// No queue (tests and tools only): send directly after commit.
		if found {
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit: %w", err)
			}
			s.sendMail(ctx, nil, msg)
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	if err := s.Jobs.EnqueueMail(ctx, tx, msg); err != nil {
		s.Logger.Error("send mail", "subject", msg.Subject, "err", err)
		found = false // a token nobody receives is of no use
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}
	// Answer the same whether or not the address is registered.
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request, _ *session) error {
	var in struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBytes)
	if err := decode(w, r, &in); err != nil {
		return err
	}
	sessionToken := newToken()
	user, err := s.DB.Queries().AuthLogin(r.Context(), dbgen.AuthLoginParams{
		LoginTokenHash: hashToken(in.Token), SessionTokenHash: hashToken(sessionToken),
		TtlSeconds: int64(sessionTTL.Seconds()),
	})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if user == uuid.Nil {
		return &apiError{http.StatusBadRequest, "invalid_token", "ログイン用のリンクが無効か、有効期限が切れています。もう一度ログイン用のリンクを送ってください"}
	}
	http.SetCookie(w, s.cookie(sessionToken, int(sessionTTL.Seconds())))
	sess, err := s.sessionByHash(r.Context(), hashToken(sessionToken))
	if err != nil {
		return err
	}
	s.recordUse(r, sess)
	return s.writeMe(w, r, sess)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := s.DB.Queries().AuthLogout(r.Context(), sess.tokenHash); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	http.SetCookie(w, s.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, sess *session) error {
	return s.writeMe(w, r, sess)
}

func (s *Server) selectSite(w http.ResponseWriter, r *http.Request, sess *session) error {
	var in struct {
		SiteID string `json:"siteId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	id, err := uuid.Parse(in.SiteID)
	if err != nil {
		return &apiError{http.StatusForbidden, "forbidden", "このサイトは選べません"}
	}
	ok, err := s.DB.Queries().AuthSelectSite(r.Context(), dbgen.AuthSelectSiteParams{TokenHash: sess.tokenHash, TenantID: id})
	if err != nil {
		return fmt.Errorf("select site: %w", err)
	}
	if !ok {
		return &apiError{http.StatusForbidden, "forbidden", "このサイトは選べません"}
	}
	sess.tenantID = id
	return s.writeMe(w, r, sess)
}

func (s *Server) writeMe(w http.ResponseWriter, r *http.Request, sess *session) error {
	sites, err := s.DB.Queries().AuthSessionSites(r.Context(), sess.tokenHash)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	out := meJSON{
		User:  userJSON{ID: sess.userID.String(), Email: sess.email, DisplayName: sess.displayName},
		Sites: make([]siteJSON, 0, len(sites)),
	}
	for _, st := range sites {
		out.Sites = append(out.Sites, siteJSON{ID: st.TenantID.String(), Name: st.Name, Role: st.Role})
	}
	if sess.tenantID != uuid.Nil {
		id := sess.tenantID.String()
		out.CurrentSiteID = &id
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.SecureCookies, SameSite: http.SameSiteLaxMode,
	}
}
