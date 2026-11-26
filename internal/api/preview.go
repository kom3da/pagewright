package api

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbgen"
)

const (
	defaultPreviewDays = 3
	maxPreviewDays     = 30
)

type previewLinkJSON struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url,omitempty"` // only when created: the token is not stored
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) previewURL(token string) string {
	return s.BaseURL + "/p/" + token + "/"
}

func (s *Server) createPreviewLink(w http.ResponseWriter, r *http.Request, sess *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Days *int `json:"days"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	days := defaultPreviewDays
	if in.Days != nil {
		days = *in.Days
	}
	if days < 1 || days > maxPreviewDays {
		return invalid("invalid_days", fmt.Sprintf("有効期間は1日から%d日のあいだで指定してください", maxPreviewDays))
	}
	token := newToken()
	expires := time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC().Truncate(time.Second)
	var out previewLinkJSON
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		if _, err := q.GetContent(r.Context(), id); err != nil {
			return err
		}
		row, err := q.CreatePreviewLink(r.Context(), dbgen.CreatePreviewLinkParams{
			TokenHash: hashToken(token), ContentID: id, ExpiresAt: expires, CreatedBy: &sess.userID,
		})
		out = previewLinkJSON{ID: row.ID, URL: s.previewURL(token), ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

func (s *Server) listPreviewLinks(w http.ResponseWriter, r *http.Request, _ *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	out := []previewLinkJSON{}
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		if _, err := q.GetContent(r.Context(), id); err != nil {
			return err
		}
		rows, err := q.ListPreviewLinks(r.Context(), id)
		for _, l := range rows {
			out = append(out, previewLinkJSON{ID: l.ID, ExpiresAt: l.ExpiresAt, CreatedAt: l.CreatedAt})
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) deletePreviewLink(w http.ResponseWriter, r *http.Request, _ *session) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	err = s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		n, err := q.DeletePreviewLink(r.Context(), id)
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

// PreviewHandler serves preview links without login at /p/{token}/: the
// page, the theme's assets and the site's images.
func (s *Server) PreviewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{token}/{$}", s.previewPage)
	mux.HandleFunc("GET /p/{token}/_assets/{name}", s.previewAsset)
	mux.HandleFunc("GET /p/{token}/media/{key...}", s.previewMedia)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The token is in the URL: keep it out of logs elsewhere and of
		// search engines.
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Previews show the tenant's own body and theme on the admin's
		// origin: sandbox them so nothing in a page can act with the
		// admin's cookies, and allow no scripts at all.
		w.Header().Set("Content-Security-Policy", "sandbox; script-src 'none'; object-src 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) lookupPreview(r *http.Request) (dbgen.LookupPreviewLinkRow, bool) {
	row, err := s.DB.Queries().LookupPreviewLink(r.Context(), hashToken(r.PathValue("token")))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.Logger.Error("preview lookup", "err", err)
		}
		return row, false
	}
	return row, true
}

func (s *Server) previewPage(w http.ResponseWriter, r *http.Request) {
	link, ok := s.lookupPreview(r)
	if !ok {
		s.formPage(w, http.StatusNotFound, "", "プレビューが見つかりません", []string{"このプレビュー用のリンクは、有効期限が切れたか、取り消されました。"}, "")
		return
	}
	jst := time.FixedZone("JST", 9*60*60)
	page, err := build.RenderPreview(r.Context(), s.DB, link.TenantID, link.ContentID, build.PreviewOptions{
		PublicBaseURL: s.PublicBaseURL,
		SelfURL:       s.previewURL(r.PathValue("token")),
		FormBaseURL:   s.BaseURL,
		Notice:        "これはプレビューです。公開サイトにはまだ出ていません（このリンクは" + link.ExpiresAt.In(jst).Format("2006年1月2日 15:04") + "まで有効）",
	})
	if err != nil {
		s.Logger.Error("preview render", "tenant_id", link.TenantID, "content_id", link.ContentID, "err", err)
		s.formPage(w, http.StatusInternalServerError, "", "プレビューを表示できませんでした", []string{"時間をおいてもう一度お試しください。"}, "")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (s *Server) previewAsset(w http.ResponseWriter, r *http.Request) {
	link, ok := s.lookupPreview(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	b, ok := build.ThemeAsset(r.Context(), s.DB, link.TenantID, name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeOf(name))
	_, _ = w.Write(b)
}

func (s *Server) previewMedia(w http.ResponseWriter, r *http.Request) {
	link, ok := s.lookupPreview(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := s.Media.Open(link.TenantID, r.PathValue("key"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", contentTypeOf(r.PathValue("key")))
	http.ServeContent(w, r, "", time.Time{}, f)
}

func contentTypeOf(name string) string {
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}
