package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kom3da/pagewright/internal/api"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/db/dbtest"
	"github.com/kom3da/pagewright/internal/jobs"
	"github.com/kom3da/pagewright/internal/mail"
	"github.com/kom3da/pagewright/internal/media"
)

type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Message
	// pending are mails queued in a transaction, delivered once it has
	// committed (dropped when it rolled back), as a River job would be.
	pending []pendingMail
	admin   *pgxpool.Pool
}

type pendingMail struct {
	xid string
	m   mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

// queue holds m until the transaction tx is over.
func (f *fakeMail) queue(ctx context.Context, tx pgx.Tx, m mail.Message) error {
	var xid string
	if err := tx.QueryRow(ctx, "SELECT pg_current_xact_id()::text").Scan(&xid); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, pendingMail{xid, m})
	return nil
}

// all returns the mails sent so far, delivering queued ones whose
// transaction committed.
func (f *fakeMail) all() []mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	var still []pendingMail
	for _, p := range f.pending {
		var status *string
		if err := f.admin.QueryRow(context.Background(), "SELECT pg_xact_status($1::text::xid8)", p.xid).Scan(&status); err != nil {
			panic(err)
		}
		switch {
		case status != nil && *status == "committed":
			f.sent = append(f.sent, p.m)
		case status != nil && *status == "in progress":
			still = append(still, p)
		}
	}
	f.pending = still
	return append([]mail.Message(nil), f.sent...)
}

func (f *fakeMail) last() (mail.Message, bool) {
	sent := f.all()
	if len(sent) == 0 {
		return mail.Message{}, false
	}
	return sent[len(sent)-1], true
}

type fakeBuilds struct {
	mu        sync.Mutex
	tenants   []uuid.UUID
	publishes []scheduledPublish
	prunes    []uuid.UUID
	mailJobs  int // EnqueueMail calls, committed or not
	mail      *fakeMail
}

func (f *fakeBuilds) EnqueueBuild(_ context.Context, _ pgx.Tx, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tenants = append(f.tenants, id)
	return nil
}

func (f *fakeBuilds) EnqueuePublish(_ context.Context, _ pgx.Tx, _ uuid.UUID, contentID, revisionID int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishes = append(f.publishes, scheduledPublish{contentID, revisionID, at})
	return nil
}

func (f *fakeBuilds) EnqueuePruneRevisions(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prunes = append(f.prunes, id)
	return nil
}

type scheduledPublish struct {
	contentID, revisionID int64
	at                    time.Time
}

// EnqueueContactNotify delivers right away, reading the submission in
// tx as the job would after commit.
func (f *fakeBuilds) EnqueueContactNotify(ctx context.Context, tx pgx.Tx, args jobs.ContactNotifyArgs) error {
	sub, err := dbgen.New(tx).GetFormSubmission(ctx, args.SubmissionID)
	if err != nil {
		return err
	}
	return f.mail.Send(ctx, jobs.ContactNotification(args.To, args.SiteName, sub))
}

// EnqueueMail delivers through the env's fake mail: right away without
// tx, else when tx commits.
func (f *fakeBuilds) EnqueueMail(ctx context.Context, tx pgx.Tx, m mail.Message) error {
	f.mu.Lock()
	f.mailJobs++
	f.mu.Unlock()
	if tx == nil {
		return f.mail.Send(ctx, m)
	}
	return f.mail.queue(ctx, tx, m)
}

func (f *fakeBuilds) mailJobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mailJobs
}

func (f *fakeBuilds) count(id uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.tenants {
		if t == id {
			n++
		}
	}
	return n
}

type env struct {
	t       *testing.T
	db      *dbtest.Env
	srv     *httptest.Server
	mail    *fakeMail
	builds  *fakeBuilds
	tenantA uuid.UUID
	tenantB uuid.UUID
}

// newEnv starts the API on a fresh database with two sites:
// alice (A admin), dave (A editor), bob (B admin), carol (editor of both).
// Site B's ids start after 1000, so an id of B is never also an id of A
// and "A asks for B's item" asks for something A does not have.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := newEnvOverlappingIDs(t)
	dbtest.SkipIDs(t, e.db.Admin, e.tenantB, 1000)
	return e
}

// newEnvOverlappingIDs is newEnv where both sites number from 1, as two
// real sites do.
func newEnvOverlappingIDs(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, nil)
}

// newEnvWith is newEnvOverlappingIDs with options changed by set.
func newEnvWith(t *testing.T, set func(*api.Options)) *env {
	t.Helper()
	d := dbtest.New(t)
	fm := &fakeMail{admin: d.Admin}
	e := &env{t: t, db: d, mail: fm, builds: &fakeBuilds{mail: fm}, tenantA: uuid.New(), tenantB: uuid.New()}
	ctx := context.Background()
	seed := []string{
		fmt.Sprintf("INSERT INTO tenants (id, name) VALUES ('%s', 'サイトA'), ('%s', 'サイトB')", e.tenantA, e.tenantB),
		"INSERT INTO users (email) VALUES ('alice@example.com'), ('bob@example.com'), ('carol@example.com'), ('dave@example.com')",
		fmt.Sprintf(`INSERT INTO memberships (tenant_id, user_id, role)
			SELECT '%s'::uuid, id, 'admin' FROM users WHERE email = 'alice@example.com'
			UNION ALL SELECT '%s'::uuid, id, 'editor' FROM users WHERE email = 'dave@example.com'
			UNION ALL SELECT '%s'::uuid, id, 'admin' FROM users WHERE email = 'bob@example.com'
			UNION ALL SELECT '%s'::uuid, id, 'editor' FROM users WHERE email = 'carol@example.com'
			UNION ALL SELECT '%s'::uuid, id, 'editor' FROM users WHERE email = 'carol@example.com'`,
			e.tenantA, e.tenantA, e.tenantB, e.tenantA, e.tenantB),
	}
	for _, q := range seed {
		if _, err := d.Admin.Exec(ctx, q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	opts := api.Options{
		DB: d.DB, Mail: e.mail, Media: media.Store{Root: t.TempDir()}, Jobs: e.builds,
		BaseURL: "http://admin.test", PublicBaseURL: "https://site-a.example",
	}
	if set != nil {
		set(&opts)
	}
	s := api.New(opts)
	mux := http.NewServeMux()
	mux.Handle("/api/", s.Handler())
	mux.Handle("/f/", s.FormHandler())
	mux.Handle("/p/", s.PreviewHandler())
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	return e
}

// client is one browser.
type client struct {
	e    *env
	http *http.Client
	// userAgent, when set, is sent as the User-Agent.
	userAgent string
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar}}
}

var tokenRE = regexp.MustCompile(`token=([A-Za-z0-9_%-]+)`)

// login signs in through the mailed link.
func (e *env) login(email string) *client {
	e.t.Helper()
	return e.loginFrom(email, "")
}

// loginFrom is login from a browser with this User-Agent.
func (e *env) loginFrom(email, userAgent string) *client {
	e.t.Helper()
	c := e.anon()
	c.userAgent = userAgent
	c.mustDo(http.MethodPost, "/api/auth/login-link", map[string]string{"email": email}, nil, http.StatusNoContent)
	m, ok := e.mail.last()
	if !ok || m.To != email {
		e.t.Fatalf("no login mail to %s", email)
	}
	match := tokenRE.FindStringSubmatch(m.Body)
	if match == nil {
		e.t.Fatalf("no token in mail: %s", m.Body)
	}
	token, _ := url.QueryUnescape(match[1])
	c.mustDo(http.MethodPost, "/api/auth/verify", map[string]string{"token": token}, nil, http.StatusOK)
	return c
}

// do sends a JSON request with the CSRF header and decodes the response.
func (c *client) do(method, path string, in any, out any) int {
	c.e.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pagewright", "1")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	return c.send(req, out)
}

func (c *client) send(req *http.Request, out any) int {
	c.e.t.Helper()
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			c.e.t.Fatalf("%s %s: decode %q: %v", req.Method, req.URL.Path, b, err)
		}
	}
	return res.StatusCode
}

func (c *client) mustDo(method, path string, in, out any, want int) {
	c.e.t.Helper()
	var raw json.RawMessage
	if got := c.do(method, path, in, &raw); got != want {
		c.e.t.Fatalf("%s %s = %d, want %d: %s", method, path, got, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.e.t.Fatal(err)
		}
	}
}

// errorCode returns the status and error code of a request.
func (c *client) errorCode(method, path string, in any) (int, string) {
	c.e.t.Helper()
	var out struct {
		Error struct{ Code string } `json:"error"`
	}
	status := c.do(method, path, in, &out)
	return status, out.Error.Code
}

type me struct {
	User          struct{ Email string }
	Sites         []struct{ ID, Name, Role string }
	CurrentSiteID *string
}

type detail struct {
	ID                    int64
	Number                int64
	Type                  string
	Title                 string
	Slug                  *string
	Status                string
	HasUnpublishedChanges bool
	URL                   *string
	ScheduledAt           *time.Time
	PublishedAt           *time.Time
	ParentID              *int64
	CategoryIDs           []int64
	TagIDs                []int64
	PendingSettings       bool
	Revision              struct {
		ID    int64
		Title string
		Body  json.RawMessage
	}
}

const paragraph = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"こんにちは"}]}]}`

// newPost creates, fills and optionally publishes a content.
func (c *client) newContent(typ, title string, publish bool) detail {
	c.e.t.Helper()
	var d detail
	c.mustDo(http.MethodPost, "/api/contents", map[string]string{"type": typ}, &d, http.StatusCreated)
	c.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID),
		map[string]any{"title": title, "body": json.RawMessage(paragraph)}, &d, http.StatusOK)
	if publish {
		c.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), nil, &d, http.StatusOK)
	}
	return d
}

func TestLogin(t *testing.T) {
	e := newEnv(t)
	anon := e.anon()

	if got, code := anon.errorCode(http.MethodGet, "/api/me", nil); got != http.StatusUnauthorized || code != "unauthenticated" {
		t.Errorf("GET /api/me without login = %d %s, want 401 unauthenticated", got, code)
	}

	// Without the CSRF header the request is refused.
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/auth/login-link", strings.NewReader(`{"email":"alice@example.com"}`))
	if got := anon.send(req, nil); got != http.StatusForbidden {
		t.Errorf("login-link without X-Pagewright = %d, want 403", got)
	}

	// Unknown addresses get the same answer and no mail, though they go
	// through the same steps (a mail job, rolled back) as known ones.
	anon.mustDo(http.MethodPost, "/api/auth/login-link", map[string]string{"email": "nobody@example.com"}, nil, http.StatusNoContent)
	if _, sent := e.mail.last(); sent {
		t.Error("mail sent to an unknown address")
	}
	if got := e.builds.mailJobCount(); got != 1 {
		t.Errorf("mail jobs for an unknown address = %d, want 1 (rolled back)", got)
	}
	var tokens int
	if err := e.db.Admin.QueryRow(context.Background(), "SELECT count(*) FROM login_tokens").Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 {
		t.Errorf("login tokens after an unknown address = %d, want 0", tokens)
	}
	if got, code := anon.errorCode(http.MethodPost, "/api/auth/verify", map[string]string{"token": "bogus"}); got != http.StatusBadRequest || code != "invalid_token" {
		t.Errorf("verify bogus token = %d %s, want 400 invalid_token", got, code)
	}

	alice := e.login("alice@example.com")
	var m me
	alice.mustDo(http.MethodGet, "/api/me", nil, &m, http.StatusOK)
	if m.CurrentSiteID == nil || *m.CurrentSiteID != e.tenantA.String() || len(m.Sites) != 1 {
		t.Errorf("alice me = %+v, want site A selected", m)
	}
	if got, _ := alice.errorCode(http.MethodPost, "/api/me/site", map[string]string{"siteId": e.tenantB.String()}); got != http.StatusForbidden {
		t.Errorf("alice selecting site B = %d, want 403", got)
	}

	carol := e.login("carol@example.com")
	carol.mustDo(http.MethodGet, "/api/me", nil, &m, http.StatusOK)
	if m.CurrentSiteID != nil || len(m.Sites) != 2 {
		t.Errorf("carol me = %+v, want two sites and none selected", m)
	}
	if got, code := carol.errorCode(http.MethodGet, "/api/contents?type=post", nil); got != http.StatusForbidden || code != "no_site" {
		t.Errorf("contents before choosing a site = %d %s, want 403 no_site", got, code)
	}
	carol.mustDo(http.MethodPost, "/api/me/site", map[string]string{"siteId": e.tenantB.String()}, &m, http.StatusOK)
	if m.CurrentSiteID == nil || *m.CurrentSiteID != e.tenantB.String() {
		t.Errorf("carol after choosing B = %+v", m)
	}

	alice.mustDo(http.MethodPost, "/api/auth/logout", nil, nil, http.StatusNoContent)
	if got, _ := alice.errorCode(http.MethodGet, "/api/me", nil); got != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", got)
	}
}

func TestContentLifecycle(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")

	d := alice.newContent("post", "お知らせ", false)
	if d.Status != "draft" || d.URL != nil || d.Revision.Title != "お知らせ" {
		t.Fatalf("draft = %+v", d)
	}
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), nil, &d, http.StatusOK)
	wantURL := fmt.Sprintf("/posts/%d/", d.Number)
	if d.Status != "published" || d.URL == nil || *d.URL != wantURL || d.HasUnpublishedChanges {
		t.Fatalf("published = %+v, want url %s", d, wantURL)
	}
	if e.builds.count(e.tenantA) == 0 {
		t.Error("publishing did not schedule a build")
	}
	e.builds.mu.Lock()
	prunes := slices.Clone(e.builds.prunes)
	e.builds.mu.Unlock()
	if len(prunes) == 0 || prunes[0] != e.tenantA {
		t.Errorf("saving a draft scheduled pruning for %v, want tenant A", prunes)
	}

	// A new draft leaves the published version and shows pending changes.
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID),
		map[string]any{"title": "お知らせ（改）", "body": json.RawMessage(paragraph)}, &d, http.StatusOK)
	if !d.HasUnpublishedChanges || d.Status != "published" {
		t.Errorf("after new draft = %+v, want published with unpublished changes", d)
	}

	// Changing the URL keeps the old one as a redirect, once published.
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", d.ID), map[string]any{"slug": "news-1"}, &d, http.StatusOK)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), nil, &d, http.StatusOK)
	if d.URL == nil || *d.URL != "/posts/news-1/" {
		t.Errorf("url after slug change = %v, want /posts/news-1/", d.URL)
	}
	var kind string
	if err := e.db.Admin.QueryRow(context.Background(), "SELECT kind FROM routes WHERE tenant_id = $1 AND path = $2", e.tenantA, wantURL).Scan(&kind); err != nil || kind != "redirect" {
		t.Errorf("old url %s: kind=%q err=%v, want redirect", wantURL, kind, err)
	}

	// Pages nest under their parent's URL and follow its changes.
	parent := alice.newContent("page", "会社概要", true)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", parent.ID), map[string]any{"slug": "company"}, &parent, http.StatusOK)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", parent.ID), nil, &parent, http.StatusOK)
	child := alice.newContent("page", "アクセス", false)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", child.ID), map[string]any{"slug": "access", "parentId": parent.ID}, &child, http.StatusOK)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", child.ID), nil, &child, http.StatusOK)
	if child.URL == nil || *child.URL != "/company/access/" {
		t.Errorf("child url = %v, want /company/access/", child.URL)
	}
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", parent.ID), map[string]any{"slug": "about"}, nil, http.StatusOK)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", parent.ID), nil, nil, http.StatusOK)
	alice.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", child.ID), nil, &child, http.StatusOK)
	if child.URL == nil || *child.URL != "/about/access/" {
		t.Errorf("child url after parent change = %v, want /about/access/", child.URL)
	}
	if got, _ := alice.errorCode(http.MethodPatch, fmt.Sprintf("/api/contents/%d", parent.ID), map[string]any{"parentId": child.ID}); got != http.StatusUnprocessableEntity {
		t.Errorf("making a page its own grandparent = %d, want 422", got)
	}

	// Another content cannot take a URL in use.
	other := alice.newContent("post", "別の記事", true)
	if got, code := alice.errorCode(http.MethodPatch, fmt.Sprintf("/api/contents/%d", other.ID), map[string]any{"slug": "news-1"}); got != http.StatusConflict {
		t.Errorf("taking a used URL = %d %s, want 409", got, code)
	}

	var list []detail
	alice.mustDo(http.MethodGet, "/api/contents?type=post", nil, &list, http.StatusOK)
	if len(list) != 2 || list[0].ID != other.ID {
		t.Errorf("post list = %+v, want 2 posts, newest first", list)
	}

	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/unpublish", d.ID), nil, &d, http.StatusOK)
	if d.Status != "draft" {
		t.Errorf("after unpublish status = %s", d.Status)
	}
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/contents/%d", d.ID), nil, nil, http.StatusNoContent)
	if got, _ := alice.errorCode(http.MethodGet, fmt.Sprintf("/api/contents/%d", d.ID), nil); got != http.StatusNotFound {
		t.Errorf("deleted content = %d, want 404", got)
	}
}

// TestUnpublishDraft makes sure taking down a never-published content
// does not give it a publication date, which would date its first
// publication in the past.
func TestUnpublishDraft(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	d := alice.newContent("post", "下書き", false)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/unpublish", d.ID), nil, &d, http.StatusOK)
	if d.PublishedAt != nil {
		t.Errorf("publishedAt after unpublishing a draft = %v, want none", d.PublishedAt)
	}
}

// TestMoveUnderDraftPage moves a page whose child is a draft: the
// published grandchild must follow to the new URL.
func TestMoveUnderDraftPage(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	patch := func(id int64, in map[string]any) detail {
		var d detail
		alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", id), in, &d, http.StatusOK)
		return d
	}
	publish := func(id int64) detail {
		var d detail
		alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", id), nil, &d, http.StatusOK)
		return d
	}
	a := alice.newContent("page", "A", false)
	patch(a.ID, map[string]any{"slug": "a"})
	publish(a.ID)
	b := alice.newContent("page", "B", false)
	patch(b.ID, map[string]any{"slug": "b", "parentId": a.ID})
	c := alice.newContent("page", "C", false)
	patch(c.ID, map[string]any{"slug": "c", "parentId": b.ID})
	c = publish(c.ID)
	if c.URL == nil || *c.URL != "/a/b/c/" {
		t.Fatalf("C's URL = %v, want /a/b/c/", c.URL)
	}
	patch(a.ID, map[string]any{"slug": "a2"})
	publish(a.ID)
	alice.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", c.ID), nil, &c, http.StatusOK)
	if c.URL == nil || *c.URL != "/a2/b/c/" {
		t.Errorf("C's URL after moving A = %v, want /a2/b/c/", c.URL)
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	dave := e.login("dave@example.com")
	d := alice.newContent("post", "x", false)
	path := fmt.Sprintf("/api/contents/%d", d.ID)
	page := alice.newContent("page", "p", false)

	cases := []struct {
		name     string
		c        *client
		method   string
		path     string
		in       any
		want     int
		wantCode string
	}{
		{"japanese slug", alice, http.MethodPatch, path, map[string]any{"slug": "お知らせ"}, 422, "invalid_slug"},
		{"digits slug", alice, http.MethodPatch, path, map[string]any{"slug": "123"}, 422, "invalid_slug"},
		{"unknown field", alice, http.MethodPatch, path, map[string]any{"title": "x"}, 422, "invalid_json"},
		{"bad type", alice, http.MethodPost, "/api/contents", map[string]any{"type": "menu"}, 422, "invalid_type"},
		{"bad body", alice, http.MethodPut, path + "/draft", map[string]any{"title": "x", "body": json.RawMessage(`{"type":"doc","content":[{"type":"script"}]}`)}, 422, "invalid_body"},
		{"editor adds html", dave, http.MethodPut, path + "/draft", map[string]any{"title": "x", "body": json.RawMessage(`{"type":"doc","content":[{"type":"rawHtml","attrs":{"html":"<b>x</b>"}}]}`)}, 403, "forbidden"},
		{"parent on a post", alice, http.MethodPatch, path, map[string]any{"parentId": d.ID}, 422, "invalid_parent"},
		{"page at /posts/", alice, http.MethodPatch, fmt.Sprintf("/api/contents/%d", page.ID), map[string]any{"slug": "posts"}, 422, "reserved_slug"},
		{"page at /search/", alice, http.MethodPatch, fmt.Sprintf("/api/contents/%d", page.ID), map[string]any{"slug": "search"}, 422, "reserved_slug"},
		{"category slug", alice, http.MethodPost, "/api/categories", map[string]any{"name": "x", "slug": "カテゴリ"}, 422, "invalid_slug"},
		{"bad email", alice, http.MethodPost, "/api/auth/login-link", map[string]any{"email": "not an email"}, 422, "invalid_email"},
		// Bodies of the login endpoints stop at 64 KB, before parsing.
		{"huge login-link", alice, http.MethodPost, "/api/auth/login-link", map[string]any{"email": strings.Repeat("a", 70<<10)}, 422, "invalid_json"},
		{"huge verify", alice, http.MethodPost, "/api/auth/verify", map[string]any{"token": strings.Repeat("a", 70<<10)}, 422, "invalid_json"},
	}
	for _, tc := range cases {
		got, code := tc.c.errorCode(tc.method, tc.path, tc.in)
		if got != tc.want || code != tc.wantCode {
			t.Errorf("%s: %d %s, want %d %s", tc.name, got, code, tc.want, tc.wantCode)
		}
	}
	// Admins may add HTML blocks.
	alice.mustDo(http.MethodPut, path+"/draft", map[string]any{"title": "x", "body": json.RawMessage(`{"type":"doc","content":[{"type":"rawHtml","attrs":{"html":"<b>x</b>"}}]}`)}, nil, http.StatusOK)
	// Editors may keep them while editing around them.
	dave.mustDo(http.MethodPut, path+"/draft", map[string]any{"title": "y", "body": json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"},{"type":"rawHtml","attrs":{"html":"<b>x</b>"}}]}`)}, nil, http.StatusOK)
}

func pngBytes(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (c *client) upload(name string, data []byte, alt string, out any) int {
	c.e.t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = w.WriteField("altText", alt)
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, c.e.srv.URL+"/api/media", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Pagewright", "1")
	return c.send(req, out)
}

type mediaOut struct {
	ID          int64
	URL         string
	PreviewURL  string
	ContentType string
	Width       *int64
	AltText     string
}

func TestMedia(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	img := pngBytes(t)

	var m mediaOut
	if got := alice.upload("社屋.png", img, "社屋の外観", &m); got != http.StatusCreated {
		t.Fatalf("upload = %d", got)
	}
	if m.ContentType != "image/png" || m.Width == nil || *m.Width != 3 || !strings.HasPrefix(m.URL, "/media/") {
		t.Errorf("media = %+v", m)
	}
	if got := alice.upload("a.png", img, " ", nil); got != http.StatusUnprocessableEntity {
		t.Errorf("upload without alt = %d, want 422", got)
	}
	if got := alice.upload("a.png", []byte("not an image"), "x", nil); got != http.StatusUnprocessableEntity {
		t.Errorf("upload text = %d, want 422", got)
	}

	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+m.PreviewURL, nil)
	res, err := alice.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !bytes.Equal(got, img) {
		t.Errorf("file = %d (%d bytes), want the uploaded image", res.StatusCode, len(got))
	}

	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/media/%d", m.ID), map[string]string{"altText": "新しい説明"}, &m, http.StatusOK)
	if m.AltText != "新しい説明" {
		t.Errorf("alt after patch = %q", m.AltText)
	}
}

func TestMediaQuota(t *testing.T) {
	img := pngBytes(t)
	e := newEnvWith(t, func(o *api.Options) { o.MediaQuotaBytes = int64(2 * len(img)) })
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	var first mediaOut
	if got := alice.upload("a.png", img, "1枚目", &first); got != http.StatusCreated {
		t.Fatalf("first upload = %d", got)
	}
	if got := alice.upload("b.png", img, "2枚目", nil); got != http.StatusCreated {
		t.Fatalf("second upload = %d", got)
	}
	var out struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if got := alice.upload("c.png", img, "3枚目", &out); got != http.StatusUnprocessableEntity || out.Error.Code != "quota_exceeded" ||
		!strings.Contains(out.Error.Message, "古い画像を削除") || !strings.Contains(out.Error.Message, "サーバーの管理者") {
		t.Errorf("upload over quota = %d %+v, want 422 quota_exceeded", got, out.Error)
	}
	// Site A's images do not count against site B.
	if got := bob.upload("b.png", img, "Bの画像", nil); got != http.StatusCreated {
		t.Errorf("other site's upload = %d, want 201", got)
	}
	// Deleting an image frees its space.
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/media/%d", first.ID), nil, nil, http.StatusNoContent)
	if got := alice.upload("c.png", img, "3枚目", nil); got != http.StatusCreated {
		t.Errorf("upload after deleting = %d, want 201", got)
	}
}

// TestTenantIsolation checks every entry point against another site's data.
func TestTenantIsolation(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	// Bob's site B data, with settings waiting to be published.
	bPost := bob.newContent("post", "Bの記事", true)
	bob.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", bPost.ID), map[string]any{"slug": "b-pending"}, &bPost, http.StatusOK)
	if !bPost.PendingSettings {
		t.Fatalf("B's post has no pending settings: %+v", bPost)
	}
	bPage := bob.newContent("page", "Bのページ", true)
	var bCat, bTag struct{ ID int64 }
	bob.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "B", "slug": "b-cat"}, &bCat, http.StatusCreated)
	bob.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "B", "slug": "b-tag"}, &bTag, http.StatusCreated)
	var bMedia mediaOut
	if got := bob.upload("b.png", pngBytes(t), "B", &bMedia); got != http.StatusCreated {
		t.Fatalf("bob upload = %d", got)
	}

	// Alice's site A data.
	aPost := alice.newContent("post", "Aの記事", true)
	aPage := alice.newContent("page", "Aのページ", true)
	var aCat struct{ ID int64 }
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "A", "slug": "a-cat"}, &aCat, http.StatusCreated)

	// Lists show only A.
	lists := map[string]int{"/api/contents?type=post": 1, "/api/contents?type=page": 1, "/api/contents?type=post&sort=updated": 1, "/api/categories": 1, "/api/tags": 0, "/api/media": 0}
	for path, want := range lists {
		var got []json.RawMessage
		alice.mustDo(http.MethodGet, path, nil, &got, http.StatusOK)
		if len(got) != want {
			t.Errorf("GET %s from A: %d items, want %d", path, len(got), want)
		}
	}

	// Counts are of A only: B's pending settings are not A's changes.
	var counts map[string]contentCounts
	alice.mustDo(http.MethodGet, "/api/contents/counts", nil, &counts, http.StatusOK)
	if diff := cmp.Diff(map[string]contentCounts{"post": {Published: 1}, "page": {Published: 1}}, counts); diff != "" {
		t.Errorf("counts from A (-want +got):\n%s", diff)
	}

	// Direct access to B's items is "not found".
	notFound := []struct {
		method, path string
		in           any
	}{
		{http.MethodGet, fmt.Sprintf("/api/contents/%d", bPost.ID), nil},
		{http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", bPost.ID), map[string]any{"title": "x", "body": json.RawMessage(paragraph)}},
		{http.MethodPatch, fmt.Sprintf("/api/contents/%d", bPost.ID), map[string]any{"slug": "hijack"}},
		{http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", bPost.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/contents/%d/unpublish", bPost.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/contents/%d/schedule", bPost.ID), map[string]any{"publishAt": time.Now().Add(time.Hour)}},
		{http.MethodDelete, fmt.Sprintf("/api/contents/%d/schedule", bPost.ID), nil},
		{http.MethodDelete, fmt.Sprintf("/api/contents/%d", bPost.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/contents/%d/preview-links", bPost.ID), map[string]any{"days": 1}},
		{http.MethodPatch, fmt.Sprintf("/api/categories/%d", bCat.ID), map[string]any{"name": "x"}},
		{http.MethodDelete, fmt.Sprintf("/api/categories/%d", bCat.ID), nil},
		{http.MethodPatch, fmt.Sprintf("/api/tags/%d", bTag.ID), map[string]any{"name": "x"}},
		{http.MethodDelete, fmt.Sprintf("/api/tags/%d", bTag.ID), nil},
		{http.MethodDelete, fmt.Sprintf("/api/media/%d", bMedia.ID), nil},
		{http.MethodGet, fmt.Sprintf("/api/media/%d/file", bMedia.ID), nil},
		{http.MethodPatch, fmt.Sprintf("/api/media/%d", bMedia.ID), map[string]any{"altText": "x"}},
	}
	for _, n := range notFound {
		if got, _ := alice.errorCode(n.method, n.path, n.in); got != http.StatusNotFound {
			t.Errorf("%s %s from A = %d, want 404", n.method, n.path, got)
		}
	}

	// B's items cannot be attached to A's content or used as a parent.
	refs := []map[string]any{
		{"categoryIds": []int64{bCat.ID}},
		{"tagIds": []int64{bTag.ID}},
	}
	for _, in := range refs {
		if got, _ := alice.errorCode(http.MethodPatch, fmt.Sprintf("/api/contents/%d", aPost.ID), in); got != http.StatusUnprocessableEntity {
			t.Errorf("attach B's %v to A = %d, want 422", in, got)
		}
	}
	if got, _ := alice.errorCode(http.MethodPatch, fmt.Sprintf("/api/contents/%d", aPage.ID), map[string]any{"parentId": bPage.ID}); got != http.StatusUnprocessableEntity {
		t.Errorf("A's page under B's page = %d, want 422", got)
	}
	if got, _ := alice.errorCode(http.MethodPost, "/api/categories", map[string]any{"name": "x", "slug": "x", "parentId": bCat.ID}); got != http.StatusUnprocessableEntity {
		t.Errorf("category under B's category = %d, want 422", got)
	}

	// B is untouched, its pending settings included.
	var after detail
	bob.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", bPost.ID), nil, &after, http.StatusOK)
	if diff := cmp.Diff(bPost, after); diff != "" {
		t.Errorf("B's content changed (-before +after):\n%s", diff)
	}
	var pending []byte
	if err := e.db.Admin.QueryRow(context.Background(), "SELECT pending_settings FROM contents WHERE tenant_id = $1 AND id = $2", e.tenantB, bPost.ID).Scan(&pending); err != nil || string(pending) != `{"slug": "b-pending"}` {
		t.Errorf("B's pending settings = %s (%v), want only its own slug", pending, err)
	}
	// A's pending settings are A's alone.
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", aPost.ID), map[string]any{"slug": "a-pending"}, nil, http.StatusOK)
	if got, _ := bob.errorCode(http.MethodGet, fmt.Sprintf("/api/contents/%d", aPost.ID), nil); got != http.StatusNotFound {
		t.Errorf("B reading A's content with pending settings = %d, want 404", got)
	}
	if got, _ := bob.errorCode(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", aPost.ID), nil); got != http.StatusNotFound {
		t.Errorf("B publishing A's pending settings = %d, want 404", got)
	}
	var bCats []json.RawMessage
	bob.mustDo(http.MethodGet, "/api/categories", nil, &bCats, http.StatusOK)
	if len(bCats) != 1 {
		t.Errorf("B has %d categories, want 1", len(bCats))
	}
}

func TestSchedule(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	d := alice.newContent("post", "予約の記事", false)
	path := fmt.Sprintf("/api/contents/%d/schedule", d.ID)

	if got, code := alice.errorCode(http.MethodPost, path, map[string]any{"publishAt": time.Now().Add(-time.Hour)}); got != http.StatusUnprocessableEntity || code != "invalid_publish_at" {
		t.Errorf("schedule in the past = %d %s, want 422 invalid_publish_at", got, code)
	}

	at := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	alice.mustDo(http.MethodPost, path, map[string]any{"publishAt": at}, &d, http.StatusOK)
	if d.Status != "scheduled" || d.ScheduledAt == nil || !d.ScheduledAt.Equal(at) {
		t.Errorf("after schedule = %+v, want scheduled at %v", d, at)
	}
	e.builds.mu.Lock()
	got := e.builds.publishes
	e.builds.mu.Unlock()
	if len(got) != 1 || got[0].contentID != d.ID || got[0].revisionID != d.Revision.ID || !got[0].at.Equal(at) {
		t.Errorf("publish jobs = %+v, want one for content %d revision %d at %v", got, d.ID, d.Revision.ID, at)
	}

	alice.mustDo(http.MethodDelete, path, nil, &d, http.StatusOK)
	if d.Status != "draft" || d.ScheduledAt != nil {
		t.Errorf("after unschedule = %+v, want draft", d)
	}

	// Publishing now drops a pending schedule, so it cannot republish an
	// older revision later.
	alice.mustDo(http.MethodPost, path, map[string]any{"publishAt": at}, &d, http.StatusOK)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), nil, &d, http.StatusOK)
	if d.Status != "published" || d.ScheduledAt != nil {
		t.Errorf("after publishing now = %+v, want published without a schedule", d)
	}
}

func TestErrorField(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	d := alice.newContent("post", "x", false)
	var out struct {
		Error struct{ Code, Field string } `json:"error"`
	}
	if got := alice.do(http.MethodPatch, fmt.Sprintf("/api/contents/%d", d.ID), map[string]any{"slug": "日本語"}, &out); got != http.StatusUnprocessableEntity || out.Error.Field != "slug" {
		t.Errorf("bad slug = %d %+v, want 422 with field slug", got, out.Error)
	}
}

func TestEditConflict(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	d := alice.newContent("post", "最初", false)
	base := d.Revision.ID
	path := fmt.Sprintf("/api/contents/%d/draft", d.ID)

	// Someone else saves first.
	alice.mustDo(http.MethodPut, path, map[string]any{"title": "他の人", "body": json.RawMessage(paragraph), "baseRevisionId": base}, &d, http.StatusOK)
	if got, code := alice.errorCode(http.MethodPut, path, map[string]any{"title": "私", "body": json.RawMessage(paragraph), "baseRevisionId": base}); got != http.StatusConflict || code != "edit_conflict" {
		t.Errorf("save on a stale base = %d %s, want 409 edit_conflict", got, code)
	}
	alice.mustDo(http.MethodPut, path, map[string]any{"title": "私", "body": json.RawMessage(paragraph), "baseRevisionId": d.Revision.ID}, nil, http.StatusOK)
}

func TestDeletePageWithChildren(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	parent := alice.newContent("page", "親", true)
	child := alice.newContent("page", "子", false)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", child.ID), map[string]any{"parentId": parent.ID}, &child, http.StatusOK)
	if got, code := alice.errorCode(http.MethodDelete, fmt.Sprintf("/api/contents/%d", parent.ID), nil); got != http.StatusUnprocessableEntity || code != "has_children" {
		t.Errorf("delete a parent page = %d %s, want 422 has_children", got, code)
	}
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/contents/%d", child.ID), nil, nil, http.StatusNoContent)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/contents/%d", parent.ID), nil, nil, http.StatusNoContent)
}

// TestPublishConflict refuses to publish or schedule a revision the
// editor has not seen.
func TestPublishConflict(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	d := alice.newContent("post", "最初", false)
	seen := d.Revision.ID
	// Someone else saves a newer revision.
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID), map[string]any{"title": "別の人の変更", "body": json.RawMessage(paragraph)}, &d, http.StatusOK)

	stale := map[string]any{"baseRevisionId": seen}
	if got, code := alice.errorCode(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), stale); got != http.StatusConflict || code != "edit_conflict" {
		t.Errorf("publish with a stale revision = %d %s, want 409 edit_conflict", got, code)
	}
	at := map[string]any{"publishAt": time.Now().Add(time.Hour), "baseRevisionId": seen}
	if got, code := alice.errorCode(http.MethodPost, fmt.Sprintf("/api/contents/%d/schedule", d.ID), at); got != http.StatusConflict || code != "edit_conflict" {
		t.Errorf("schedule with a stale revision = %d %s, want 409 edit_conflict", got, code)
	}
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), map[string]any{"baseRevisionId": d.Revision.ID}, &d, http.StatusOK)
	if d.Status != "published" {
		t.Errorf("publish with the latest revision: status = %s", d.Status)
	}
	// The body stays optional.
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", d.ID), nil, &d, http.StatusOK)
}

func TestTagEditAndMediaDelete(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")

	var tag struct {
		ID   int64
		Slug string
	}
	alice.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "旧", "slug": "old"}, &tag, http.StatusCreated)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/tags/%d", tag.ID), map[string]any{"name": "新", "slug": "new"}, &tag, http.StatusOK)
	var kinds []string
	rows, err := e.db.Admin.Query(context.Background(), "SELECT path || ' ' || kind FROM routes WHERE tenant_id = $1 AND tag_id = $2 ORDER BY path", e.tenantA, tag.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		kinds = append(kinds, k)
	}
	rows.Close()
	if diff := cmp.Diff([]string{"/posts/tag/new/ canonical", "/posts/tag/old/ redirect"}, kinds); diff != "" {
		t.Errorf("tag routes (-want +got):\n%s", diff)
	}

	var used, unused mediaOut
	alice.upload("a.png", pngBytes(t), "使う画像", &used)
	alice.upload("b.png", pngBytes(t), "使わない画像", &unused)
	d := alice.newContent("post", "画像つき", false)
	body := fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"src":%q,"alt":"使う画像","mediaId":%d}}]}`, used.URL, used.ID)
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID), map[string]any{"title": "画像つき", "body": json.RawMessage(body)}, nil, http.StatusOK)

	if got, code := alice.errorCode(http.MethodDelete, fmt.Sprintf("/api/media/%d", used.ID), nil); got != http.StatusConflict || code != "media_in_use" {
		t.Errorf("deleting a used image = %d %s, want 409 media_in_use", got, code)
	}
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/media/%d", unused.ID), nil, nil, http.StatusNoContent)
	if got, _ := alice.errorCode(http.MethodGet, fmt.Sprintf("/api/media/%d/file", unused.ID), nil); got != http.StatusNotFound {
		t.Errorf("deleted image file = %d, want 404", got)
	}
}

func TestDeleteParentCategory(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	var parent, child struct {
		ID       int64
		ParentID *int64
	}
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "親", "slug": "parent"}, &parent, http.StatusCreated)
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "子", "slug": "child", "parentId": parent.ID}, &child, http.StatusCreated)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/categories/%d", parent.ID), nil, nil, http.StatusNoContent)
	var cats []struct {
		ID       int64
		ParentID *int64
	}
	alice.mustDo(http.MethodGet, "/api/categories", nil, &cats, http.StatusOK)
	if len(cats) != 1 || cats[0].ID != child.ID || cats[0].ParentID != nil {
		t.Errorf("categories after deleting the parent = %+v, want the child at the top", cats)
	}
}

type contentCounts struct {
	Draft, Scheduled, Published, Changed int64
	NextScheduledAt                      *time.Time
}

func TestContentCounts(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	path := func(id int64, suffix string) string { return fmt.Sprintf("/api/contents/%d%s", id, suffix) }

	var counts map[string]contentCounts
	alice.mustDo(http.MethodGet, "/api/contents/counts", nil, &counts, http.StatusOK)
	if diff := cmp.Diff(map[string]contentCounts{"post": {}, "page": {}}, counts); diff != "" {
		t.Errorf("counts of an empty site (-want +got):\n%s", diff)
	}

	first := alice.newContent("post", "下書き", false)
	alice.newContent("page", "下書きのページ", false)
	soon := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	later := soon.Add(time.Hour)
	for _, at := range []time.Time{later, soon} {
		d := alice.newContent("post", "予約", false)
		alice.mustDo(http.MethodPost, path(d.ID, "/schedule"), map[string]any{"publishAt": at}, nil, http.StatusOK)
	}
	alice.newContent("post", "公開中", true)
	edited := alice.newContent("post", "公開後に編集", true)
	alice.mustDo(http.MethodPut, path(edited.ID, "/draft"), map[string]any{"title": "編集", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)
	pending := alice.newContent("post", "公開後にURLを変更", true)
	alice.mustDo(http.MethodPatch, path(pending.ID, ""), map[string]any{"slug": "new-url"}, nil, http.StatusOK)
	// A live content with a scheduled update counts as published, not
	// scheduled, as its status says.
	update := alice.newContent("page", "更新を予約", true)
	alice.mustDo(http.MethodPut, path(update.ID, "/draft"), map[string]any{"title": "更新", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)
	alice.mustDo(http.MethodPost, path(update.ID, "/schedule"), map[string]any{"publishAt": soon.Add(-30 * time.Minute)}, nil, http.StatusOK)

	alice.mustDo(http.MethodGet, "/api/contents/counts", nil, &counts, http.StatusOK)
	want := map[string]contentCounts{
		"post": {Draft: 1, Scheduled: 2, Published: 3, Changed: 2, NextScheduledAt: &soon},
		"page": {Draft: 1, Published: 1, Changed: 1},
	}
	if diff := cmp.Diff(want, counts); diff != "" {
		t.Errorf("counts (-want +got):\n%s", diff)
	}

	// The counts agree with the lists.
	for typ, c := range counts {
		var list []detail
		alice.mustDo(http.MethodGet, "/api/contents?type="+typ, nil, &list, http.StatusOK)
		got := contentCounts{NextScheduledAt: c.NextScheduledAt}
		for _, d := range list {
			switch d.Status {
			case "draft":
				got.Draft++
			case "scheduled":
				got.Scheduled++
			case "published":
				got.Published++
			}
			if d.HasUnpublishedChanges {
				got.Changed++
			}
		}
		if diff := cmp.Diff(c, got); diff != "" {
			t.Errorf("%s counts differ from the list (-counts +list):\n%s", typ, diff)
		}
	}

	// sort=updated lists the last edited first.
	alice.mustDo(http.MethodPut, path(first.ID, "/draft"), map[string]any{"title": "いま編集", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)
	var recent []detail
	alice.mustDo(http.MethodGet, "/api/contents?type=post&sort=updated&limit=2", nil, &recent, http.StatusOK)
	if len(recent) != 2 || recent[0].ID != first.ID || recent[0].Title != "いま編集" || recent[1].ID != pending.ID {
		t.Errorf("recent posts = %+v, want %d then %d", recent, first.ID, pending.ID)
	}
	for _, q := range []string{"sort=number", "sort=updated&status=draft", "sort=updated&q=x", "sort=updated&before=3"} {
		if got, code := alice.errorCode(http.MethodGet, "/api/contents?type=post&"+q, nil); got != http.StatusUnprocessableEntity || code != "invalid_sort" {
			t.Errorf("%s = %d %s, want 422 invalid_sort", q, got, code)
		}
	}
}

func TestTaxonomySlugConflict(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "お知らせ", "slug": "news"}, nil, http.StatusCreated)
	alice.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "イベント", "slug": "event"}, nil, http.StatusCreated)
	var other struct{ ID int64 }
	alice.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "その他", "slug": "other"}, &other, http.StatusCreated)

	tests := []struct {
		name, method, path string
		in                 map[string]any
		word               string
	}{
		{"new category", http.MethodPost, "/api/categories", map[string]any{"name": "別", "slug": "news"}, "カテゴリで"},
		{"new tag", http.MethodPost, "/api/tags", map[string]any{"name": "別", "slug": "event"}, "タグ"},
		{"renamed tag", http.MethodPatch, fmt.Sprintf("/api/tags/%d", other.ID), map[string]any{"slug": "event"}, "タグ"},
	}
	for _, tt := range tests {
		var out struct {
			Error struct{ Code, Message, Field string } `json:"error"`
		}
		if got := alice.do(tt.method, tt.path, tt.in, &out); got != http.StatusConflict || out.Error.Code != "conflict" || out.Error.Field != "slug" || !strings.Contains(out.Error.Message, tt.word) {
			t.Errorf("%s = %d %+v, want 409 conflict on slug naming %s", tt.name, got, out.Error, tt.word)
		}
	}
	// A slug that cannot be used names its field as the admin labels it.
	for path, word := range map[string]string{"/api/categories": "カテゴリのURL（英数字）", "/api/tags": "タグのURL（英数字）"} {
		var out struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		if got := alice.do(http.MethodPost, path, map[string]any{"name": "空", "slug": ""}, &out); got != http.StatusUnprocessableEntity || !strings.HasPrefix(out.Error.Message, word) {
			t.Errorf("%s with no slug = %d %+v, want 422 naming %s", path, got, out.Error, word)
		}
	}
	// Another site may use the same slugs.
	bob.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "お知らせ", "slug": "news"}, nil, http.StatusCreated)
	bob.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "イベント", "slug": "event"}, nil, http.StatusCreated)
}

func TestLoginLinkRateLimit(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	for i := range 6 {
		if got := c.do(http.MethodPost, "/api/auth/login-link", map[string]string{"email": "Alice@Example.com"}, nil); got != http.StatusNoContent {
			t.Fatalf("request %d = %d, want 204", i+1, got)
		}
	}
	// The 6th answers the same but sends nothing.
	if n := len(e.mail.all()); n != 5 {
		t.Errorf("%d login mails sent, want 5", n)
	}
	// Other addresses are not affected.
	c.mustDo(http.MethodPost, "/api/auth/login-link", map[string]string{"email": "bob@example.com"}, nil, http.StatusNoContent)
	if _, ok := e.mail.last(); !ok {
		t.Error("no mail to bob")
	}
}

type siteOut struct {
	Name         string
	LogoMediaID  *int64
	PrimaryColor string
	Font         string
	Menu         []struct {
		Label     *string
		ContentID *int64
		URL       *string
	}
	Logo          *mediaOut
	PublicBaseURL string
}

func TestSiteSettings(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	dave := e.login("dave@example.com")
	bob := e.login("bob@example.com")

	var s siteOut
	alice.mustDo(http.MethodGet, "/api/site", nil, &s, http.StatusOK)
	if s.Name != "サイトA" || len(s.Menu) != 0 || s.PublicBaseURL != "https://site-a.example" {
		t.Fatalf("site = %+v", s)
	}

	var logo mediaOut
	alice.upload("logo.png", pngBytes(t), "サイトAのロゴ", &logo)
	page := alice.newContent("page", "会社概要", true)
	in := map[string]any{
		"name": "サイトA（新）", "logoMediaId": logo.ID, "primaryColor": "#0a4f8f", "font": "mincho",
		"menu": []map[string]any{{"contentId": page.ID}, {"label": "採用情報", "url": "https://example.com/jobs"}},
	}
	if got, _ := dave.errorCode(http.MethodPut, "/api/site", in); got != http.StatusForbidden {
		t.Errorf("editor changing settings = %d, want 403", got)
	}
	alice.mustDo(http.MethodPut, "/api/site", in, &s, http.StatusOK)
	if s.Name != "サイトA（新）" || s.Logo == nil || s.Logo.ID != logo.ID || s.Font != "mincho" || len(s.Menu) != 2 || *s.Menu[1].URL != "https://example.com/jobs" {
		t.Errorf("after update = %+v", s)
	}
	if got, code := alice.errorCode(http.MethodDelete, fmt.Sprintf("/api/media/%d", logo.ID), nil); got != http.StatusConflict || code != "media_in_use" {
		t.Errorf("deleting the logo = %d %s, want 409 media_in_use", got, code)
	}

	var out struct {
		Error struct{ Code, Field string } `json:"error"`
	}
	in["primaryColor"] = "#a0c4ff"
	if got := alice.do(http.MethodPut, "/api/site", in, &out); got != http.StatusUnprocessableEntity || out.Error.Field != "primaryColor" {
		t.Errorf("low-contrast color = %d %+v, want 422 on primaryColor", got, out.Error)
	}
	in["primaryColor"] = "#0a4f8f"

	// Another site's page and image cannot be used.
	bPage := bob.newContent("page", "Bのページ", true)
	var bLogo mediaOut
	bob.upload("b.png", pngBytes(t), "B", &bLogo)
	bad := []map[string]any{
		{"name": "x", "menu": []map[string]any{{"contentId": bPage.ID}}},
		{"name": "x", "logoMediaId": bLogo.ID},
	}
	for _, b := range bad {
		if got, _ := alice.errorCode(http.MethodPut, "/api/site", b); got != http.StatusUnprocessableEntity {
			t.Errorf("PUT %v from A = %d, want 422", b, got)
		}
	}
	var bs siteOut
	bob.mustDo(http.MethodGet, "/api/site", nil, &bs, http.StatusOK)
	if bs.Name != "サイトB" || bs.Logo != nil {
		t.Errorf("site B changed: %+v", bs)
	}
}

// Each site's public URL (set by the operator) reaches its own admin,
// previews and form result pages only; a site without one uses the
// server's PublicBaseURL.
func TestSitePublicURL(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.db.Admin.Exec(ctx, "UPDATE tenants SET public_base_url = 'https://site-b.example' WHERE id = $1", e.tenantB); err != nil {
		t.Fatal(err)
	}
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	var as, bs siteOut
	alice.mustDo(http.MethodGet, "/api/site", nil, &as, http.StatusOK)
	bob.mustDo(http.MethodGet, "/api/site", nil, &bs, http.StatusOK)
	if as.PublicBaseURL != "https://site-a.example" || bs.PublicBaseURL != "https://site-b.example" {
		t.Errorf("publicBaseUrl: A = %q, B = %q", as.PublicBaseURL, bs.PublicBaseURL)
	}
	bob.mustDo(http.MethodPut, "/api/site", map[string]any{"name": "サイトB"}, &bs, http.StatusOK)
	if bs.PublicBaseURL != "https://site-b.example" {
		t.Errorf("after PUT: B = %q", bs.PublicBaseURL)
	}

	// The admin cannot change it: not through the API, nor as the app role.
	if got, code := bob.errorCode(http.MethodPut, "/api/site", map[string]any{"name": "サイトB", "publicBaseUrl": "https://evil.example"}); got != http.StatusUnprocessableEntity || code != "invalid_json" {
		t.Errorf("PUT publicBaseUrl = %d %s, want 422 invalid_json", got, code)
	}
	err := e.db.DB.InTenantTx(db.WithTenant(ctx, e.tenantB), func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := tx.Exec(ctx, "UPDATE tenants SET public_base_url = 'https://evil.example'")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("app role updating public_base_url: err = %v, want permission denied", err)
	}
	// A cannot read B's row at all.
	var n int
	err = e.db.DB.InTenantTx(db.WithTenant(ctx, e.tenantA), func(tx pgx.Tx, _ *dbgen.Queries) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM tenants WHERE public_base_url IS NOT NULL").Scan(&n)
	})
	if err != nil || n != 0 {
		t.Errorf("A sees %d sites with a URL (err %v), want 0", n, err)
	}

	// Form result pages link back to the site the form is on.
	for tenant, want := range map[uuid.UUID]string{e.tenantA: "https://site-a.example/contact/", e.tenantB: "https://site-b.example/contact/"} {
		status, page := e.postContact(tenant, contactForm())
		if status != http.StatusOK || !strings.Contains(page, `href="`+want+`"`) {
			t.Errorf("contact on %s = %d, want a link to %s:\n%s", tenant, status, want, page)
		}
	}

	// Previews resolve links against the site's own URL.
	d := bob.newContent("post", "Bの下書き", false)
	_, _, page := bob.getPage(fmt.Sprintf("/api/contents/%d/preview/", d.ID))
	if !strings.Contains(page, `<base href="https://site-b.example/">`) || strings.Contains(page, "site-a.example") {
		t.Errorf("B's admin preview does not use B's URL:\n%s", page)
	}
	var link struct{ URL string }
	bob.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/preview-links", d.ID), map[string]any{"days": 1}, &link, http.StatusCreated)
	if _, _, page := e.getPreview(link.URL, ""); !strings.Contains(page, `<base href="https://site-b.example/">`) {
		t.Errorf("B's preview link does not use B's URL:\n%s", page)
	}
}

func (e *env) postContact(tenant uuid.UUID, form url.Values) (int, string) {
	e.t.Helper()
	status, _, page := e.postContactHeader(tenant, form, "")
	return status, page
}

// postContactFrom is postContact with X-Forwarded-For set to ip (when not
// empty), for servers that trust the test client as a proxy.
func (e *env) postContactFrom(tenant uuid.UUID, form url.Values, ip string) (int, string) {
	e.t.Helper()
	status, _, page := e.postContactHeader(tenant, form, ip)
	return status, page
}

func (e *env) postContactHeader(tenant uuid.UUID, form url.Values, ip string) (int, http.Header, string) {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/f/"+tenant.String()+"/contact", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://site-a.example/contact/")
	if ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(b)
}

func contactForm() url.Values {
	return url.Values{
		"name": {"山田 太郎<script>"}, "kana": {"やまだ たろう"}, "email": {"taro@example.jp"},
		"phone": {"03-1234-5678"}, "message": {"見積もりをお願いします。"}, "consent": {"yes"}, "page": {"/contact/"},
	}
}

func TestContactForm(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	dave := e.login("dave@example.com")
	sentBefore := len(e.mail.all())
	if status, _ := e.postContact(uuid.New(), contactForm()); status != http.StatusNotFound {
		t.Errorf("unknown site = %d, want 404", status)
	}

	status, page := e.postContact(e.tenantA, contactForm())
	if status != http.StatusOK || !strings.Contains(page, "送信しました") || !strings.Contains(page, `href="https://site-a.example/contact/"`) {
		t.Fatalf("post = %d:\n%s", status, page)
	}
	sent := e.mail.all()[sentBefore:]
	var to []string
	for _, m := range sent {
		to = append(to, m.To)
	}
	if diff := cmp.Diff([]string{"alice@example.com", "taro@example.jp"}, to); diff != "" {
		t.Errorf("mail recipients (-want +got):\n%s", diff)
	}
	// The answer is drawn in the site's theme, with the public site as its
	// base, and a policy that lets in that site's styles and images only.
	for _, want := range []string{`<base href="https://site-a.example/">`, `class="site-header"`, `href="/_assets/style.css?v=`} {
		if !strings.Contains(page, want) {
			t.Errorf("result page lacks %q", want)
		}
	}

	// Errors show the form again, filled in, with a message at each field.
	bad := contactForm()
	bad.Set("email", "taro")
	bad.Del("consent")
	status, hdr, page := e.postContactHeader(e.tenantA, bad, "")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("bad post = %d", status)
	}
	for _, want := range []string{
		"入力内容を確認してください", `value="taro"`, `value="山田 太郎&lt;script&gt;"`, "見積もりをお願いします。</textarea>",
		`id="cf-email" name="email"`, `aria-describedby="cf-email-error"`, `href="` + "http://admin.test/f/" + e.tenantA.String() + `/contact#cf-consent"`,
		`action="http://admin.test/f/` + e.tenantA.String() + `/contact"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("error page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "<script>") || strings.Contains(page, `aria-describedby="cf-name-error"`) {
		t.Errorf("error page has a script tag or an error on a good field:\n%s", page)
	}
	csp := hdr.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "style-src 'unsafe-inline' https://site-a.example;", "form-action http://admin.test"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "script-src") {
		t.Errorf("CSP %q allows scripts", csp)
	}

	var subs []struct {
		ID   int64
		Name string
	}
	alice.mustDo(http.MethodGet, "/api/forms/submissions", nil, &subs, http.StatusOK)
	if len(subs) != 1 || subs[0].Name != "山田 太郎<script>" {
		t.Fatalf("A's submissions = %+v", subs)
	}

	rejected := map[string]url.Values{}
	for _, c := range []struct{ name, key, value string }{
		{"no consent", "consent", ""}, {"bad email", "email", "taro"}, {"empty message", "message", " "},
	} {
		f := contactForm()
		f.Set(c.key, c.value)
		rejected[c.name] = f
	}
	for name, f := range rejected {
		if status, _ := e.postContact(e.tenantA, f); status != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422", name, status)
		}
	}
	bot := contactForm()
	bot.Set("website", "http://spam.example")
	if status, _ := e.postContact(e.tenantA, bot); status != http.StatusOK {
		t.Errorf("honeypot = %d, want 200 (silently dropped)", status)
	}
	alice.mustDo(http.MethodGet, "/api/forms/submissions", nil, &subs, http.StatusOK)
	if len(subs) != 1 {
		t.Errorf("rejected or bot posts were stored: %d submissions", len(subs))
	}

	// Personal data: only admins of the same site.
	var bSubs []json.RawMessage
	bob.mustDo(http.MethodGet, "/api/forms/submissions", nil, &bSubs, http.StatusOK)
	if len(bSubs) != 0 {
		t.Errorf("site B sees %d of A's submissions", len(bSubs))
	}
	if got, _ := bob.errorCode(http.MethodDelete, fmt.Sprintf("/api/forms/submissions/%d", subs[0].ID), nil); got != http.StatusNotFound {
		t.Errorf("B deleting A's submission = %d, want 404", got)
	}
	if got, _ := dave.errorCode(http.MethodGet, "/api/forms/submissions", nil); got != http.StatusForbidden {
		t.Errorf("editor reading submissions = %d, want 403", got)
	}
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/forms/submissions/%d", subs[0].ID), nil, nil, http.StatusNoContent)
}

type submissionOut struct {
	ID        int64
	Name      string
	HandledAt *time.Time
	HandledBy *string
}

func TestSubmissionHandled(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	dave := e.login("dave@example.com")
	for range 2 {
		if status, _ := e.postContact(e.tenantA, contactForm()); status != http.StatusOK {
			t.Fatalf("post to A = %d", status)
		}
	}
	if status, _ := e.postContact(e.tenantB, contactForm()); status != http.StatusOK {
		t.Fatalf("post to B = %d", status)
	}
	var subs, bSubs []submissionOut
	alice.mustDo(http.MethodGet, "/api/forms/submissions", nil, &subs, http.StatusOK)
	bob.mustDo(http.MethodGet, "/api/forms/submissions", nil, &bSubs, http.StatusOK)
	if len(subs) != 2 || len(bSubs) != 1 || subs[0].HandledAt != nil || subs[0].HandledBy != nil {
		t.Fatalf("A's = %+v, B's = %+v; want 2 and 1, unhandled", subs, bSubs)
	}
	newer, older := subs[0], subs[1]

	var got submissionOut
	path := fmt.Sprintf("/api/forms/submissions/%d", older.ID)
	alice.mustDo(http.MethodPatch, path, map[string]any{"handled": true}, &got, http.StatusOK)
	if got.ID != older.ID || got.HandledAt == nil || got.HandledBy == nil || *got.HandledBy != "alice@example.com" {
		t.Fatalf("after handling = %+v, want handled by alice", got)
	}
	// Marking it again keeps the first time.
	first := *got.HandledAt
	alice.mustDo(http.MethodPatch, path, map[string]any{"handled": true}, &got, http.StatusOK)
	if got.HandledAt == nil || !got.HandledAt.Equal(first) {
		t.Errorf("handled again at %v, want kept %v", got.HandledAt, first)
	}

	filters := map[string][]int64{
		"":               {newer.ID, older.ID},
		"?handled=false": {newer.ID},
		"?handled=true":  {older.ID},
		fmt.Sprintf("?handled=false&before=%d", newer.ID): {},
	}
	for q, want := range filters {
		var list []submissionOut
		alice.mustDo(http.MethodGet, "/api/forms/submissions"+q, nil, &list, http.StatusOK)
		ids := []int64{}
		for _, s := range list {
			ids = append(ids, s.ID)
		}
		if diff := cmp.Diff(want, ids); diff != "" {
			t.Errorf("GET %s (-want +got):\n%s", q, diff)
		}
	}
	if got, code := alice.errorCode(http.MethodGet, "/api/forms/submissions?handled=yes", nil); got != http.StatusUnprocessableEntity || code != "invalid_handled" {
		t.Errorf("handled=yes = %d %s, want 422 invalid_handled", got, code)
	}
	if got, code := alice.errorCode(http.MethodPatch, path, map[string]any{}); got != http.StatusUnprocessableEntity || code != "invalid_handled" {
		t.Errorf("PATCH without handled = %d %s, want 422 invalid_handled", got, code)
	}
	// The message itself cannot be changed.
	if got, code := alice.errorCode(http.MethodPatch, path, map[string]any{"handled": true, "message": "x"}); got != http.StatusUnprocessableEntity || code != "invalid_json" {
		t.Errorf("PATCH with message = %d %s, want 422 invalid_json", got, code)
	}

	// Admins of the same site only: another site's message is not found,
	// in either direction, and editors are refused.
	if got, _ := bob.errorCode(http.MethodPatch, path, map[string]any{"handled": false}); got != http.StatusNotFound {
		t.Errorf("B marking A's message = %d, want 404", got)
	}
	if got, _ := alice.errorCode(http.MethodPatch, fmt.Sprintf("/api/forms/submissions/%d", bSubs[0].ID), map[string]any{"handled": true}); got != http.StatusNotFound {
		t.Errorf("A marking B's message = %d, want 404", got)
	}
	if got, _ := dave.errorCode(http.MethodPatch, path, map[string]any{"handled": false}); got != http.StatusForbidden {
		t.Errorf("editor marking a message = %d, want 403", got)
	}
	bob.mustDo(http.MethodGet, "/api/forms/submissions?handled=true", nil, &bSubs, http.StatusOK)
	if len(bSubs) != 0 {
		t.Errorf("B sees %d handled messages, want 0", len(bSubs))
	}
	ctx := context.Background()
	var bHandled *time.Time
	if err := e.db.Admin.QueryRow(ctx, "SELECT handled_at FROM form_submissions WHERE tenant_id = $1", e.tenantB).Scan(&bHandled); err != nil || bHandled != nil {
		t.Errorf("B's message handled_at = %v (%v), want untouched", bHandled, err)
	}
	alice.mustDo(http.MethodGet, "/api/forms/submissions?handled=true", nil, &subs, http.StatusOK)
	if len(subs) != 1 || subs[0].ID != older.ID {
		t.Errorf("A's handled after B's attempts = %+v, want only %d", subs, older.ID)
	}

	alice.mustDo(http.MethodPatch, path, map[string]any{"handled": false}, &got, http.StatusOK)
	if got.HandledAt != nil || got.HandledBy != nil {
		t.Errorf("after unhandling = %+v, want unhandled", got)
	}

	// When the member who handled it leaves, it stays handled, by nobody.
	alice.mustDo(http.MethodPatch, path, map[string]any{"handled": true}, nil, http.StatusOK)
	// dave takes over as admin first: a site always keeps one.
	if _, err := e.db.Admin.Exec(ctx, "UPDATE memberships SET role = 'admin' WHERE tenant_id = $1 AND user_id = (SELECT id FROM users WHERE email = 'dave@example.com')", e.tenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Admin.Exec(ctx, "DELETE FROM memberships WHERE tenant_id = $1 AND user_id = (SELECT id FROM users WHERE email = 'alice@example.com')", e.tenantA); err != nil {
		t.Fatal(err)
	}
	var handledAt *time.Time
	var handledBy *uuid.UUID
	if err := e.db.Admin.QueryRow(ctx, "SELECT handled_at, handled_by FROM form_submissions WHERE tenant_id = $1 AND id = $2", e.tenantA, older.ID).Scan(&handledAt, &handledBy); err != nil {
		t.Fatal(err)
	}
	if handledAt == nil || handledBy != nil {
		t.Errorf("after alice left: handled_at=%v handled_by=%v, want still handled, by nobody", handledAt, handledBy)
	}
}

func TestContactFormRateLimit(t *testing.T) {
	e := newEnv(t)
	for i := range 10 {
		if status, _ := e.postContact(e.tenantA, contactForm()); status != http.StatusOK {
			t.Fatalf("post %d = %d", i+1, status)
		}
	}
	if status, _ := e.postContact(e.tenantA, contactForm()); status != http.StatusTooManyRequests {
		t.Errorf("11th post = %d, want 429", status)
	}
}

// Junk posts (honeypot hits, invalid input) from many addresses must not
// use up the site's budget (200 an hour) for real messages.
func TestContactFormJunkDoesNotUseSiteLimit(t *testing.T) {
	e := newEnvWith(t, func(o *api.Options) {
		o.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}
	})
	bot := contactForm()
	bot.Set("website", "http://spam.example")
	invalid := contactForm()
	invalid.Set("email", "taro")
	for i := range 210 {
		f, want := bot, http.StatusOK
		if i%2 == 1 {
			f, want = invalid, http.StatusUnprocessableEntity
		}
		if status, _ := e.postContactFrom(e.tenantA, f, fmt.Sprintf("203.0.113.%d", i%250+1)); status != want {
			t.Fatalf("junk post %d = %d, want %d", i+1, status, want)
		}
	}
	if status, page := e.postContactFrom(e.tenantA, contactForm(), "198.51.100.1"); status != http.StatusOK {
		t.Fatalf("real post after junk = %d:\n%s", status, page)
	}
}

// getPreview fetches a preview URL (issued for http://admin.test) from the
// test server, without any session.
func (e *env) getPreview(previewURL, suffix string) (int, http.Header, string) {
	e.t.Helper()
	path := strings.TrimPrefix(previewURL, "http://admin.test")
	res, err := http.Get(e.srv.URL + path + suffix)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(b)
}

// Guessing preview tokens through the files of a link is rate-limited too.
func TestPreviewFilesRateLimit(t *testing.T) {
	e := newEnv(t)
	for i := range 600 {
		suffix := "media/x.png"
		if i%2 == 1 {
			suffix = "_assets/style.css"
		}
		if status, _, _ := e.getPreview(fmt.Sprintf("http://admin.test/p/guess%d/", i), suffix); status != http.StatusNotFound {
			t.Fatalf("lookup %d = %d, want 404", i+1, status)
		}
	}
	if status, _, _ := e.getPreview("http://admin.test/p/guess/", "media/x.png"); status != http.StatusTooManyRequests {
		t.Errorf("lookup 601 = %d, want 429", status)
	}
}

func TestPreviewLinks(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	var img mediaOut
	alice.upload("a.png", pngBytes(t), "社屋", &img)
	d := alice.newContent("post", "下書きの記事", false)
	body := fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"まだ公開していない本文"}]},{"type":"image","attrs":{"src":%q,"alt":"社屋","mediaId":%d}}]}`, img.URL, img.ID)
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID), map[string]any{"title": "下書きの記事", "body": json.RawMessage(body)}, nil, http.StatusOK)

	if got, _ := alice.errorCode(http.MethodPost, fmt.Sprintf("/api/contents/%d/preview-links", d.ID), map[string]any{"days": 31}); got != http.StatusUnprocessableEntity {
		t.Errorf("31 days = %d, want 422", got)
	}
	var link struct {
		ID        int64
		URL       string
		ExpiresAt time.Time
	}
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/preview-links", d.ID), map[string]any{"days": 1}, &link, http.StatusCreated)
	if !strings.HasPrefix(link.URL, "http://admin.test/p/") || time.Until(link.ExpiresAt) > 25*time.Hour {
		t.Fatalf("link = %+v", link)
	}

	status, hdr, page := e.getPreview(link.URL, "")
	if status != http.StatusOK {
		t.Fatalf("preview = %d:\n%s", status, page)
	}
	for _, want := range []string{"まだ公開していない本文", "これはプレビューです", `name="robots" content="noindex"`, `<base href="https://site-a.example/">`, link.URL + "media/"} {
		if !strings.Contains(page, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	if hdr.Get("X-Robots-Tag") == "" || hdr.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("preview headers = %v", hdr)
	}
	key := strings.TrimPrefix(img.URL, "/media/")
	if status, _, _ := e.getPreview(link.URL, "media/"+key); status != http.StatusOK {
		t.Errorf("preview image = %d, want 200", status)
	}

	// The newest draft shows up without a new link.
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID), map[string]any{"title": "書き直した記事", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)
	if _, _, page := e.getPreview(link.URL, ""); !strings.Contains(page, "書き直した記事") {
		t.Error("preview does not show the latest revision")
	}

	// Another site can neither see, revoke nor use A's links, and a link
	// of A does not serve B's images.
	if got, _ := bob.errorCode(http.MethodGet, fmt.Sprintf("/api/contents/%d/preview-links", d.ID), nil); got != http.StatusNotFound {
		t.Errorf("B listing A's links = %d, want 404", got)
	}
	if got, _ := bob.errorCode(http.MethodDelete, fmt.Sprintf("/api/preview-links/%d", link.ID), nil); got != http.StatusNotFound {
		t.Errorf("B revoking A's link = %d, want 404", got)
	}
	var bImg mediaOut
	bob.upload("b.png", pngBytes(t), "B", &bImg)
	if status, _, _ := e.getPreview(link.URL, "media/"+strings.TrimPrefix(bImg.URL, "/media/")); status != http.StatusNotFound {
		t.Errorf("B's image through A's link = %d, want 404", status)
	}

	// Expired and revoked links stop working.
	if _, err := e.db.Admin.Exec(context.Background(), "UPDATE preview_links SET expires_at = now() - interval '1 second' WHERE tenant_id = $1 AND id = $2", e.tenantA, link.ID); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := e.getPreview(link.URL, ""); status != http.StatusNotFound {
		t.Errorf("expired link = %d, want 404", status)
	}
	var second struct {
		ID  int64
		URL string
	}
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/preview-links", d.ID), map[string]any{}, &second, http.StatusCreated)
	alice.mustDo(http.MethodDelete, fmt.Sprintf("/api/preview-links/%d", second.ID), nil, nil, http.StatusNoContent)
	if status, _, _ := e.getPreview(second.URL, ""); status != http.StatusNotFound {
		t.Errorf("revoked link = %d, want 404", status)
	}
}

// getPage fetches a page (not JSON) with the client's cookies.
func (c *client) getPage(path string) (int, http.Header, string) {
	c.e.t.Helper()
	res, err := c.http.Get(c.e.srv.URL + path)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(b)
}

func TestAdminPreview(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	var img mediaOut
	alice.upload("a.png", pngBytes(t), "社屋", &img)
	d := alice.newContent("post", "下書きの記事", false)
	body := fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"まだ公開していない本文"}]},{"type":"image","attrs":{"src":%q,"alt":"社屋","mediaId":%d}}]}`, img.URL, img.ID)
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID), map[string]any{"title": "下書きの記事", "body": json.RawMessage(body)}, nil, http.StatusOK)

	self := fmt.Sprintf("/api/contents/%d/preview/", d.ID)
	status, hdr, page := alice.getPage(self)
	if status != http.StatusOK {
		t.Fatalf("preview = %d:\n%s", status, page)
	}
	for _, want := range []string{"まだ公開していない本文", "これはプレビューです", `<base href="https://site-a.example/">`, "http://admin.test" + self + "media/"} {
		if !strings.Contains(page, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	if csp := hdr.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox allow-same-origin;") || !strings.Contains(csp, "script-src 'none'") {
		t.Errorf("preview CSP = %q", csp)
	}
	if hdr.Get("X-Robots-Tag") == "" || hdr.Get("Cache-Control") != "private, no-store" {
		t.Errorf("preview headers = %v", hdr)
	}
	key := strings.TrimPrefix(img.URL, "/media/")
	if status, _, _ := alice.getPage(self + "media/" + key); status != http.StatusOK {
		t.Errorf("preview image = %d, want 200", status)
	}
	// No preview link is made.
	var links []json.RawMessage
	alice.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d/preview-links", d.ID), nil, &links, http.StatusOK)
	if len(links) != 0 {
		t.Errorf("links = %s, want none", links)
	}

	// Without a login, and from another site, nothing is shown.
	if status, _, page := e.anon().getPage(self); status != http.StatusUnauthorized || strings.Contains(page, "まだ公開していない本文") {
		t.Errorf("anonymous preview = %d", status)
	}
	if status, _, page := bob.getPage(self); status != http.StatusNotFound || strings.Contains(page, "まだ公開していない本文") {
		t.Errorf("B previewing A's content = %d, want 404", status)
	}
	if status, _, _ := bob.getPage(self + "media/" + key); status != http.StatusNotFound {
		t.Errorf("B fetching A's image = %d, want 404", status)
	}
	if status, _, _ := e.anon().getPage(self + "media/" + key); status != http.StatusUnauthorized {
		t.Errorf("anonymous image = %d, want 401", status)
	}
}

func TestListContentsPaging(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	for _, title := range []string{"一", "二", "三"} {
		alice.newContent("post", title, false)
	}
	bob.newContent("post", "B", false)

	titles := func(ds []detail) []string {
		var out []string
		for _, d := range ds {
			out = append(out, d.Title)
		}
		return out
	}
	var first, rest []detail
	alice.mustDo(http.MethodGet, "/api/contents?type=post&limit=2", nil, &first, http.StatusOK)
	if diff := cmp.Diff([]string{"三", "二"}, titles(first)); diff != "" {
		t.Errorf("first page (-want +got):\n%s", diff)
	}
	alice.mustDo(http.MethodGet, fmt.Sprintf("/api/contents?type=post&limit=2&before=%d", first[1].Number), nil, &rest, http.StatusOK)
	if diff := cmp.Diff([]string{"一"}, titles(rest)); diff != "" {
		t.Errorf("next page (-want +got):\n%s", diff)
	}
	for _, q := range []string{"limit=0", "limit=1001", "limit=x", "before=0", "before=-1", "before=x"} {
		if got, _ := alice.errorCode(http.MethodGet, "/api/contents?type=post&"+q, nil); got != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422", q, got)
		}
	}
}

func TestListChangedContents(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	alice.newContent("post", "そのまま", true)
	alice.newContent("post", "下書き", false)
	edited := alice.newContent("post", "書き直し前", true)
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", edited.ID), map[string]any{"title": "書き直した", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)
	slugged := alice.newContent("post", "URLだけ", true)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", slugged.ID), map[string]any{"slug": "only-url"}, nil, http.StatusOK)
	bEdited := bob.newContent("post", "B", true)
	bob.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", bEdited.ID), map[string]any{"title": "Bの書き直し", "body": json.RawMessage(paragraph)}, nil, http.StatusOK)

	var got []detail
	alice.mustDo(http.MethodGet, "/api/contents?type=post&status=changed", nil, &got, http.StatusOK)
	var titles []string
	for _, d := range got {
		titles = append(titles, d.Title)
		if !d.HasUnpublishedChanges {
			t.Errorf("%s: hasUnpublishedChanges = false", d.Title)
		}
	}
	if diff := cmp.Diff([]string{"URLだけ", "書き直した"}, titles); diff != "" {
		t.Errorf("changed (-want +got):\n%s", diff)
	}
}

func TestContentSearchAndBuilds(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	alice.newContent("post", "夏のセール 50%オフ", true)
	alice.newContent("post", "秋のお知らせ", false)
	bob.newContent("post", "夏のセール（B）", true)

	tests := []struct {
		query string
		want  []string
	}{
		{"q=" + url.QueryEscape("セール"), []string{"夏のセール 50%オフ"}},
		{"q=" + url.QueryEscape("50%"), []string{"夏のセール 50%オフ"}},
		{"q=" + url.QueryEscape("%"), []string{"夏のセール 50%オフ"}}, // % is literal
		{"status=draft", []string{"秋のお知らせ"}},
		{"status=published&q=" + url.QueryEscape("秋"), nil},
	}
	for _, tt := range tests {
		var got []detail
		alice.mustDo(http.MethodGet, "/api/contents?type=post&"+tt.query, nil, &got, http.StatusOK)
		var titles []string
		for _, d := range got {
			titles = append(titles, d.Title)
		}
		if diff := cmp.Diff(tt.want, titles); diff != "" {
			t.Errorf("%s (-want +got):\n%s", tt.query, diff)
		}
	}
	if got, _ := alice.errorCode(http.MethodGet, "/api/contents?type=post&status=gone", nil); got != http.StatusUnprocessableEntity {
		t.Errorf("bad status = %d, want 422", got)
	}

	// Build records are per site.
	for _, s := range []struct {
		tenant  uuid.UUID
		warning string
	}{{e.tenantA, "Aの警告"}, {e.tenantB, "Bの警告"}} {
		if _, err := e.db.Admin.Exec(context.Background(),
			`INSERT INTO site_builds (tenant_id, started_at, files, changed, warnings) VALUES ($1, now(), 10, 2, jsonb_build_array($2::text))`,
			s.tenant, s.warning); err != nil {
			t.Fatal(err)
		}
	}
	var builds []struct {
		Changed  int64
		Warnings []string
	}
	alice.mustDo(http.MethodGet, "/api/site/builds", nil, &builds, http.StatusOK)
	if len(builds) != 1 || builds[0].Changed != 2 || len(builds[0].Warnings) != 1 || builds[0].Warnings[0] != "Aの警告" {
		t.Errorf("A's builds = %+v", builds)
	}
}

func TestThemeSettings(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	var s map[string]any
	alice.mustDo(http.MethodGet, "/api/site", nil, &s, http.StatusOK)
	themes, _ := s["themes"].([]any)
	if len(themes) == 0 {
		t.Fatal("no themes listed")
	}

	put := func(c *client, ts map[string]any) (int, string) {
		var out struct {
			Error struct{ Code, Field string } `json:"error"`
		}
		got := c.do(http.MethodPut, "/api/site", map[string]any{"name": "サイト", "themeSettings": map[string]any{"corporate": ts}}, &out)
		return got, out.Error.Field
	}
	if got, field := put(alice, map[string]any{"phone": "03-1234-5678", "address": "東京都"}); got != http.StatusOK {
		t.Fatalf("valid theme settings = %d %s", got, field)
	}
	alice.mustDo(http.MethodGet, "/api/site", nil, &s, http.StatusOK)
	ts, _ := s["themeSettings"].(map[string]any)
	if corp, _ := ts["corporate"].(map[string]any); corp["phone"] != "03-1234-5678" {
		t.Errorf("stored theme settings = %v", ts)
	}
	if got, field := put(alice, map[string]any{"phone": "call me"}); got != http.StatusUnprocessableEntity || field != "themeSettings.phone" {
		t.Errorf("bad phone = %d %s, want 422 on themeSettings.phone", got, field)
	}
	if got, field := put(alice, map[string]any{"favoriteColor": "red"}); got != http.StatusUnprocessableEntity || field != "themeSettings.favoriteColor" {
		t.Errorf("undeclared key = %d %s, want 422", got, field)
	}
	var unknown struct {
		Error struct{ Field string } `json:"error"`
	}
	if got := alice.do(http.MethodPut, "/api/site", map[string]any{"name": "サイト", "themeSettings": map[string]any{"nope": map[string]any{}}}, &unknown); got != http.StatusUnprocessableEntity {
		t.Errorf("unknown theme = %d, want 422", got)
	}
	// B's site keeps its own (empty) values.
	var bs map[string]any
	bob.mustDo(http.MethodGet, "/api/site", nil, &bs, http.StatusOK)
	if bs["themeSettings"] != nil {
		t.Errorf("site B sees theme settings %v", bs["themeSettings"])
	}
}

func TestSetPublishedAt(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")
	d := alice.newContent("post", "夏祭りに出店しました", true)
	at := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	var out struct{ PublishedAt *time.Time }
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", d.ID), map[string]any{"publishedAt": at}, &out, http.StatusOK)
	if out.PublishedAt == nil || !out.PublishedAt.Equal(at) {
		t.Errorf("publishedAt = %v, want %v", out.PublishedAt, at)
	}
	if got, code := alice.errorCode(http.MethodPatch, fmt.Sprintf("/api/contents/%d", d.ID), map[string]any{"publishedAt": time.Now().Add(48 * time.Hour)}); got != http.StatusUnprocessableEntity || code != "invalid_published_at" {
		t.Errorf("future publishedAt = %d %s, want 422 invalid_published_at", got, code)
	}
	if got, _ := bob.errorCode(http.MethodPatch, fmt.Sprintf("/api/contents/%d", d.ID), map[string]any{"publishedAt": at}); got != http.StatusNotFound {
		t.Errorf("B changing A's date = %d, want 404", got)
	}
}
