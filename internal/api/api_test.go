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
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/api"
	"github.com/kom3da/pagewright/internal/db/dbtest"
	"github.com/kom3da/pagewright/internal/mail"
	"github.com/kom3da/pagewright/internal/media"
)

type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) last() (mail.Message, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return mail.Message{}, false
	}
	return f.sent[len(f.sent)-1], true
}

type fakeBuilds struct {
	mu      sync.Mutex
	tenants []uuid.UUID
}

func (f *fakeBuilds) EnqueueBuild(_ context.Context, _ pgx.Tx, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tenants = append(f.tenants, id)
	return nil
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
	db      *dbtest.DB
	srv     *httptest.Server
	mail    *fakeMail
	builds  *fakeBuilds
	tenantA uuid.UUID
	tenantB uuid.UUID
}

// newEnv starts the API on a fresh database with two sites:
// alice (A admin), dave (A editor), bob (B admin), carol (editor of both).
func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.New(t)
	e := &env{t: t, db: d, mail: &fakeMail{}, builds: &fakeBuilds{}, tenantA: uuid.New(), tenantB: uuid.New()}
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
	s := api.New(api.Options{
		DB: d.App, Mail: e.mail, Media: media.Store{Root: t.TempDir()}, Builds: e.builds,
		BaseURL: "http://admin.test",
	})
	e.srv = httptest.NewServer(s.Handler())
	t.Cleanup(e.srv.Close)
	return e
}

// client is one browser.
type client struct {
	e    *env
	http *http.Client
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar}}
}

var tokenRE = regexp.MustCompile(`token=([A-Za-z0-9_%-]+)`)

// login signs in through the mailed link.
func (e *env) login(email string) *client {
	e.t.Helper()
	c := e.anon()
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
	ParentID              *int64
	CategoryIDs           []int64
	TagIDs                []int64
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

	// Unknown addresses get the same answer and no mail.
	anon.mustDo(http.MethodPost, "/api/auth/login-link", map[string]string{"email": "nobody@example.com"}, nil, http.StatusNoContent)
	if _, sent := e.mail.last(); sent {
		t.Error("mail sent to an unknown address")
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

	// A new draft leaves the published version and shows pending changes.
	alice.mustDo(http.MethodPut, fmt.Sprintf("/api/contents/%d/draft", d.ID),
		map[string]any{"title": "お知らせ（改）", "body": json.RawMessage(paragraph)}, &d, http.StatusOK)
	if !d.HasUnpublishedChanges || d.Status != "published" {
		t.Errorf("after new draft = %+v, want published with unpublished changes", d)
	}

	// Changing the URL keeps the old one as a redirect.
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", d.ID), map[string]any{"slug": "news-1"}, &d, http.StatusOK)
	if d.URL == nil || *d.URL != "/posts/news-1/" {
		t.Errorf("url after slug change = %v, want /posts/news-1/", d.URL)
	}
	var kind string
	if err := e.db.Admin.QueryRow(context.Background(), "SELECT kind FROM routes WHERE path = $1", wantURL).Scan(&kind); err != nil || kind != "redirect" {
		t.Errorf("old url %s: kind=%q err=%v, want redirect", wantURL, kind, err)
	}

	// Pages nest under their parent's URL and follow its changes.
	parent := alice.newContent("page", "会社概要", true)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", parent.ID), map[string]any{"slug": "company"}, &parent, http.StatusOK)
	child := alice.newContent("page", "アクセス", false)
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", child.ID), map[string]any{"slug": "access", "parentId": parent.ID}, &child, http.StatusOK)
	alice.mustDo(http.MethodPost, fmt.Sprintf("/api/contents/%d/publish", child.ID), nil, &child, http.StatusOK)
	if child.URL == nil || *child.URL != "/company/access/" {
		t.Errorf("child url = %v, want /company/access/", child.URL)
	}
	alice.mustDo(http.MethodPatch, fmt.Sprintf("/api/contents/%d", parent.ID), map[string]any{"slug": "about"}, nil, http.StatusOK)
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

func TestValidation(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	dave := e.login("dave@example.com")
	d := alice.newContent("post", "x", false)
	path := fmt.Sprintf("/api/contents/%d", d.ID)

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
		{"category slug", alice, http.MethodPost, "/api/categories", map[string]any{"name": "x", "slug": "カテゴリ"}, 422, "invalid_slug"},
		{"bad email", alice, http.MethodPost, "/api/auth/login-link", map[string]any{"email": "not an email"}, 422, "invalid_email"},
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

// TestTenantIsolation checks every entry point against another site's data.
func TestTenantIsolation(t *testing.T) {
	e := newEnv(t)
	alice := e.login("alice@example.com")
	bob := e.login("bob@example.com")

	// Bob's site B data.
	bPost := bob.newContent("post", "Bの記事", true)
	var bCat, bTag struct{ ID int64 }
	bob.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "B", "slug": "b-cat"}, &bCat, http.StatusCreated)
	bob.mustDo(http.MethodPost, "/api/tags", map[string]any{"name": "B", "slug": "b-tag"}, &bTag, http.StatusCreated)
	var bMedia mediaOut
	if got := bob.upload("b.png", pngBytes(t), "B", &bMedia); got != http.StatusCreated {
		t.Fatalf("bob upload = %d", got)
	}

	// Alice's site A data.
	aPost := alice.newContent("post", "Aの記事", true)
	var aCat struct{ ID int64 }
	alice.mustDo(http.MethodPost, "/api/categories", map[string]any{"name": "A", "slug": "a-cat"}, &aCat, http.StatusCreated)

	// Lists show only A.
	lists := map[string]int{"/api/contents?type=post": 1, "/api/categories": 1, "/api/tags": 0, "/api/media": 0}
	for path, want := range lists {
		var got []json.RawMessage
		alice.mustDo(http.MethodGet, path, nil, &got, http.StatusOK)
		if len(got) != want {
			t.Errorf("GET %s from A: %d items, want %d", path, len(got), want)
		}
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
		{http.MethodDelete, fmt.Sprintf("/api/contents/%d", bPost.ID), nil},
		{http.MethodPatch, fmt.Sprintf("/api/categories/%d", bCat.ID), map[string]any{"name": "x"}},
		{http.MethodDelete, fmt.Sprintf("/api/categories/%d", bCat.ID), nil},
		{http.MethodDelete, fmt.Sprintf("/api/tags/%d", bTag.ID), nil},
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
	if got, _ := alice.errorCode(http.MethodPost, "/api/categories", map[string]any{"name": "x", "slug": "x", "parentId": bCat.ID}); got != http.StatusUnprocessableEntity {
		t.Errorf("category under B's category = %d, want 422", got)
	}

	// B is untouched.
	var after detail
	bob.mustDo(http.MethodGet, fmt.Sprintf("/api/contents/%d", bPost.ID), nil, &after, http.StatusOK)
	if diff := cmp.Diff(bPost, after); diff != "" {
		t.Errorf("B's content changed (-before +after):\n%s", diff)
	}
	var bCats []json.RawMessage
	bob.mustDo(http.MethodGet, "/api/categories", nil, &bCats, http.StatusOK)
	if len(bCats) != 1 {
		t.Errorf("B has %d categories, want 1", len(bCats))
	}
}
