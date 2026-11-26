// Command sampleload fills an empty site with a theme's sample content
// (themes/<theme>/sample/) through the admin API, as any admin client
// would. It is a development tool, not part of the pagewright binary.
//
//	go run ./tools/sampleload -login "<URL printed by pagewright login-url>"
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// sample is site.json.
type sample struct {
	Site struct {
		Name          string                    `json:"name"`
		ThemeSettings map[string]map[string]any `json:"themeSettings"`
	} `json:"site"`
	Images []struct {
		Key, File, Alt string
	} `json:"images"`
	Categories []struct{ Slug, Name string } `json:"categories"`
	Tags       []struct{ Slug, Name string } `json:"tags"`
	Pages      []struct {
		Slug, Title, Parent string
		Front, Privacy      bool
		Body                []block
	} `json:"pages"`
	Posts []struct {
		Slug, Title, Date string
		Categories, Tags  []string
		Body              []block
	} `json:"posts"`
	Menu []struct {
		Page, Label, URL string
	} `json:"menu"`
}

// block is a body block in a compact form: exactly one key is set.
type block map[string]json.RawMessage

type client struct {
	base string
	http *http.Client
}

func main() {
	login := flag.String("login", "", "login URL printed by `pagewright login-url` (one-time)")
	dir := flag.String("dir", "themes/corporate/sample", "sample directory with site.json and images/")
	flag.Parse()
	if err := run(*login, *dir); err != nil {
		fmt.Fprintln(os.Stderr, "sampleload:", err)
		os.Exit(1)
	}
}

func run(login, dir string) error {
	u, err := url.Parse(login)
	if err != nil || u.Query().Get("token") == "" {
		return errors.New("-login must be the URL printed by `pagewright login-url`")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "site.json"))
	if err != nil {
		return err
	}
	var s sample
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("site.json: %w", err)
	}
	jar, _ := cookiejar.New(nil)
	c := &client{base: u.Scheme + "://" + u.Host, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	if err := c.call("POST", "/api/auth/verify", map[string]string{"token": u.Query().Get("token")}, nil); err != nil {
		return fmt.Errorf("log in: %w", err)
	}
	for _, typ := range []string{"post", "page"} {
		var existing []any
		if err := c.call("GET", "/api/contents?type="+typ, nil, &existing); err != nil {
			return err
		}
		if len(existing) > 0 {
			return errors.New("the site already has contents; load samples into an empty site")
		}
	}

	media := map[string]mediaRef{}
	for _, im := range s.Images {
		m, err := c.upload(filepath.Join(dir, "images", im.File), im.Alt)
		if err != nil {
			return fmt.Errorf("image %s: %w", im.File, err)
		}
		media[im.Key] = m
	}
	cats, tags := map[string]int64{}, map[string]int64{}
	for _, x := range s.Categories {
		var out struct{ ID int64 }
		if err := c.call("POST", "/api/categories", map[string]string{"name": x.Name, "slug": x.Slug}, &out); err != nil {
			return err
		}
		cats[x.Slug] = out.ID
	}
	for _, x := range s.Tags {
		var out struct{ ID int64 }
		if err := c.call("POST", "/api/tags", map[string]string{"name": x.Name, "slug": x.Slug}, &out); err != nil {
			return err
		}
		tags[x.Slug] = out.ID
	}

	pages := map[string]int64{}
	var front, privacy int64
	for _, p := range s.Pages {
		body, err := toDoc(p.Body, media)
		if err != nil {
			return fmt.Errorf("page %s: %w", p.Slug, err)
		}
		patch := map[string]any{"slug": p.Slug}
		if p.Parent != "" {
			patch["parentId"] = pages[p.Parent]
		}
		id, err := c.content("page", p.Title, body, patch)
		if err != nil {
			return fmt.Errorf("page %s: %w", p.Slug, err)
		}
		pages[p.Slug] = id
		if p.Front {
			front = id
		}
		if p.Privacy {
			privacy = id
		}
	}
	// Oldest first, so numbers follow the dates.
	for i := len(s.Posts) - 1; i >= 0; i-- {
		p := s.Posts[i]
		body, err := toDoc(p.Body, media)
		if err != nil {
			return fmt.Errorf("post %s: %w", p.Slug, err)
		}
		at, err := time.Parse("2006-01-02", p.Date)
		if err != nil {
			return fmt.Errorf("post %s: date: %w", p.Slug, err)
		}
		patch := map[string]any{"slug": p.Slug, "publishedAt": at.Add(9 * time.Hour), "categoryIds": ids(p.Categories, cats), "tagIds": ids(p.Tags, tags)}
		if _, err := c.content("post", p.Title, body, patch); err != nil {
			return fmt.Errorf("post %s: %w", p.Slug, err)
		}
	}

	var site map[string]any
	if err := c.call("GET", "/api/site", nil, &site); err != nil {
		return err
	}
	for _, k := range []string{"logo", "themes"} {
		delete(site, k)
	}
	site["name"] = s.Site.Name
	site["themeSettings"] = s.Site.ThemeSettings
	site["frontPageId"] = front
	site["privacyPageId"] = privacy
	var menu []map[string]any
	for _, m := range s.Menu {
		if m.Page != "" {
			menu = append(menu, map[string]any{"contentId": pages[m.Page]})
		} else {
			menu = append(menu, map[string]any{"label": m.Label, "url": m.URL})
		}
	}
	site["menu"] = menu
	if err := c.call("PUT", "/api/site", site, nil); err != nil {
		return fmt.Errorf("site settings: %w", err)
	}
	if err := c.call("POST", "/api/site/build", nil, nil); err != nil {
		return err
	}
	fmt.Printf("loaded %d pages and %d posts; the site is being built\n", len(s.Pages), len(s.Posts))
	return nil
}

func ids(slugs []string, m map[string]int64) []int64 {
	out := []int64{}
	for _, s := range slugs {
		out = append(out, m[s])
	}
	return out
}

// content creates, fills, configures and publishes a content.
func (c *client) content(typ, title string, body any, patch map[string]any) (int64, error) {
	var d struct {
		ID       int64
		Revision struct{ ID int64 }
	}
	if err := c.call("POST", "/api/contents", map[string]string{"type": typ}, &d); err != nil {
		return 0, err
	}
	if err := c.call("PUT", fmt.Sprintf("/api/contents/%d/draft", d.ID), map[string]any{"title": title, "body": body, "baseRevisionId": d.Revision.ID}, nil); err != nil {
		return 0, err
	}
	if err := c.call("PATCH", fmt.Sprintf("/api/contents/%d", d.ID), patch, nil); err != nil {
		return 0, err
	}
	return d.ID, c.call("POST", fmt.Sprintf("/api/contents/%d/publish", d.ID), nil, nil)
}

type mediaRef struct {
	ID      int64  `json:"id"`
	URL     string `json:"url"`
	AltText string `json:"altText"`
}

func (c *client) upload(path, alt string) (mediaRef, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return mediaRef{}, err
	}
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("altText", alt)
	fw, _ := w.CreateFormFile("file", filepath.Base(path))
	_, _ = fw.Write(f)
	_ = w.Close()
	var m mediaRef
	return m, c.do("POST", "/api/media", &b, w.FormDataContentType(), &m)
}

func (c *client) call(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.do(method, path, body, "application/json", out)
}

func (c *client) do(method, path string, body io.Reader, ctype string, out any) error {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Pagewright", "1")
	if body != nil {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s %s", method, path, res.Status, b)
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

// toDoc turns compact blocks into body JSON (schema/body-v1.md).
func toDoc(blocks []block, media map[string]mediaRef) (map[string]any, error) {
	var content []any
	for _, b := range blocks {
		if len(b) != 1 {
			return nil, fmt.Errorf("a block must have exactly one key: %v", b)
		}
		for kind, raw := range b {
			n, err := toNode(kind, raw, media)
			if err != nil {
				return nil, err
			}
			content = append(content, n)
		}
	}
	return map[string]any{"type": "doc", "content": content}, nil
}

func toNode(kind string, raw json.RawMessage, media map[string]mediaRef) (any, error) {
	str := func() (string, error) {
		var s string
		err := json.Unmarshal(raw, &s)
		return s, err
	}
	switch kind {
	case "p", "h2", "h3", "quote":
		s, err := str()
		if err != nil {
			return nil, err
		}
		para := map[string]any{"type": "paragraph", "content": inline(s)}
		switch kind {
		case "h2", "h3":
			return map[string]any{"type": "heading", "attrs": map[string]any{"level": int(kind[1] - '0')}, "content": inline(s)}, nil
		case "quote":
			return map[string]any{"type": "blockquote", "content": []any{para}}, nil
		}
		return para, nil
	case "ul", "ol":
		var items []string
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		var lis []any
		for _, it := range items {
			lis = append(lis, map[string]any{"type": "listItem", "content": []any{map[string]any{"type": "paragraph", "content": inline(it)}}})
		}
		typ := "bulletList"
		if kind == "ol" {
			typ = "orderedList"
		}
		return map[string]any{"type": typ, "content": lis}, nil
	case "img":
		key, err := str()
		if err != nil {
			return nil, err
		}
		m, ok := media[key]
		if !ok {
			return nil, fmt.Errorf("unknown image %q", key)
		}
		return map[string]any{"type": "image", "attrs": map[string]any{"src": m.URL, "alt": m.AltText, "mediaId": m.ID}}, nil
	case "table":
		var t struct {
			Head []string
			Rows [][]string
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, err
		}
		cell := func(typ, text string) any {
			return map[string]any{"type": typ, "content": []any{map[string]any{"type": "paragraph", "content": inline(text)}}}
		}
		var rows []any
		if len(t.Head) > 0 {
			var head []any
			for _, h := range t.Head {
				head = append(head, cell("tableHeader", h))
			}
			rows = append(rows, map[string]any{"type": "tableRow", "content": head})
		}
		for _, r := range t.Rows {
			var cells []any
			for i, v := range r {
				typ := "tableCell"
				if i == 0 && len(t.Head) <= 2 {
					typ = "tableHeader" // item / value tables read better with row headers
				}
				cells = append(cells, cell(typ, v))
			}
			rows = append(rows, map[string]any{"type": "tableRow", "content": cells})
		}
		return map[string]any{"type": "table", "content": rows}, nil
	case "hr":
		return map[string]any{"type": "horizontalRule"}, nil
	case "form":
		return map[string]any{"type": "contactForm"}, nil
	}
	return nil, fmt.Errorf("unknown block %q", kind)
}

// inlineToken finds **bold**, {base|ruby} and [text](href).
var inlineToken = regexp.MustCompile(`\*\*(.+?)\*\*|\{([^|{}]+)\|([^{}]+)\}|\[([^\]]+)\]\(([^)]+)\)`)

func inline(s string) []any {
	var out []any
	text := func(t string, marks ...any) {
		if t == "" {
			return
		}
		n := map[string]any{"type": "text", "text": t}
		if len(marks) > 0 {
			n["marks"] = marks
		}
		out = append(out, n)
	}
	rest := s
	for {
		loc := inlineToken.FindStringSubmatchIndex(rest)
		if loc == nil {
			text(rest)
			return out
		}
		text(rest[:loc[0]])
		m := func(i int) string { return rest[loc[2*i]:loc[2*i+1]] }
		switch {
		case loc[2] >= 0:
			text(m(1), map[string]any{"type": "bold"})
		case loc[4] >= 0:
			text(m(2), map[string]any{"type": "ruby", "attrs": map[string]any{"rt": m(3)}})
		default:
			text(m(4), map[string]any{"type": "link", "attrs": map[string]any{"href": strings.TrimSpace(m(5))}})
		}
		rest = rest[loc[1]:]
	}
}
