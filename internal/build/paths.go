package build

import (
	"net/url"
	"path"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// postsBase is where posts live (/posts/{slug}/). Fixed for now; the design
// lets sites rename it later.
const postsBase = "/posts/"

// fileForPath maps a public path to the file that serves it, relative to
// the site root: "/a/" → "a/index.html", "/a.html" → "a.html",
// "/a" → "a/index.html". Paths with control characters, backslashes or
// dot segments are refused, so a stored path can never escape the site
// directory.
func fileForPath(p string) (string, bool) {
	p = norm.NFC.String(p)
	if !strings.HasPrefix(p, "/") {
		return "", false
	}
	rel := strings.TrimPrefix(p, "/")
	switch last := path.Base(p); {
	case rel == "" || strings.HasSuffix(rel, "/"):
		rel += "index.html"
	case !strings.Contains(last, "."):
		rel += "/index.html"
	}
	if !validRel(rel) {
		return "", false
	}
	return rel, true
}

// validRel reports whether rel is a clean relative file path.
func validRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsAny(rel, "\\\x00") ||
		strings.ContainsFunc(rel, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return false
	}
	for seg := range strings.SplitSeq(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// absURL joins the base URL and a decoded path, percent-encoding the path.
func absURL(base, p string) string {
	return base + escapePath(p)
}

// escapePath percent-encodes a decoded path for use in URLs and config.
func escapePath(p string) string {
	return (&url.URL{Path: p}).EscapedPath()
}

// pagePath is the path of page n (1-based) of a list at base.
func pagePath(base string, n int) string {
	if n <= 1 {
		return base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + "page/" + strconv.Itoa(n) + "/"
}

// segment is the URL segment of a content: its slug, or its number.
func segment(slug *string, number int64) string {
	if slug != nil && *slug != "" {
		return *slug
	}
	return strconv.FormatInt(number, 10)
}
