// Package render turns a stored body (ProseMirror JSON, schema/body-v1.md)
// into HTML for the public site.
//
// Everything is escaped. Only the nodes, marks and attributes in the schema
// are written; anything else is dropped and reported as a warning, so a
// broken or hostile document cannot inject markup. The one way to get
// arbitrary markup in, rawHtml, goes through bluemonday.
package render

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Options tune rendering.
type Options struct {
	// MediaSize returns the pixel size of a media item so <img> gets width
	// and height (avoids layout shift). Optional.
	MediaSize func(mediaID int64) (width, height int64, ok bool)
	// ContactFormAction is where the contactForm block posts. Empty drops
	// the block with a warning.
	ContactFormAction string
	// PagePath is the public path of the page being rendered; the contact
	// form sends it so the reply page can link back.
	PagePath string
	// MediaPrefix replaces "/media/" at the start of image sources, for
	// pages served from elsewhere than the site (previews).
	MediaPrefix string
}

// Result is a rendered body.
type Result struct {
	HTML template.HTML
	// Text is the plain text of the body, blocks separated by newlines.
	// Used for descriptions and excerpts.
	Text string
	// MediaIDs are the mediaId attributes of images, in document order.
	MediaIDs []int64
	// MediaKeys are the storage keys of images whose src is /media/{key},
	// unescaped, in document order.
	MediaKeys []string
	// Warnings describe what was dropped.
	Warnings []string
}

// MaxDepth bounds node nesting; deeper nodes are dropped.
const MaxDepth = 64

type node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs"`
	Content []node         `json:"content"`
	Text    string         `json:"text"`
	Marks   []mark         `json:"marks"`
}

type mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

// Body renders a body document. It fails only when raw is not a JSON doc.
func Body(raw []byte, opts Options) (Result, error) {
	var doc node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Result{}, fmt.Errorf("render: parse body: %w", err)
	}
	if doc.Type != "doc" {
		return Result{}, fmt.Errorf("render: root node is %q, want doc", doc.Type)
	}
	r := &renderer{opts: opts}
	r.blocks(doc.Content, 1)
	return Result{
		HTML:      template.HTML(r.b.String()), //nolint:gosec // built from escaped parts only
		Text:      strings.TrimSpace(r.text.String()),
		MediaIDs:  r.mediaIDs,
		MediaKeys: r.mediaKeys,
		Warnings:  r.warnings,
	}, nil
}

type renderer struct {
	opts      Options
	b         strings.Builder
	text      strings.Builder
	mediaIDs  []int64
	mediaKeys []string
	warnings  []string
}

func (r *renderer) warn(format string, args ...any) {
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

func (r *renderer) w(s ...string) {
	for _, p := range s {
		r.b.WriteString(p)
	}
}

func esc(s string) string { return template.HTMLEscapeString(s) }

func (r *renderer) blocks(nodes []node, depth int) {
	for _, n := range nodes {
		r.block(n, depth)
	}
}

func (r *renderer) block(n node, depth int) {
	if depth > MaxDepth {
		r.warn("node nested deeper than %d dropped", MaxDepth)
		return
	}
	switch n.Type {
	case "paragraph":
		r.w("<p>")
		r.inline(n.Content)
		r.w("</p>\n")
		r.text.WriteString("\n")
	case "heading":
		level := clamp(intAttr(n.Attrs, "level", 2), 2, 4)
		h := "h" + strconv.Itoa(level)
		r.w("<", h, ">")
		r.inline(n.Content)
		r.w("</", h, ">\n")
		r.text.WriteString("\n")
	case "bulletList":
		r.w("<ul>\n")
		r.items(n.Content, depth)
		r.w("</ul>\n")
	case "orderedList":
		if start := intAttr(n.Attrs, "start", 1); start != 1 {
			r.w(`<ol start="`, strconv.Itoa(start), `">`, "\n")
		} else {
			r.w("<ol>\n")
		}
		r.items(n.Content, depth)
		r.w("</ol>\n")
	case "blockquote":
		r.w("<blockquote>\n")
		r.blocks(n.Content, depth+1)
		r.w("</blockquote>\n")
	case "horizontalRule":
		r.w("<hr>\n")
	case "image":
		r.image(n)
	case "table":
		r.table(n, depth)
	case "embed":
		r.embed(n)
	case "contactForm":
		r.contactForm()
	case "rawHtml":
		html, _ := n.Attrs["html"].(string)
		clean := ugc.Sanitize(html)
		if strings.TrimSpace(clean) == "" {
			return
		}
		r.w(`<div class="raw-html">`, clean, "</div>\n")
		r.text.WriteString(stdhtml.UnescapeString(strict.Sanitize(html)) + "\n")
	default:
		r.warn("unknown block node %q dropped", n.Type)
	}
}

var (
	ugc    = bluemonday.UGCPolicy()
	strict = bluemonday.StrictPolicy()
)

func (r *renderer) items(nodes []node, depth int) {
	for _, it := range nodes {
		if it.Type != "listItem" {
			r.warn("%q inside a list dropped", it.Type)
			continue
		}
		r.w("<li>")
		r.blocks(it.Content, depth+2)
		r.w("</li>\n")
	}
}

func (r *renderer) table(n node, depth int) {
	r.w(`<div class="table-wrap"><table>`, "\n<tbody>\n")
	for _, row := range n.Content {
		if row.Type != "tableRow" {
			r.warn("%q inside a table dropped", row.Type)
			continue
		}
		r.w("<tr>")
		for _, cell := range row.Content {
			var tag string
			switch cell.Type {
			case "tableHeader":
				tag = "th"
			case "tableCell":
				tag = "td"
			default:
				r.warn("%q inside a table row dropped", cell.Type)
				continue
			}
			r.w("<", tag)
			for _, a := range []string{"colspan", "rowspan"} {
				if v := clamp(intAttr(cell.Attrs, a, 1), 1, 1000); v > 1 {
					r.w(" ", a, `="`, strconv.Itoa(v), `"`)
				}
			}
			r.w(">")
			r.blocks(cell.Content, depth+3)
			r.w("</", tag, ">")
		}
		r.w("</tr>\n")
	}
	r.w("</tbody>\n</table></div>\n")
}

func (r *renderer) image(n node) {
	src, _ := n.Attrs["src"].(string)
	alt, _ := n.Attrs["alt"].(string)
	src, ok := safeURL(src, false)
	if !ok {
		r.warn("image with unsafe or empty src dropped")
		return
	}
	if strings.TrimSpace(alt) == "" {
		r.warn("image without alt dropped: %s", src)
		return
	}
	mediaID := int64(intAttr(n.Attrs, "mediaId", 0))
	if mediaID > 0 {
		r.mediaIDs = append(r.mediaIDs, mediaID)
	}
	if key, ok := strings.CutPrefix(src, "/media/"); ok && key != "" {
		if r.opts.MediaPrefix != "" {
			src = r.opts.MediaPrefix + key
		}
		if u, err := url.PathUnescape(key); err == nil {
			key = u
		}
		r.mediaKeys = append(r.mediaKeys, key)
	}
	r.w(`<figure class="image"><img src="`, esc(src), `" alt="`, esc(alt), `"`)
	if title, _ := n.Attrs["title"].(string); title != "" {
		r.w(` title="`, esc(title), `"`)
	}
	if mediaID > 0 && r.opts.MediaSize != nil {
		if w, h, ok := r.opts.MediaSize(mediaID); ok && w > 0 && h > 0 {
			r.w(` width="`, strconv.FormatInt(w, 10), `" height="`, strconv.FormatInt(h, 10), `"`)
		}
	}
	r.w(` loading="lazy" decoding="async"></figure>`, "\n")
}

// inline writes inline content. Marks are kept open across adjacent nodes
// that share them (as ProseMirror's serializer does), so a link spanning a
// bold word is one <a>.
func (r *renderer) inline(nodes []node) {
	var active []mark
	for _, n := range nodes {
		var marks []mark
		switch n.Type {
		case "text":
			marks = r.marks(n.Marks)
		case "hardBreak":
			marks = active // a line break never splits the marks around it
		default:
			r.warn("unknown inline node %q dropped", n.Type)
			continue
		}
		keep := 0
		for keep < len(active) && keep < len(marks) && sameMark(active[keep], marks[keep]) {
			keep++
		}
		for i := len(active) - 1; i >= keep; i-- {
			r.closeMark(active[i])
		}
		active = active[:keep]
		for _, m := range marks[keep:] {
			r.openMark(m)
			active = append(active, m)
		}
		if n.Type == "hardBreak" {
			r.w("<br>")
			r.text.WriteString("\n")
			continue
		}
		r.w(esc(n.Text))
		r.text.WriteString(n.Text)
	}
	for i := len(active) - 1; i >= 0; i-- {
		r.closeMark(active[i])
	}
}

// markRank orders marks from outermost to innermost.
var markRank = map[string]int{"link": 0, "bold": 1, "italic": 2, "code": 3, "ruby": 4}

// marks validates, dedupes and orders the marks of a text node.
func (r *renderer) marks(in []mark) []mark {
	var out []mark
	for _, m := range in {
		if _, ok := markRank[m.Type]; !ok {
			r.warn("unknown mark %q dropped", m.Type)
			continue
		}
		if slices.ContainsFunc(out, func(o mark) bool { return o.Type == m.Type }) {
			continue
		}
		clean := mark{Type: m.Type}
		switch m.Type {
		case "link":
			href, _ := m.Attrs["href"].(string)
			href, ok := safeURL(href, true)
			if !ok {
				r.warn("link with unsafe href dropped")
				continue
			}
			clean.Attrs = map[string]any{"href": href}
			if t, _ := m.Attrs["target"].(string); t == "_blank" {
				clean.Attrs["target"] = t
			}
		case "ruby":
			rt, _ := m.Attrs["rt"].(string)
			if strings.TrimSpace(rt) == "" {
				r.warn("ruby without rt dropped")
				continue
			}
			clean.Attrs = map[string]any{"rt": rt}
		}
		out = append(out, clean)
	}
	slices.SortStableFunc(out, func(a, b mark) int { return markRank[a.Type] - markRank[b.Type] })
	return out
}

func sameMark(a, b mark) bool {
	if a.Type != b.Type || len(a.Attrs) != len(b.Attrs) {
		return false
	}
	for k, v := range a.Attrs {
		if b.Attrs[k] != v {
			return false
		}
	}
	return true
}

func (r *renderer) openMark(m mark) {
	switch m.Type {
	case "bold":
		r.w("<strong>")
	case "italic":
		r.w("<em>")
	case "code":
		r.w("<code>")
	case "ruby":
		r.w("<ruby>")
	case "link":
		r.w(`<a href="`, esc(m.Attrs["href"].(string)), `"`)
		if m.Attrs["target"] == "_blank" {
			r.w(` target="_blank" rel="noopener noreferrer"`)
		}
		r.w(">")
	}
}

func (r *renderer) closeMark(m mark) {
	switch m.Type {
	case "bold":
		r.w("</strong>")
	case "italic":
		r.w("</em>")
	case "code":
		r.w("</code>")
	case "ruby":
		r.w("<rp>(</rp><rt>", esc(m.Attrs["rt"].(string)), "</rt><rp>)</rp></ruby>")
	case "link":
		r.w("</a>")
	}
}

// safeURL accepts http(s) URLs, relative URLs and, when allowMailto is set,
// mailto: links. Anything else (javascript:, data:, ...) is refused.
func safeURL(s string, allowMailto bool) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsFunc(s, func(c rune) bool { return c < 0x20 || c == 0x7f }) {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "", "http", "https":
		return s, true
	case "mailto":
		return s, allowMailto
	}
	return "", false
}

func intAttr(attrs map[string]any, key string, def int) int {
	f, ok := attrs[key].(float64)
	if !ok || f != float64(int(f)) {
		return def
	}
	return int(f)
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }
