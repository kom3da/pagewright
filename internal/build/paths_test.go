package build

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFileForPath(t *testing.T) {
	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"/", "index.html", true},
		{"/posts/a/", "posts/a/index.html", true},
		{"/about", "about/index.html", true},
		{"/old.html", "old.html", true},
		{"/沿革/", "沿革/index.html", true},
		{"/é/", "é/index.html", true}, // NFD becomes NFC
		{"relative/", "", false},
		{"/a/../b/", "", false},
		{"/./", "", false},
		{"/a//b/", "", false},
		{"/a\\b/", "", false},
		{"/a\nb/", "", false},
		{"/a\x00/", "", false},
	}
	for _, tt := range tests {
		got, ok := fileForPath(tt.path)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("fileForPath(%q) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestPagePath(t *testing.T) {
	got := []string{pagePath("/posts/", 1), pagePath("/posts/", 2), pagePath("/news", 3)}
	want := []string{"/posts/", "/posts/page/2/", "/news/page/3/"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("pagePath (-want +got):\n%s", diff)
	}
}

func TestNginxQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/a/", `"/a/"`},
		{`/a"; return 200 "x`, `"/a\"; return 200 \"x"`},
		{`/a\b`, `"/a\\b"`},
		{"/a b{}/", `"/a b{}/"`},
	}
	for _, tt := range tests {
		if got := nginxQuote(tt.in); got != tt.want {
			t.Errorf("nginxQuote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("あいう\n えお", 4); got != "あいう …" {
		t.Errorf("excerpt = %q", got)
	}
	if got := excerpt("短い", 10); got != "短い" {
		t.Errorf("excerpt = %q", got)
	}
}
