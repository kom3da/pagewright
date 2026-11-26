package build_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kom3da/pagewright/internal/build"
)

func TestDirHandler(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"index.html":       "home",
		"404.html":         "not found page",
		"沿革/index.html":    "history",
		"feed.xml":         "<rss/>",
		"_redirects.nginx": "location",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := build.DirHandler(dir)
	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/", http.StatusOK, "home"},
		{"/%E6%B2%BF%E9%9D%A9/", http.StatusOK, "history"},
		{"/feed.xml", http.StatusOK, "<rss/>"},
		{"/nope/", http.StatusNotFound, "not found page"},
		{"/_redirects.nginx", http.StatusNotFound, "not found page"},
		{"/../etc/passwd", http.StatusNotFound, "not found page"},
		{"/%E6%B2%BF%E9%9D%A9", http.StatusMovedPermanently, ""},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantBody) {
			t.Errorf("GET %s = %d %q, want %d %q", tt.path, rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
		}
	}
}
