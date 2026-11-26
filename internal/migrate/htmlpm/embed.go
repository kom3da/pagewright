package htmlpm

import (
	"net/url"
	"strings"
)

// Embed providers of the embed node.
const (
	ProviderYouTube   = "youtube"
	ProviderGoogleMap = "googlemap"
)

// provider tells which embed provider serves rawURL. Only http(s) URLs of
// YouTube and Google Maps qualify; anything else stays out of embed nodes.
func provider(rawURL string) (string, bool) {
	if strings.HasPrefix(rawURL, "//") {
		rawURL = "https:" + rawURL
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	switch host {
	case "youtube.com", "m.youtube.com", "youtu.be", "youtube-nocookie.com":
		return ProviderYouTube, true
	case "maps.google.com", "maps.google.co.jp", "maps.app.goo.gl":
		return ProviderGoogleMap, true
	case "google.com", "google.co.jp":
		if strings.HasPrefix(u.Path, "/maps") {
			return ProviderGoogleMap, true
		}
	case "goo.gl":
		if strings.HasPrefix(u.Path, "/maps") {
			return ProviderGoogleMap, true
		}
	}
	return "", false
}
