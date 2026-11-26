package main

import (
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/build"
)

// previewHandler serves a built site for development, standing in for
// nginx. It shows the site named by PAGEWRIGHT_PREVIEW_SITE, or the only
// site that has been built; with several, it lists them.
func previewHandler(siteDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := os.Getenv("PAGEWRIGHT_PREVIEW_SITE")
		if id == "" {
			built := builtSites(siteDir)
			if len(built) != 1 {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				var b strings.Builder
				b.WriteString(`<!doctype html><html lang="ja"><meta charset="utf-8"><title>プレビュー</title><main>`)
				if len(built) == 0 {
					b.WriteString(`<p>まだ公開されたサイトがありません。管理画面で記事を公開するか「サイトを更新」を押してください。</p>`)
				} else {
					b.WriteString(`<p>サイトが複数あります。環境変数 PAGEWRIGHT_PREVIEW_SITE に表示するサイトのIDを指定してください。</p><ul>`)
					for _, s := range built {
						b.WriteString("<li><code>" + html.EscapeString(s) + "</code></li>")
					}
					b.WriteString("</ul>")
				}
				b.WriteString("</main></html>")
				_, _ = w.Write([]byte(b.String()))
				return
			}
			id = built[0]
		}
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "PAGEWRIGHT_PREVIEW_SITE はサイトのIDで指定してください", http.StatusInternalServerError)
			return
		}
		build.DirHandler(filepath.Join(siteDir, id)).ServeHTTP(w, r)
	})
}

func builtSites(siteDir string) []string {
	entries, err := os.ReadDir(siteDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if _, err := uuid.Parse(e.Name()); err == nil && e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}
