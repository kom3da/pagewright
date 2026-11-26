// Package adminui serves the built admin UI (web/admin/dist) under /admin/.
package adminui

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// Handler serves fsys under /admin/. Unknown paths get index.html so the
// client-side router can handle them. A nil fsys serves a short notice.
func Handler(fsys fs.FS) http.Handler {
	if fsys == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<!doctype html><html lang="ja"><meta charset="utf-8"><title>Pagewright</title>` +
				`<p>管理画面がまだビルドされていません。<code>mise run admin:build</code> を実行してください。</p></html>`))
		})
	}
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/admin")
		name = strings.TrimPrefix(name, "/")
		if name != "" {
			if st, err := fs.Stat(fsys, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + name
				files.ServeHTTP(w, r2)
				return
			}
		}
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

// Dir returns the built admin UI at dir, or nil if it has not been built.
func Dir(dir string) fs.FS {
	if _, err := os.Stat(path.Join(dir, "index.html")); err != nil {
		return nil
	}
	return os.DirFS(dir)
}
