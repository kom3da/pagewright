package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kom3da/pagewright/internal/db/dbgen"
	pwmail "github.com/kom3da/pagewright/internal/mail"
)

// invitationTTL is how long an invitation link works.
const invitationTTL = 7 * 24 * time.Hour

const maxDisplayName = 50

var roleLabels = map[string]string{"admin": "管理者", "editor": "編集者"}

type memberJSON struct {
	UserID      string    `json:"userId"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	AddedAt     time.Time `json:"addedAt"`
	You         bool      `json:"you"`
}

type invitationJSON struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
	// InvitedBy names who sent it last; null once they have left the site.
	InvitedBy *string   `json:"invitedBy"`
	SentAt    time.Time `json:"sentAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Expired   bool      `json:"expired"`
}

func requireMemberAdmin(sess *session) error {
	if sess.role != "admin" {
		return &apiError{http.StatusForbidden, "forbidden", "メンバーの招待や変更ができるのは管理者だけです"}
	}
	return nil
}

// isLastAdmin reports whether err is memberships_keep_admin refusing to
// leave the site without an admin (migrations/00017_invitations.sql).
func isLastAdmin(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23514" && pe.ConstraintName == "memberships_keep_admin"
}

func memberUserID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		return uuid.Nil, errNotFound
	}
	return id, nil
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	var rows []dbgen.ListMembersRow
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		var err error
		rows, err = q.ListMembers(r.Context())
		return err
	})
	if err != nil {
		return err
	}
	out := make([]memberJSON, 0, len(rows))
	for _, m := range rows {
		out = append(out, memberJSON{UserID: m.ID.String(), Email: m.Email, DisplayName: m.DisplayName,
			Role: m.Role, AddedAt: m.CreatedAt, You: m.ID == sess.userID})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) patchMember(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	id, err := memberUserID(r)
	if err != nil {
		return err
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if _, ok := roleLabels[in.Role]; !ok {
		return invalid("invalid_role", "役割は「管理者」か「編集者」から選んでください")
	}
	ctx := r.Context()
	var m dbgen.GetMemberRow
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if err := q.LockMembers(ctx); err != nil {
			return err
		}
		n, err := q.SetMemberRole(ctx, dbgen.SetMemberRoleParams{Role: in.Role, UserID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return errNotFound
		}
		m, err = q.GetMember(ctx, id)
		return err
	})
	if isLastAdmin(err) {
		return &apiError{http.StatusConflict, "last_admin", "管理者がいなくなるため、役割を変えられません。先にほかのメンバーを管理者にしてください"}
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, memberJSON{UserID: m.ID.String(), Email: m.Email, DisplayName: m.DisplayName,
		Role: m.Role, AddedAt: m.CreatedAt, You: m.ID == sess.userID})
	return nil
}

// removeMember takes the member off the site. What they made stays; their
// sessions lose the site (sessions_tenant_id_user_id_fkey).
func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	id, err := memberUserID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if err := q.LockMembers(ctx); err != nil {
			return err
		}
		n, err := q.RemoveMember(ctx, id)
		if err == nil && n == 0 {
			return errNotFound
		}
		return err
	})
	if isLastAdmin(err) {
		return &apiError{http.StatusConflict, "last_admin", "このサイトの管理者はこの人だけのため、外せません。先にほかのメンバーを管理者にしてください"}
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func toInvitationJSON(i dbgen.Invitation, inviter string) invitationJSON {
	out := invitationJSON{ID: i.ID, Email: i.Email, Role: i.Role, SentAt: i.SentAt,
		ExpiresAt: i.ExpiresAt, Expired: !i.ExpiresAt.After(time.Now())}
	if inviter != "" {
		out.InvitedBy = &inviter
	}
	return out
}

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	var rows []dbgen.ListInvitationsRow
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		var err error
		rows, err = q.ListInvitations(r.Context())
		return err
	})
	if err != nil {
		return err
	}
	out := make([]invitationJSON, 0, len(rows))
	for _, i := range rows {
		out = append(out, toInvitationJSON(dbgen.Invitation{ID: i.ID, Email: i.Email, Role: i.Role,
			ExpiresAt: i.ExpiresAt, SentAt: i.SentAt}, i.InviterName))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// allowInvitation applies the sending limits: per site, and per address
// across sites (so that sites cannot together flood one mailbox).
func (s *Server) allowInvitation(sess *session, email string) error {
	if !s.inviteBySite.allow(sess.tenantID.String()) {
		return &apiError{http.StatusTooManyRequests, "too_many_requests", "招待のメールを送りすぎています。明日になってから、もう一度お試しください"}
	}
	if !s.inviteTo.allow(email) {
		return &apiError{http.StatusTooManyRequests, "too_many_requests", "このメールアドレスには、今日はもう招待のメールを送れません。明日になってから、もう一度お試しください"}
	}
	return nil
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	email := strings.TrimSpace(in.Email)
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		return invalid("invalid_email", "メールアドレスの形式が正しくありません。例：taro@example.jp")
	}
	email = strings.ToLower(email)
	if _, ok := roleLabels[in.Role]; !ok {
		return invalid("invalid_role", "役割は「管理者」か「編集者」から選んでください")
	}
	return s.sendInvitation(w, r, sess, http.StatusCreated, func(q *dbgen.Queries, token []byte, expires time.Time) (dbgen.Invitation, error) {
		member, err := q.IsMemberEmail(r.Context(), email)
		if err != nil {
			return dbgen.Invitation{}, err
		}
		if member {
			return dbgen.Invitation{}, &apiError{http.StatusConflict, "already_member", "このメールアドレスの人は、すでにこのサイトのメンバーです"}
		}
		if err := s.allowInvitation(sess, email); err != nil {
			return dbgen.Invitation{}, err
		}
		return q.UpsertInvitation(r.Context(), dbgen.UpsertInvitationParams{
			Email: email, Role: in.Role, TokenHash: token, ExpiresAt: expires, InvitedBy: &sess.userID,
		})
	})
}

// resendInvitation mails a new link (the old one stops working) with a
// new expiry, also for an expired invitation.
func (s *Server) resendInvitation(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	return s.sendInvitation(w, r, sess, http.StatusOK, func(q *dbgen.Queries, token []byte, expires time.Time) (dbgen.Invitation, error) {
		inv, err := q.GetInvitation(r.Context(), id)
		if err != nil {
			return inv, err
		}
		if err := s.allowInvitation(sess, inv.Email); err != nil {
			return inv, err
		}
		return q.RenewInvitation(r.Context(), dbgen.RenewInvitationParams{
			ID: id, TokenHash: token, ExpiresAt: expires, InvitedBy: &sess.userID,
		})
	})
}

// sendInvitation stores an invitation with a new token (through save) and
// mails its link, in one transaction: an invitation nobody receives is of
// no use.
func (s *Server) sendInvitation(w http.ResponseWriter, r *http.Request, sess *session, status int,
	save func(q *dbgen.Queries, tokenHash []byte, expires time.Time) (dbgen.Invitation, error)) error {
	ctx := r.Context()
	token := newToken()
	expires := time.Now().Add(invitationTTL).Truncate(time.Second)
	var inv dbgen.Invitation
	var msg pwmail.Message
	err := s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		var err error
		if inv, err = save(q, hashToken(token), expires); err != nil {
			return err
		}
		site, err := q.GetCurrentTenant(ctx)
		if err != nil {
			return err
		}
		msg = s.invitationMail(sess, site.Name, inv, token)
		if s.Jobs == nil {
			return nil
		}
		return s.Jobs.EnqueueMail(ctx, tx, msg)
	})
	if err != nil {
		return err
	}
	if s.Jobs == nil {
		// No queue (tests and tools only): send directly after commit.
		s.sendMail(ctx, nil, msg)
	}
	writeJSON(w, status, toInvitationJSON(inv, inviterName(sess)))
	return nil
}

func inviterName(sess *session) string {
	if sess.displayName != "" {
		return sess.displayName
	}
	return sess.email
}

func (s *Server) invitationMail(sess *session, siteName string, inv dbgen.Invitation, token string) pwmail.Message {
	link := s.BaseURL + "/admin/invite/accept?token=" + url.QueryEscape(token)
	inviter := sess.email + " さん"
	if sess.displayName != "" {
		inviter = sess.displayName + "（" + sess.email + "）さん"
	}
	return pwmail.Message{
		To:      inv.Email,
		Subject: "「" + siteName + "」の管理画面に招待されました",
		Body: inviter + "が、あなた（" + inv.Email + "）を、サイト「" + siteName + "」の管理画面に" +
			roleLabels[inv.Role] + "として招待しました。\n\n" +
			"参加するには、次のリンクを開いて「参加する」ボタンを押してください。" +
			"Pagewright を初めて使う方は、そのときにアカウントが作られます。パスワードは要りません。\n\n" +
			link + "\n\n" +
			"このリンクの有効期限は " + formatJapanDateTime(inv.ExpiresAt) + " までで、1回だけ使えます。" +
			"期限が過ぎたときは、招待した人にもう一度送ってもらってください。\n" +
			"心当たりがない場合は、このメールを無視してください。リンクを開かなければ、何も起きません。\n",
	}
}

// formatJapanDateTime writes t in Japan time, as the admin does
// ("2026年12月1日 9:00").
func formatJapanDateTime(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		loc = time.FixedZone("JST", 9*60*60)
	}
	return t.In(loc).Format("2006年1月2日 15:04")
}

func (s *Server) deleteInvitation(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireMemberAdmin(sess); err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		n, err := q.DeleteInvitation(r.Context(), id)
		if err == nil && n == 0 {
			return errNotFound
		}
		return err
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

var errInvalidInvitation = &apiError{http.StatusBadRequest, "invalid_token",
	"この招待のリンクは使えません。すでに参加したか、招待が取り消されたか、新しい招待のメールが送られています。参加済みなら、ログインしてください"}

var errExpiredInvitation = &apiError{http.StatusBadRequest, "invitation_expired",
	"招待の有効期限が切れています。招待した人に、もう一度送ってもらってください"}

// lookupInvitation tells the invitation page what the invitation is for,
// without using it (mail scanners open links too).
func (s *Server) lookupInvitation(w http.ResponseWriter, r *http.Request, _ *session) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	inv, err := s.DB.Queries().InvitationLookup(r.Context(), hashToken(in.Token))
	if errors.Is(err, pgx.ErrNoRows) {
		return errInvalidInvitation
	}
	if err != nil {
		return fmt.Errorf("look up invitation: %w", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"siteName": inv.SiteName, "email": inv.Email, "role": inv.Role,
		"expiresAt": inv.ExpiresAt, "expired": !inv.ExpiresAt.After(time.Now()), "hasAccount": inv.HasAccount,
	})
	return nil
}

// acceptInvitation joins the site. Logged in as the invited address, the
// current session moves to the site; logged out, the link logs the person
// in (creating their user when new).
func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request, _ *session) error {
	var in struct {
		Token       string `json:"token"`
		DisplayName string `json:"displayName"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	name, err := checkDisplayName(in.DisplayName)
	if err != nil {
		return err
	}
	var current []byte
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		current = hashToken(c.Value)
	}
	ctx := r.Context()
	sessionToken := newToken()
	result, err := s.DB.Queries().AuthAcceptInvitation(ctx, dbgen.AuthAcceptInvitationParams{
		TokenHash: hashToken(in.Token), SessionHash: current, NewSessionHash: hashToken(sessionToken),
		DisplayName: name, TtlSeconds: int64(sessionTTL.Seconds()),
	})
	if err != nil {
		return fmt.Errorf("accept invitation: %w", err)
	}
	switch result {
	case "invalid":
		return errInvalidInvitation
	case "expired":
		return errExpiredInvitation
	case "wrong_account":
		sess, err := s.sessionByHash(ctx, current)
		if err != nil {
			return err
		}
		return &apiError{http.StatusConflict, "wrong_account", "いまは別のメールアドレス（" + sess.email +
			"）でログインしています。招待されたメールアドレスで参加するには、いったんログアウトしてから、もう一度このリンクを開いてください"}
	case "joined":
		http.SetCookie(w, s.cookie(sessionToken, int(sessionTTL.Seconds())))
		current = hashToken(sessionToken)
	case "joined_current":
	default:
		return fmt.Errorf("accept invitation: unexpected result %q", result)
	}
	sess, err := s.sessionByHash(ctx, current)
	if err != nil {
		return err
	}
	s.recordUse(r, sess)
	return s.writeMe(w, r, sess)
}

func checkDisplayName(s string) (string, error) {
	name := strings.TrimSpace(s)
	if len([]rune(name)) > maxDisplayName || !plainText(name, false) {
		return "", invalid("invalid_display_name", fmt.Sprintf("名前は%d文字以内で入力してください", maxDisplayName))
	}
	return name, nil
}

// patchMe changes the user's own display name. Users are shared by all
// their sites, so it shows on every one of them.
func (s *Server) patchMe(w http.ResponseWriter, r *http.Request, sess *session) error {
	var in struct {
		DisplayName string `json:"displayName"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	name, err := checkDisplayName(in.DisplayName)
	if err != nil {
		return err
	}
	ok, err := s.DB.Queries().AuthSetDisplayName(r.Context(), dbgen.AuthSetDisplayNameParams{TokenHash: sess.tokenHash, DisplayName: name})
	if err != nil {
		return fmt.Errorf("set display name: %w", err)
	}
	if !ok {
		return errUnauthenticated
	}
	sess.displayName = name
	return s.writeMe(w, r, sess)
}
