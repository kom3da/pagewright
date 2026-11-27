package site

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// PublicURL is a site's public origin: its own (tenants.public_base_url)
// when set, else fallback (PAGEWRIGHT_PUBLIC_BASE_URL). No trailing slash.
func PublicURL(own *string, fallback string) string {
	if own != nil && *own != "" {
		return *own
	}
	return strings.TrimSuffix(fallback, "/")
}

// hostRE is the host part allowed by the tenants.public_base_url CHECK.
var hostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// ParsePublicURL checks a site's public URL as the operator types it and
// returns it normalized for tenants.public_base_url: scheme://host[:port]
// in lower case, a Japanese domain in its ASCII (punycode) form, no
// trailing slash.
func ParsePublicURL(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "/")
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("http:// か https:// で始まるURLを指定してください（例: https://www.example.jp）")
	}
	if u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", errors.New("URLにはドメインまで（https://www.example.jp のように）を指定してください。/ 以降のパスや ? 以降は付けられません")
	}
	host, err := idna.Lookup.ToASCII(u.Hostname())
	if err != nil || !hostRE.MatchString(host) {
		return "", errors.New("URLのドメインが正しくありません")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("URLのポート番号が正しくありません")
		}
		host += ":" + strconv.Itoa(n)
	} else if strings.HasSuffix(u.Host, ":") {
		return "", errors.New("URLのポート番号が正しくありません")
	}
	return u.Scheme + "://" + host, nil
}
