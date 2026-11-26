package build

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// ServeDir serves a built site (OutDir/<tenantID>) for development, in
// place of nginx. Unknown paths get 404.html with status 404. Redirects
// work through the meta-refresh pages; the nginx include is not read.
func ServeDir(addr, dir string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           DirHandler(dir),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("serving built site", "addr", addr, "dir", dir)
	return srv.ListenAndServe()
}

// DirHandler is the handler behind ServeDir.
func DirHandler(dir string) http.Handler {
	root := os.DirFS(dir)
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Files are stored under NFC names; browsers may send either form.
		p := norm.NFC.String(path.Clean("/" + r.URL.Path))
		if strings.HasSuffix(r.URL.Path, "/") && p != "/" {
			p += "/"
		}
		name := strings.TrimPrefix(p, "/")
		if name == "" || strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		if strings.HasPrefix(path.Base(name), ".") || strings.HasPrefix(name, "_redirects") {
			notFound(w, dir)
			return
		}
		info, err := fs.Stat(root, strings.TrimSuffix(name, "/"))
		if err == nil && info.IsDir() {
			_, err = fs.Stat(root, name+"/index.html")
		}
		if err != nil {
			notFound(w, dir)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = p
		files.ServeHTTP(w, r2)
	})
}

func notFound(w http.ResponseWriter, dir string) {
	b, err := os.ReadFile(filepath.Join(dir, "404.html"))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("read 404.html", "err", err)
		}
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(b)
}
