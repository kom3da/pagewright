package htmlpm

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// run collects inline content into paragraphs (or headings). Block content
// met on the way (images, embeds, nested blocks) ends the current
// paragraph and is emitted in order.
type run struct {
	c     *conv
	typ   string // "paragraph" or "heading"
	level int
	out   []Node
	para  []Node
	// src is the HTML the run came from, used when the run has to be kept
	// as rawHtml.
	src       []*html.Node
	raw       bool
	rawReason string
}

func (c *conv) newRun(typ string, level int) *run {
	return &run{c: c, typ: typ, level: level}
}

func (r *run) children(n *html.Node, marks []Mark) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		r.inline(ch, marks)
	}
}

func withMark(marks []Mark, m Mark) []Mark {
	for _, have := range marks {
		if have.Type == m.Type {
			return marks // <b><strong>x</strong></b>
		}
	}
	return append(slices.Clone(marks), m)
}

func (r *run) inline(n *html.Node, marks []Mark) {
	switch n.Type {
	case html.TextNode:
		r.text(n.Data, marks)
		return
	case html.ElementNode:
	default:
		return
	}
	if p, ok := r.c.placeholderOf(n); ok {
		r.flush()
		r.out = append(r.out, p.nodes...)
		return
	}
	switch name := n.Data; {
	case name == "br":
		r.hardBreak()
	case name == "strong" || name == "b":
		r.children(n, withMark(marks, Mark{Type: "bold"}))
	case name == "em" || name == "i":
		r.children(n, withMark(marks, Mark{Type: "italic"}))
	case name == "code" || name == "kbd" || name == "samp" || name == "tt":
		r.children(n, withMark(marks, Mark{Type: "code"}))
	case name == "a":
		r.link(n, marks)
	case name == "ruby":
		r.ruby(n, marks)
	case name == "img":
		r.flush()
		if img, ok := r.c.image(n, linkHref(marks)); ok {
			r.out = append(r.out, img)
		}
	case name == "iframe":
		r.flush()
		r.out = append(r.out, r.c.iframe(n)...)
	case meaningfulInline[name]:
		r.raw = true
		r.rawReason = name
		r.children(n, marks)
	case styledInline[name] || (name == "span" && attr(n, "style") != ""):
		r.c.warn(WarnStyle, "文字の装飾（<"+name+">）は取り込めないため、文字だけを取り込みました", render(r.c, n))
		r.children(n, marks)
	case plainInline[name] || name == "rt" || name == "rb":
		r.children(n, marks)
	case ignored[name]:
	case isBlockElement(n):
		r.flush()
		r.out = append(r.out, r.c.block(n)...)
	default:
		r.flush()
		r.out = append(r.out, r.c.raw(n, "<"+name+"> は変換できないため、カスタムHTMLとして残しました")...)
	}
}

func (r *run) link(n *html.Node, marks []Mark) {
	href := strings.TrimSpace(attr(n, "href"))
	if href == "" {
		r.children(n, marks) // <a name="...">
		return
	}
	scheme := strings.ToLower(strings.Join(strings.Fields(href), ""))
	if strings.HasPrefix(scheme, "javascript:") || strings.HasPrefix(scheme, "vbscript:") || strings.HasPrefix(scheme, "data:") {
		r.c.warn(WarnLink, "安全でないリンクを外し、文字だけを取り込みました", render(r.c, n))
		r.children(n, marks)
		return
	}
	m := Mark{Type: "link", Attrs: map[string]any{"href": href}}
	if attr(n, "target") == "_blank" {
		m.Attrs["target"] = "_blank"
	}
	r.children(n, withMark(marks, m))
}

func linkHref(marks []Mark) string {
	for _, m := range marks {
		if m.Type == "link" {
			s, _ := m.Attrs["href"].(string)
			return s
		}
	}
	return ""
}

// ruby converts <ruby>漢<rt>かん</rt>字<rt>じ</rt></ruby> into text with a
// ruby mark per base.
func (r *run) ruby(n *html.Node, marks []Mark) {
	var base strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode {
			if ch.Type == html.TextNode {
				base.WriteString(ch.Data)
			}
			continue
		}
		switch ch.Data {
		case "rp":
		case "rt":
			rt := strings.TrimSpace(collapse(textContent(ch)))
			b := strings.TrimSpace(collapse(base.String()))
			base.Reset()
			switch {
			case b == "" && rt == "":
			case b == "":
				r.appendText(rt, marks)
			case rt == "":
				r.appendText(b, marks)
			default:
				r.appendText(b, withMark(marks, Mark{Type: "ruby", Attrs: map[string]any{"rt": rt}}))
			}
		default:
			base.WriteString(textContent(ch))
		}
	}
	if base.Len() > 0 {
		r.text(base.String(), marks)
	}
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		sb.WriteString(textContent(ch))
	}
	return sb.String()
}

var (
	blankLineRe = regexp.MustCompile(`\n[ \t]*\n\s*`)
	spaceRe     = regexp.MustCompile(`[ \t\n\r\f]+`)
	hSpaceRe    = regexp.MustCompile(`[ \t\r\f]+`)
)

// collapse applies HTML whitespace collapsing.
func collapse(s string) string {
	return spaceRe.ReplaceAllString(s, " ")
}

func (r *run) text(s string, marks []Mark) {
	if !r.c.autop {
		r.appendText(collapse(s), marks)
		return
	}
	// wpautop: a blank line ends a paragraph, a newline is a line break.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for i, part := range blankLineRe.Split(s, -1) {
		if i > 0 {
			r.flush()
		}
		for j, line := range strings.Split(part, "\n") {
			if j > 0 {
				r.hardBreak()
			}
			r.appendText(hSpaceRe.ReplaceAllString(line, " "), marks)
		}
	}
}

func (r *run) hardBreak() {
	if len(r.para) == 0 {
		return
	}
	r.para = append(r.para, Node{Type: "hardBreak"})
}

func (r *run) appendText(s string, marks []Mark) {
	if strings.HasPrefix(s, " ") && r.atLineStart() {
		s = strings.TrimLeft(s, " ")
	}
	if s == "" {
		return
	}
	if n := len(r.para); n > 0 {
		last := &r.para[n-1]
		if last.Type == "text" && sameMarks(last.Marks, marks) {
			last.Text += s
			return
		}
	}
	r.para = append(r.para, text(s, slices.Clone(marks)...))
}

// atLineStart reports whether a leading space would be collapsed away.
func (r *run) atLineStart() bool {
	if len(r.para) == 0 {
		return true
	}
	last := r.para[len(r.para)-1]
	return last.Type == "hardBreak" || (last.Type == "text" && strings.HasSuffix(last.Text, " "))
}

func sameMarks(a, b []Mark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || len(a[i].Attrs) != len(b[i].Attrs) {
			return false
		}
		for k, v := range a[i].Attrs {
			if b[i].Attrs[k] != v {
				return false
			}
		}
	}
	return true
}

// flush ends the current paragraph.
func (r *run) flush() {
	p := r.para
	r.para = nil
	for len(p) > 0 {
		last := &p[len(p)-1]
		if last.Type == "hardBreak" {
			p = p[:len(p)-1]
			continue
		}
		if last.Type == "text" {
			last.Text = strings.TrimRight(last.Text, " ")
			if last.Text == "" {
				p = p[:len(p)-1]
				continue
			}
		}
		break
	}
	if blank(p) {
		return // e.g. <p>&nbsp;</p> used as spacing
	}
	if r.typ == "heading" {
		r.out = append(r.out, Node{Type: "heading", Attrs: map[string]any{"level": r.level}, Content: p})
		return
	}
	// WordPress turns a URL alone on a line into an embed.
	if r.c.autop && len(p) == 1 && len(p[0].Marks) == 0 {
		u := strings.TrimSpace(p[0].Text)
		if prov, ok := provider(u); ok && !strings.ContainsAny(u, "  ") {
			r.out = append(r.out, embed(prov, u))
			return
		}
	}
	r.out = append(r.out, paragraph(p...))
}

func blank(p []Node) bool {
	for _, n := range p {
		if n.Type != "text" || strings.TrimSpace(strings.ReplaceAll(n.Text, " ", " ")) != "" {
			return false
		}
	}
	return true
}

// end finishes the run and returns its blocks.
func (r *run) end() []Node {
	r.flush()
	if !r.raw {
		return r.out
	}
	h := render(r.c, r.src...)
	r.c.warn(WarnRawHTML, "取り消し線・上付き・下付きの文字（<"+r.rawReason+">）を含むため、この部分をカスタムHTMLとして残しました", h)
	return []Node{rawHTML(h)}
}
