package site

import "testing"

func TestParsePublicURL(t *testing.T) {
	ok := map[string]string{
		"https://www.example.jp":      "https://www.example.jp",
		" https://WWW.Example.JP/ ":   "https://www.example.jp",
		"HTTP://localhost:8081":       "http://localhost:8081",
		"https://example.jp:08443":    "https://example.jp:8443",
		"https://日本語.jp":              "https://xn--wgv71a119e.jp",
		"https://site-b.example.com/": "https://site-b.example.com",
	}
	for in, want := range ok {
		got, err := ParsePublicURL(in)
		if err != nil || got != want {
			t.Errorf("ParsePublicURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "www.example.jp", "ftp://example.jp", "https://", "https://example.jp/blog",
		"https://example.jp?x=1", "https://example.jp#top", "https://user@example.jp",
		"https://example.jp:", "https://example.jp:0", "https://example.jp:65536", "https://exa mple.jp",
		"https://-example.jp", "https://[::1]", "javascript:alert(1)", "https://example.jp/?",
	} {
		if got, err := ParsePublicURL(in); err == nil {
			t.Errorf("ParsePublicURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestPublicURL(t *testing.T) {
	own, empty := "https://b.example", ""
	if got := PublicURL(&own, "https://a.example/"); got != own {
		t.Errorf("own = %q, want %q", got, own)
	}
	for _, p := range []*string{nil, &empty} {
		if got := PublicURL(p, "https://a.example/"); got != "https://a.example" {
			t.Errorf("fallback = %q, want https://a.example", got)
		}
	}
}
