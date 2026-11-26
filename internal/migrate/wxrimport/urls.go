package wxrimport

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	percentRe = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
	digitsRe  = regexp.MustCompile(`^[0-9]+$`)
)

// validPath mirrors the CHECK on routes.path, so a bad path is reported
// instead of aborting the transaction.
func validPath(p string) bool {
	return strings.HasPrefix(p, "/") &&
		!strings.Contains(p, "//") &&
		!strings.ContainsAny(p, "?#") &&
		!percentRe.MatchString(p) &&
		norm.NFC.IsNormalString(p)
}

// validSlug mirrors the CHECK on contents.slug.
func validSlug(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !digitsRe.MatchString(s)
}

var errQueryURL = errors.New("query URL")

// oldPath returns the decoded, NFC-normalized path of an old URL.
// URLs such as https://example.jp/?p=123 return errQueryURL: a static site
// cannot serve them, and routes cannot hold a query.
func oldPath(link string) (string, error) {
	if link == "" {
		return "", errors.New("empty URL")
	}
	u, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	if u.RawQuery != "" {
		return "", errQueryURL
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	p = norm.NFC.String(p)
	if !validPath(p) {
		return "", errors.New("invalid path " + strconv.Quote(p))
	}
	return p, nil
}

// sitePrefix is the path of the site URL without a trailing slash: "" for
// https://example.jp, "/blog" for https://example.jp/blog/.
func sitePrefix(siteURL string) string {
	u, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}
	return strings.TrimRight(norm.NFC.String(u.Path), "/")
}

// pathOf joins segments into "/a/b/".
func pathOf(segs ...string) string {
	return "/" + strings.Join(segs, "/") + "/"
}
