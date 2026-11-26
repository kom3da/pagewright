package render

import (
	"net/url"
	"regexp"
	"strings"
)

// embed writes an iframe from a fixed template. Only the video ID or map
// query is taken from the stored URL, and only after validation; the
// stored URL itself never reaches the page.
func (r *renderer) embed(n node) {
	provider, _ := n.Attrs["provider"].(string)
	raw, _ := n.Attrs["url"].(string)
	switch provider {
	case "youtube":
		id, ok := YouTubeID(raw)
		if !ok {
			r.warn("youtube embed with unrecognized url dropped")
			return
		}
		r.w(`<div class="embed embed-youtube"><iframe src="https://www.youtube-nocookie.com/embed/`, id,
			`" title="YouTube動画" width="560" height="315" loading="lazy" referrerpolicy="strict-origin-when-cross-origin"`,
			` allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe></div>`, "\n")
	case "googlemap":
		src, ok := GoogleMapEmbedURL(raw)
		if !ok {
			r.warn("googlemap embed with unrecognized url dropped")
			return
		}
		r.w(`<div class="embed embed-map"><iframe src="`, esc(src),
			`" title="Googleマップ" width="600" height="450" loading="lazy" referrerpolicy="no-referrer-when-downgrade"></iframe></div>`, "\n")
	default:
		r.warn("embed provider %q dropped", provider)
	}
}

var youTubeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// YouTubeID extracts the video ID from the usual YouTube URL forms
// (watch?v=, youtu.be/, embed/, shorts/, live/).
func YouTubeID(raw string) (string, bool) {
	u, ok := parseWebURL(raw)
	if !ok {
		return "", false
	}
	host := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(u.Hostname()), "www."), "m.")
	var id string
	switch host {
	case "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	case "youtube.com", "youtube-nocookie.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
			break
		}
		for _, p := range []string{"/embed/", "/shorts/", "/live/"} {
			if rest, ok := strings.CutPrefix(u.Path, p); ok {
				id = strings.TrimSuffix(rest, "/")
			}
		}
	}
	if !youTubeIDRe.MatchString(id) {
		return "", false
	}
	return id, true
}

var googleHosts = map[string]bool{
	"google.com": true, "www.google.com": true, "maps.google.com": true,
	"google.co.jp": true, "www.google.co.jp": true, "maps.google.co.jp": true,
}

// GoogleMapEmbedURL turns a Google Maps URL into an embed URL on
// www.google.com. Accepted: the "embed a map" URL (/maps/embed?pb=...) and
// a search URL (/maps?q=...).
func GoogleMapEmbedURL(raw string) (string, bool) {
	u, ok := parseWebURL(raw)
	if !ok || !googleHosts[strings.ToLower(u.Hostname())] {
		return "", false
	}
	q := u.Query()
	switch strings.TrimSuffix(u.Path, "/") {
	case "/maps/embed":
		pb := q.Get("pb")
		if pb == "" || len(pb) > 4096 {
			return "", false
		}
		return "https://www.google.com/maps/embed?" + url.Values{"pb": {pb}}.Encode(), true
	case "/maps", "":
		query := q.Get("q")
		if strings.TrimSpace(query) == "" || len(query) > 1024 {
			return "", false
		}
		return "https://www.google.com/maps?" + url.Values{"output": {"embed"}, "q": {query}}.Encode(), true
	}
	return "", false
}

func parseWebURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, false
	}
	return u, true
}
