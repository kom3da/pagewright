// Package api serves the admin JSON API (docs/dev/admin-api.md).
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kom3da/pagewright/internal/body"
	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/mail"
	"github.com/kom3da/pagewright/internal/media"
)

// BuildQueue schedules a full build of a tenant's site. The job is added
// in tx so it commits together with the change that needs it.
type BuildQueue interface {
	EnqueueBuild(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) error
}

// Options configures a Server.
type Options struct {
	DB            *db.DB
	Mail          mail.Sender
	Media         media.Store
	Builds        BuildQueue // nil: changes do not trigger builds
	BaseURL       string     // admin origin, for login links
	SecureCookies bool
	Logger        *slog.Logger
}

// Server is the admin API.
type Server struct {
	Options
}

const (
	sessionCookie = "pw_session"
	loginTTL      = 15 * time.Minute
	sessionTTL    = 30 * 24 * time.Hour
	maxJSONBytes  = 4 << 20
)

// New returns a Server.
func New(o Options) *Server {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Server{Options: o}
}

// Handler returns the routes under /api/.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/auth/login-link", s.public(s.loginLink))
	mux.Handle("POST /api/auth/verify", s.public(s.verify))
	mux.Handle("POST /api/auth/logout", s.authed(s.logout))
	mux.Handle("GET /api/me", s.authed(s.me))
	mux.Handle("POST /api/me/site", s.authed(s.selectSite))

	mux.Handle("GET /api/contents", s.site(s.listContents))
	mux.Handle("POST /api/contents", s.site(s.createContent))
	mux.Handle("GET /api/contents/{id}", s.site(s.getContent))
	mux.Handle("PUT /api/contents/{id}/draft", s.site(s.saveDraft))
	mux.Handle("PATCH /api/contents/{id}", s.site(s.patchContent))
	mux.Handle("POST /api/contents/{id}/publish", s.site(s.publish))
	mux.Handle("POST /api/contents/{id}/unpublish", s.site(s.unpublish))
	mux.Handle("DELETE /api/contents/{id}", s.site(s.deleteContent))

	mux.Handle("GET /api/categories", s.site(s.listCategories))
	mux.Handle("POST /api/categories", s.site(s.createCategory))
	mux.Handle("PATCH /api/categories/{id}", s.site(s.patchCategory))
	mux.Handle("DELETE /api/categories/{id}", s.site(s.deleteCategory))
	mux.Handle("GET /api/tags", s.site(s.listTags))
	mux.Handle("POST /api/tags", s.site(s.createTag))
	mux.Handle("DELETE /api/tags/{id}", s.site(s.deleteTag))

	mux.Handle("GET /api/media", s.site(s.listMedia))
	mux.Handle("POST /api/media", s.site(s.uploadMedia))
	mux.Handle("GET /api/media/{id}/file", s.site(s.mediaFile))
	mux.Handle("PATCH /api/media/{id}", s.site(s.patchMedia))

	mux.Handle("POST /api/site/build", s.site(s.buildSite))

	mux.Handle("/api/", s.public(func(http.ResponseWriter, *http.Request, *session) error {
		return errNotFound
	}))
	return s.recoverer(csrf(mux))
}

// session is the logged-in user of a request.
type session struct {
	tokenHash   []byte
	userID      uuid.UUID
	email       string
	displayName string
	tenantID    uuid.UUID // uuid.Nil until a site is chosen
	role        string    // "admin" or "editor"; empty until a site is chosen
}

type handler func(w http.ResponseWriter, r *http.Request, sess *session) error

// public serves a handler that needs no login.
func (s *Server) public(h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.finish(w, r, h(w, r, nil))
	})
}

// authed serves a handler that needs a login.
func (s *Server) authed(h handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.loadSession(r)
		if err != nil {
			s.finish(w, r, err)
			return
		}
		s.finish(w, r, h(w, r, sess))
	})
}

// site serves a handler that needs a login and a chosen site. The request
// context carries the tenant, so db.InTenant scopes every query to it.
func (s *Server) site(h handler) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, sess *session) error {
		if sess.tenantID == uuid.Nil {
			return &apiError{http.StatusForbidden, "no_site", "サイトを選んでください"}
		}
		return h(w, r.WithContext(db.WithTenant(r.Context(), sess.tenantID)), sess)
	})
}

func (s *Server) loadSession(r *http.Request) (*session, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, errUnauthenticated
	}
	return s.sessionByHash(r.Context(), hashToken(c.Value))
}

func (s *Server) sessionByHash(ctx context.Context, h []byte) (*session, error) {
	row, err := s.DB.Queries().AuthSession(ctx, h)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	return &session{
		tokenHash: h, userID: row.UserID, email: row.Email, displayName: row.DisplayName,
		tenantID: row.TenantID, role: row.Role,
	}, nil
}

// csrf requires the X-Pagewright header on state-changing requests. A
// cross-site form or simple request cannot set it.
func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Pagewright") != "1" {
			writeJSON(w, http.StatusForbidden, errorBody("csrf", "画面を再読み込みしてから、もう一度お試しください"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Logger.Error("panic in handler", "method", r.Method, "path", r.URL.Path, "panic", v)
				writeJSON(w, http.StatusInternalServerError, errorBody("internal", "サーバーでエラーが起きました"))
			}
		}()
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// apiError is an error with an HTTP status and a message for the admin UI.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

var (
	errUnauthenticated = &apiError{http.StatusUnauthorized, "unauthenticated", "ログインしてください"}
	errNotFound        = &apiError{http.StatusNotFound, "not_found", "見つかりません"}
	errURLTaken        = &apiError{http.StatusConflict, "conflict", "このURLはすでに使われています。別のURLにしてください"}
)

func invalid(code, msg string) *apiError {
	return &apiError{http.StatusUnprocessableEntity, code, msg}
}

// finish writes err as a JSON error response, if any.
func (s *Server) finish(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	var ae *apiError
	var be *body.Error
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &ae):
	case errors.As(err, &be):
		ae = invalid("invalid_body", be.Message)
	case errors.Is(err, pgx.ErrNoRows):
		ae = errNotFound
	case errors.As(err, &pe) && pe.Code == "23505":
		ae = errURLTaken
	case errors.As(err, &pe) && pe.Code == "23503":
		ae = invalid("invalid_reference", "指定したものが見つかりません")
	case errors.As(err, &pe) && pe.Code == "23514":
		ae = invalid("invalid_value", "入力された値が正しくありません")
	default:
		s.Logger.Error("api error", "method", r.Method, "path", r.URL.Path, "err", err)
		ae = &apiError{http.StatusInternalServerError, "internal", "サーバーでエラーが起きました。時間をおいてもう一度お試しください"}
	}
	writeJSON(w, ae.Status, errorBody(ae.Code, ae.Message))
}

func errorBody(code, msg string) any {
	return map[string]any{"error": map[string]string{"code": code, "message": msg}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads a JSON request body into v, rejecting unknown fields.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return invalid("invalid_json", "送信された内容を読み取れませんでした")
	}
	if _, err := dec.Token(); err != io.EOF {
		return invalid("invalid_json", "送信された内容を読み取れませんでした")
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

// newToken returns a random URL-safe secret.
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

// enqueueBuild schedules a site build in tx, when a queue is configured.
func (s *Server) enqueueBuild(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) error {
	if s.Builds == nil {
		return nil
	}
	if err := s.Builds.EnqueueBuild(ctx, tx, tenantID); err != nil {
		return fmt.Errorf("enqueue build: %w", err)
	}
	return nil
}
