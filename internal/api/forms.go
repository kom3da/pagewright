package api

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/jobs"
	pwmail "github.com/kom3da/pagewright/internal/mail"
	"github.com/kom3da/pagewright/internal/site"
)

const maxFormBytes = 64 << 10

// FormHandler receives contact form posts from the public sites at
// POST /f/{tenant}/contact. The sites are static and on other origins, so
// this is a plain HTML form post answered with an HTML page; it carries no
// session and changes nothing but adding a message.
func (s *Server) FormHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /f/{tenant}/contact", s.receiveContact)
	return mux
}

type contactInput struct {
	Name, Kana, Email, Phone, Message, Page string
}

var phonePattern = regexp.MustCompile(`^[0-9０-９+＋()（）\- ー－]{0,30}$`)

func (in *contactInput) check() []string {
	var errs []string
	lenOK := func(s string, max int) bool { return utf8.RuneCountInString(s) <= max }
	if in.Name == "" || !lenOK(in.Name, 100) || !plainText(in.Name, false) {
		errs = append(errs, "お名前を100文字以内で入力してください")
	}
	if !lenOK(in.Kana, 100) || !plainText(in.Kana, false) {
		errs = append(errs, "ふりがなは100文字以内で入力してください")
	}
	if !replyAddress(in.Email) {
		errs = append(errs, "メールアドレスを正しい形式で入力してください（例：taro@example.jp）")
	}
	if !phonePattern.MatchString(in.Phone) {
		errs = append(errs, "電話番号は数字とハイフンで入力してください")
	}
	if in.Message == "" || !lenOK(in.Message, 5000) || !plainText(in.Message, true) {
		errs = append(errs, "お問い合わせ内容を5000文字以内で入力してください")
	}
	return errs
}

// replyAddress accepts plain internet addresses only: no display names,
// no IP literals, and a dotted domain (so not root@localhost).
func replyAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 || !plainText(s, false) {
		return false
	}
	_, domain, _ := strings.Cut(s, "@")
	return strings.Contains(domain, ".") && !strings.ContainsAny(domain, "[]:") && !strings.HasSuffix(domain, ".")
}

func (s *Server) receiveContact(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant"))
	if err != nil {
		s.formPage(w, http.StatusNotFound, "", "ページが見つかりません", []string{"送信先が見つかりません。"}, "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		s.formPage(w, http.StatusBadRequest, "", "送信できませんでした", []string{"送信された内容が大きすぎるか、形式が正しくありません。"}, "")
		return
	}
	in := contactInput{
		Name:    strings.TrimSpace(r.PostFormValue("name")),
		Kana:    strings.TrimSpace(r.PostFormValue("kana")),
		Email:   strings.TrimSpace(r.PostFormValue("email")),
		Phone:   strings.TrimSpace(r.PostFormValue("phone")),
		Message: strings.TrimSpace(r.PostFormValue("message")),
		Page:    r.PostFormValue("page"),
	}
	if !strings.HasPrefix(in.Page, "/") || strings.HasPrefix(in.Page, "//") || len(in.Page) > 500 || !plainText(in.Page, false) {
		in.Page = "/"
	}
	// Checked before reading the site, so no back link yet: the browser's
	// back button still works.
	if !s.formByIP.allow(s.clientKey(r)) {
		s.formPage(w, http.StatusTooManyRequests, "", "送信できませんでした", []string{"短い時間に多くの送信がありました。しばらく待ってからお試しください。"}, "")
		return
	}

	ctx := db.WithTenant(r.Context(), tenantID)
	var siteName, back string
	var settings site.Settings
	if err := s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		row, err := q.GetSite(ctx)
		siteName, settings = row.Name, site.Parse(row.Settings)
		back = backLink(site.PublicURL(row.PublicBaseUrl, s.PublicBaseURL), in.Page)
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.formPage(w, http.StatusNotFound, "", "ページが見つかりません", []string{"送信先が見つかりません。"}, "")
			return
		}
		s.Logger.Error("contact form: load site", "tenant_id", tenantID, "err", err)
		s.formPage(w, http.StatusInternalServerError, "", "送信できませんでした", []string{"サーバーでエラーが起きました。時間をおいてもう一度お試しください。"}, "")
		return
	}
	// A filled honeypot is a bot: thank it and drop the message.
	if r.PostFormValue("website") != "" {
		s.formPage(w, http.StatusOK, siteName, "送信しました", []string{"お問い合わせを受け付けました。"}, back)
		return
	}
	errs := in.check()
	if r.PostFormValue("consent") != "yes" {
		errs = append(errs, "個人情報の取り扱いへの同意にチェックを入れてください")
	}
	if len(errs) > 0 {
		errs = append(errs, "ブラウザの「戻る」で入力画面に戻ると、入力した内容が残っています。")
		s.formPage(w, http.StatusUnprocessableEntity, siteName, "入力内容を確認してください", errs, back)
		return
	}
	// Count per site only messages that would be stored (not honeypot hits
	// or invalid posts, so junk cannot use up the site's budget), and only
	// for sites that exist, so made-up ids cannot fill the limiter's table.
	if !s.formBySite.allow(tenantID.String()) {
		s.formPage(w, http.StatusTooManyRequests, siteName, "送信できませんでした", []string{"短い時間に多くの送信がありました。しばらく待ってからお試しください。"}, back)
		return
	}

	err = s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		sub, err := q.CreateFormSubmission(ctx, dbgen.CreateFormSubmissionParams{
			Name: in.Name, Kana: in.Kana, Email: in.Email, Phone: in.Phone, Message: in.Message, PagePath: in.Page,
		})
		if err != nil {
			return err
		}
		recipients := []string{settings.ContactEmail}
		if settings.ContactEmail == "" {
			var err error
			if recipients, err = q.ListAdminEmails(ctx); err != nil {
				return err
			}
		}
		s.queueContactMails(ctx, tx, tenantID, siteName, recipients, in, sub)
		return nil
	})
	if err != nil {
		s.Logger.Error("contact form: save", "tenant_id", tenantID, "err", err)
		s.formPage(w, http.StatusInternalServerError, siteName, "送信できませんでした", []string{"サーバーでエラーが起きました。時間をおいてもう一度お試しください。"}, back)
		return
	}
	s.formPage(w, http.StatusOK, siteName, "送信しました",
		[]string{"お問い合わせを受け付けました。確認のメールをお送りしましたので、ご確認ください。", "担当者より折り返しご連絡いたします。"}, back)
}

// queueContactMails notifies the site and acknowledges the sender. The
// acknowledgement repeats nothing the sender typed, so the form cannot be
// used to mail arbitrary text to arbitrary people, and each address gets
// at most a few per day.
func (s *Server) queueContactMails(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, siteName string, recipients []string, in contactInput, sub dbgen.FormSubmission) {
	for _, to := range recipients {
		var err error
		if s.Jobs != nil {
			err = s.Jobs.EnqueueContactNotify(ctx, tx, jobs.ContactNotifyArgs{TenantID: tenantID, SubmissionID: sub.ID, To: to, SiteName: siteName})
		} else {
			err = s.Mail.Send(ctx, jobs.ContactNotification(to, siteName, sub))
		}
		if err != nil {
			s.Logger.Error("contact form: notify", "tenant_id", tenantID, "err", err)
		}
	}
	if !s.autoReplyTo.allow(tenantID.String() + " " + strings.ToLower(in.Email)) {
		return
	}
	s.sendMail(ctx, tx, pwmail.Message{
		To:      in.Email,
		Subject: "【" + siteName + "】お問い合わせを受け付けました",
		Body: siteName + " のお問い合わせフォームから送信された内容を受け付けました。\n" +
			"担当者より折り返しご連絡いたします。\n\n" +
			"※このメールは送信専用のアドレスからお送りしています。このメールに返信されても届きません。\n" +
			"※お心当たりのない場合は、どなたかが誤ってあなたのメールアドレスを入力した可能性があります。お手数ですがこのメールを破棄してください。\n",
	})
}

// backLink is the page the form was on, on the site's public origin base.
func backLink(base, page string) string {
	if base == "" {
		return ""
	}
	return base + page
}

var formPageTmpl = template.Must(template.New("form").Parse(`<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}}{{with .Site}} | {{.}}{{end}}</title>
<style>
body { font-family: "Hiragino Sans", "Noto Sans JP", "Yu Gothic UI", Meiryo, system-ui, sans-serif; line-height: 1.85; color: #1f2328; margin: 0; }
main { max-width: 40rem; margin: 3rem auto; padding: 0 1.25rem; }
h1 { font-size: 1.5rem; }
a { color: #0a4f8f; }
:focus-visible { outline: 3px solid #0a4f8f; outline-offset: 2px; }
</style>
</head>
<body>
<main>
<h1 tabindex="-1">{{.Title}}</h1>
{{range .Lines}}<p>{{.}}</p>
{{end}}{{with .Back}}<p><a href="{{.}}">{{$.BackLabel}}</a></p>{{end}}
</main>
</body>
</html>
`))

func (s *Server) formPage(w http.ResponseWriter, status int, siteName, title string, lines []string, back string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.WriteHeader(status)
	label := "サイトに戻る"
	if status == http.StatusUnprocessableEntity {
		label = "入力画面に戻る"
	}
	_ = formPageTmpl.Execute(w, map[string]any{"Site": siteName, "Title": title, "Lines": lines, "Back": back, "BackLabel": label})
}

type submissionJSON struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Kana      string     `json:"kana"`
	Email     string     `json:"email"`
	Phone     string     `json:"phone"`
	Message   string     `json:"message"`
	PagePath  string     `json:"pagePath"`
	CreatedAt time.Time  `json:"createdAt"`
	HandledAt *time.Time `json:"handledAt"`
	// HandledBy is the name of the member who marked it handled; null when
	// unhandled or when that member has left the site.
	HandledBy *string `json:"handledBy"`
}

func toSubmissionJSON(f dbgen.FormSubmission, handledBy string) submissionJSON {
	out := submissionJSON{ID: f.ID, Name: f.Name, Kana: f.Kana, Email: f.Email, Phone: f.Phone,
		Message: f.Message, PagePath: f.PagePath, CreatedAt: f.CreatedAt, HandledAt: f.HandledAt}
	if f.HandledAt != nil && handledBy != "" {
		out.HandledBy = &handledBy
	}
	return out
}

func requireAdmin(sess *session) error {
	if sess.role != "admin" {
		return &apiError{http.StatusForbidden, "forbidden", "お問い合わせの内容を見られるのは管理者だけです"}
	}
	return nil
}

func (s *Server) listSubmissions(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireAdmin(sess); err != nil {
		return err
	}
	var before *int64
	if v := r.URL.Query().Get("before"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return invalid("invalid_before", "ページの指定が正しくありません")
		}
		before = &id
	}
	var handled *bool
	switch v := r.URL.Query().Get("handled"); v {
	case "":
	case "true", "false":
		b := v == "true"
		handled = &b
	default:
		return invalid("invalid_handled", "対応の状態の指定が正しくありません")
	}
	out := []submissionJSON{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListFormSubmissions(r.Context(), dbgen.ListFormSubmissionsParams{Before: before, Handled: handled})
		for _, row := range rows {
			out = append(out, toSubmissionJSON(row.FormSubmission, row.HandledByName))
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) deleteSubmission(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireAdmin(sess); err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		n, err := q.DeleteFormSubmission(r.Context(), id)
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

// patchSubmission marks a message handled (対応済み) or unhandled (未対応).
func (s *Server) patchSubmission(w http.ResponseWriter, r *http.Request, sess *session) error {
	if err := requireAdmin(sess); err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Handled *bool `json:"handled"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.Handled == nil {
		return invalid("invalid_handled", "対応の状態の指定が正しくありません")
	}
	var out submissionJSON
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		n, err := q.SetFormSubmissionHandled(r.Context(), dbgen.SetFormSubmissionHandledParams{
			Handled: *in.Handled, UserID: sess.userID, ID: id,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return errNotFound
		}
		row, err := q.GetFormSubmissionWithHandler(r.Context(), id)
		out = toSubmissionJSON(row.FormSubmission, row.HandledByName)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
