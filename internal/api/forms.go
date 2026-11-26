package api

import (
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
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
	if in.Name == "" || !lenOK(in.Name, 100) {
		errs = append(errs, "お名前を100文字以内で入力してください")
	}
	if !lenOK(in.Kana, 100) {
		errs = append(errs, "ふりがなは100文字以内で入力してください")
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email || len(in.Email) > 254 {
		errs = append(errs, "メールアドレスを正しい形式で入力してください（例：taro@example.jp）")
	}
	if !phonePattern.MatchString(in.Phone) {
		errs = append(errs, "電話番号は数字とハイフンで入力してください")
	}
	if in.Message == "" || !lenOK(in.Message, 5000) {
		errs = append(errs, "お問い合わせ内容を5000文字以内で入力してください")
	}
	return errs
}

func (s *Server) receiveContact(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant"))
	if err != nil {
		s.formPage(w, http.StatusNotFound, "", "ページが見つかりません", []string{"送信先が見つかりません。"}, "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		s.formPage(w, http.StatusBadRequest, "", "送信できませんでした", []string{"送信された内容が大きすぎるか、形式が正しくありません。"}, backLink(r, ""))
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !s.formByIP.allow(ip) {
		s.formPage(w, http.StatusTooManyRequests, "", "送信できませんでした", []string{"短い時間に何度も送信されています。しばらく待ってからお試しください。"}, backLink(r, ""))
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
	if !strings.HasPrefix(in.Page, "/") || strings.HasPrefix(in.Page, "//") || len(in.Page) > 500 {
		in.Page = "/"
	}
	back := backLink(r, in.Page)

	ctx := db.WithTenant(r.Context(), tenantID)
	var siteName string
	var settings site.Settings
	if err := s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		row, err := q.GetSite(ctx)
		siteName, settings = row.Name, site.Parse(row.Settings)
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.formPage(w, http.StatusNotFound, "", "ページが見つかりません", []string{"送信先が見つかりません。"}, "")
			return
		}
		s.Logger.Error("contact form: load site", "tenant_id", tenantID, "err", err)
		s.formPage(w, http.StatusInternalServerError, "", "送信できませんでした", []string{"サーバーでエラーが起きました。時間をおいてもう一度お試しください。"}, back)
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

	var recipients []string
	err = s.DB.InTenant(ctx, func(q *dbgen.Queries) error {
		if _, err := q.CreateFormSubmission(ctx, dbgen.CreateFormSubmissionParams{
			Name: in.Name, Kana: in.Kana, Email: in.Email, Phone: in.Phone, Message: in.Message, PagePath: in.Page,
		}); err != nil {
			return err
		}
		if settings.ContactEmail != "" {
			recipients = []string{settings.ContactEmail}
			return nil
		}
		recipients, err = q.ListAdminEmails(ctx)
		return err
	})
	if err != nil {
		s.Logger.Error("contact form: save", "tenant_id", tenantID, "err", err)
		s.formPage(w, http.StatusInternalServerError, siteName, "送信できませんでした", []string{"サーバーでエラーが起きました。時間をおいてもう一度お試しください。"}, back)
		return
	}
	s.sendContactMails(r, siteName, recipients, in)
	s.formPage(w, http.StatusOK, siteName, "送信しました",
		[]string{"お問い合わせを受け付けました。確認のメールをお送りしましたので、ご確認ください。", "担当者より折り返しご連絡いたします。"}, back)
}

func (s *Server) sendContactMails(r *http.Request, siteName string, recipients []string, in contactInput) {
	summary := fmt.Sprintf("お名前：%s\nふりがな：%s\nメールアドレス：%s\n電話番号：%s\n\nお問い合わせ内容：\n%s\n",
		in.Name, in.Kana, in.Email, in.Phone, in.Message)
	for _, to := range recipients {
		err := s.Mail.Send(r.Context(), pwmail.Message{
			To:      to,
			Subject: "【" + siteName + "】お問い合わせがありました",
			Body: "サイトのお問い合わせフォームから、次の内容が届きました。\n" +
				"返信するときは、下のメールアドレスあてに送ってください。\n\n" + summary +
				"\n送信元のページ：" + in.Page + "\n受付日時：" + time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006年1月2日 15:04") + "\n",
		})
		if err != nil {
			s.Logger.Error("contact form: notify", "err", err)
		}
	}
	err := s.Mail.Send(r.Context(), pwmail.Message{
		To:      in.Email,
		Subject: "【" + siteName + "】お問い合わせを受け付けました",
		Body: in.Name + " 様\n\n" + siteName + " へのお問い合わせありがとうございます。\n" +
			"次の内容で受け付けました。担当者より折り返しご連絡いたします。\n\n" + summary +
			"\n※このメールは送信専用のアドレスからお送りしています。このメールに返信されても届きません。\n" +
			"※お心当たりのない場合は、お手数ですがこのメールを破棄してください。\n",
	})
	if err != nil {
		s.Logger.Error("contact form: auto reply", "err", err)
	}
}

// backLink is where the result page sends people back to: the page the
// form was on, from the Referer when it is an http(s) URL.
func backLink(r *http.Request, page string) string {
	ref, err := url.Parse(r.Referer())
	if err != nil || (ref.Scheme != "http" && ref.Scheme != "https") || ref.Host == "" {
		return ""
	}
	if page == "" {
		return ref.String()
	}
	return ref.Scheme + "://" + ref.Host + page
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
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Kana      string    `json:"kana"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Message   string    `json:"message"`
	PagePath  string    `json:"pagePath"`
	CreatedAt time.Time `json:"createdAt"`
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
	out := []submissionJSON{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListFormSubmissions(r.Context())
		for _, f := range rows {
			out = append(out, submissionJSON{ID: f.ID, Name: f.Name, Kana: f.Kana, Email: f.Email, Phone: f.Phone,
				Message: f.Message, PagePath: f.PagePath, CreatedAt: f.CreatedAt})
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
