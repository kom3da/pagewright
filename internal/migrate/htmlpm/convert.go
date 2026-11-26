package htmlpm

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Options tune Convert.
type Options struct {
	// Image is called for every image source found. It may rewrite the src
	// (e.g. to a downloaded copy under /media/), give the media ID, and
	// supply alt text known for the file (the attachment's alt). Nil keeps
	// sources as they are.
	Image func(src string) ImageInfo
}

// ImageInfo is what Options.Image knows about an image.
type ImageInfo struct {
	Src     string // empty keeps the original src
	MediaID *int64
	Alt     string
}

// Convert turns WordPress post HTML into a ProseMirror doc node.
func Convert(src string, opts Options) (Node, []Warning) {
	c := &conv{opts: opts}
	root, hasBlocks := parseBlocks(src)
	// The classic editor stores text without <p>; WordPress adds
	// paragraphs and line breaks on output (wpautop). Block content is
	// stored as finished HTML.
	c.autop = !hasBlocks
	h := c.blocksHTML(root)
	nodes, err := html.ParseFragment(strings.NewReader(h), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		// The HTML5 parser accepts any input; this is not expected.
		c.warn(WarnRawHTML, "HTMLを解析できないため、本文全体をカスタムHTMLとして残しました", src)
		return doc([]Node{rawHTML(src)}), c.warnings
	}
	return doc(c.container(nodes)), c.warnings
}

type conv struct {
	opts         Options
	autop        bool
	warnings     []Warning
	placeholders []placeholderEntry
}

type placeholderEntry struct {
	src   string
	nodes []Node
}

const placeholderTag = "pw-placeholder"

// placeholder registers nodes converted ahead of the DOM pass and returns
// an element that stands for them in the HTML.
func (c *conv) placeholder(src string, nodes ...Node) string {
	c.placeholders = append(c.placeholders, placeholderEntry{src: src, nodes: nodes})
	return fmt.Sprintf(`<%s data-i="%d"></%s>`, placeholderTag, len(c.placeholders)-1, placeholderTag)
}

func (c *conv) placeholderOf(n *html.Node) (placeholderEntry, bool) {
	if n.Type != html.ElementNode || n.Data != placeholderTag {
		return placeholderEntry{}, false
	}
	i, err := strconv.Atoi(attr(n, "data-i"))
	if err != nil || i < 0 || i >= len(c.placeholders) {
		return placeholderEntry{}, false
	}
	return c.placeholders[i], true
}

func (c *conv) warn(code, msg, fragment string) {
	c.warnings = append(c.warnings, Warning{Code: code, Message: msg, Fragment: shorten(fragment)})
}

func shorten(s string) string {
	s = strings.TrimSpace(s)
	const limit = 200
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return string(r[:limit]) + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

// Element classes.
var (
	transparentBlocks = set("div", "section", "article", "header", "footer", "aside",
		"main", "nav", "center", "figure", "address", "hgroup", "picture", "tbody")
	rawBlocks = set("pre", "dl", "form", "script", "style", "noscript", "video", "audio",
		"object", "embed", "canvas", "svg", "math", "select", "textarea", "input", "button",
		"details", "map", "fieldset", "template", "frameset", "marquee", "menu")
	// Inline elements that carry meaning the schema cannot hold: the
	// paragraph containing them is kept as rawHtml so the meaning is not
	// lost (a struck-through old price, a superscript unit).
	meaningfulInline = set("sub", "sup", "s", "strike", "del")
	// Inline elements whose look is lost but whose text is kept.
	styledInline = set("u", "mark", "font", "big", "small", "ins", "blink")
	plainInline  = set("span", "abbr", "acronym", "cite", "q", "time", "label", "bdi", "bdo",
		"dfn", "var", "data", "output", "nobr")
	ignored = set("wbr", "source", "track", "col", "colgroup", "rp", "meta", "link", "param")
)

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func isBlockElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "hr",
		"table", "figcaption", "caption", "tr", "td", "th", "thead", "tfoot":
		return true
	}
	return transparentBlocks[n.Data] || rawBlocks[n.Data]
}

// container converts flow content: block elements become blocks, runs of
// inline content become paragraphs.
func (c *conv) container(nodes []*html.Node) []Node {
	var out []Node
	var r *run
	endRun := func() {
		if r != nil {
			out = append(out, r.end()...)
			r = nil
		}
	}
	for _, n := range nodes {
		switch {
		case n.Type == html.CommentNode || n.Type == html.DoctypeNode:
			continue
		case n.Type == html.TextNode && r == nil && isSpace(n.Data):
			continue
		case isBlockElement(n):
			endRun()
			out = append(out, c.block(n)...)
		default:
			if _, ok := c.placeholderOf(n); ok {
				endRun()
				out = append(out, c.blockPlaceholder(n)...)
				continue
			}
			if r == nil {
				r = c.newRun("paragraph", 0)
			}
			r.src = append(r.src, n)
			r.inline(n, nil)
		}
	}
	endRun()
	return out
}

func (c *conv) blockPlaceholder(n *html.Node) []Node {
	p, _ := c.placeholderOf(n)
	return p.nodes
}

func children(n *html.Node) []*html.Node {
	var out []*html.Node
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		out = append(out, ch)
	}
	return out
}

func (c *conv) block(n *html.Node) []Node {
	switch n.Data {
	case "p", "figcaption", "caption":
		return c.textBlock(n, "paragraph", 0)
	case "h1", "h2":
		return c.textBlock(n, "heading", 2)
	case "h3":
		return c.textBlock(n, "heading", 3)
	case "h4", "h5", "h6":
		return c.textBlock(n, "heading", 4)
	case "ul", "ol":
		return c.list(n)
	case "li":
		// A list item outside a list.
		return c.container(children(n))
	case "blockquote":
		content := c.container(children(n))
		if len(content) == 0 {
			return nil
		}
		return []Node{{Type: "blockquote", Content: content}}
	case "hr":
		return []Node{{Type: "horizontalRule"}}
	case "table":
		return c.table(n)
	case "tr", "td", "th", "thead", "tfoot":
		// Table parts outside a table: keep their content.
		return c.container(children(n))
	}
	if transparentBlocks[n.Data] {
		if attr(n, "style") != "" {
			c.warn(WarnStyle, "<"+n.Data+"> の style（配置や色など）は取り込めないため、中身だけを取り込みました", render(c, n))
		}
		return c.container(children(n))
	}
	return c.raw(n, "<"+n.Data+"> は変換できないため、カスタムHTMLとして残しました")
}

func (c *conv) raw(n *html.Node, msg string) []Node {
	h := render(c, n)
	c.warn(WarnRawHTML, msg, h)
	return []Node{rawHTML(h)}
}

// textBlock converts a paragraph-like element. Block content nested in it
// (images, embeds) splits it into several blocks.
func (c *conv) textBlock(n *html.Node, typ string, level int) []Node {
	r := c.newRun(typ, level)
	r.src = []*html.Node{n}
	for _, ch := range children(n) {
		r.inline(ch, nil)
	}
	return r.end()
}

func (c *conv) list(n *html.Node) []Node {
	var items []Node
	for _, ch := range children(n) {
		switch {
		case ch.Type == html.CommentNode:
		case ch.Type == html.TextNode && isSpace(ch.Data):
		case ch.Type == html.ElementNode && ch.Data == "li":
			items = append(items, listItem(c.container(children(ch))))
		case ch.Type == html.ElementNode && (ch.Data == "ul" || ch.Data == "ol") && len(items) > 0:
			// <ul><li>a</li><ul>...</ul></ul>: the nested list belongs
			// to the previous item.
			last := &items[len(items)-1]
			last.Content = append(last.Content, c.list(ch)...)
		default:
			items = append(items, listItem(c.container([]*html.Node{ch})))
		}
	}
	if len(items) == 0 {
		return nil
	}
	if n.Data == "ul" {
		return []Node{{Type: "bulletList", Content: items}}
	}
	start := 1
	if v, err := strconv.Atoi(strings.TrimSpace(attr(n, "start"))); err == nil && v >= 1 {
		start = v
	}
	return []Node{{Type: "orderedList", Attrs: map[string]any{"start": start}, Content: items}}
}

func listItem(content []Node) Node {
	if len(content) == 0 {
		content = []Node{paragraph()}
	}
	return Node{Type: "listItem", Content: content}
}

func (c *conv) table(n *html.Node) []Node {
	var before, rows []Node
	var walk func(*html.Node)
	walk = func(p *html.Node) {
		for _, ch := range children(p) {
			if ch.Type != html.ElementNode {
				if ch.Type == html.TextNode && !isSpace(ch.Data) {
					before = append(before, c.container([]*html.Node{ch})...)
				}
				continue
			}
			switch ch.Data {
			case "caption":
				before = append(before, c.textBlock(ch, "paragraph", 0)...)
			case "thead", "tbody", "tfoot":
				walk(ch)
			case "tr":
				if row, ok := c.tableRow(ch); ok {
					rows = append(rows, row)
				}
			case "colgroup", "col":
			default:
				before = append(before, c.container([]*html.Node{ch})...)
			}
		}
	}
	walk(n)
	if len(rows) == 0 {
		return before
	}
	return append(before, Node{Type: "table", Content: rows})
}

func (c *conv) tableRow(tr *html.Node) (Node, bool) {
	var cells []Node
	for _, ch := range children(tr) {
		if ch.Type != html.ElementNode || (ch.Data != "td" && ch.Data != "th") {
			continue
		}
		typ := "tableCell"
		if ch.Data == "th" {
			typ = "tableHeader"
		}
		content := c.container(children(ch))
		if len(content) == 0 {
			content = []Node{paragraph()}
		}
		cells = append(cells, Node{
			Type:    typ,
			Attrs:   map[string]any{"colspan": span(ch, "colspan"), "rowspan": span(ch, "rowspan")},
			Content: content,
		})
	}
	if len(cells) == 0 {
		return Node{}, false
	}
	return Node{Type: "tableRow", Content: cells}, true
}

func span(n *html.Node, key string) int {
	v, err := strconv.Atoi(strings.TrimSpace(attr(n, key)))
	if err != nil || v < 1 {
		return 1
	}
	return v
}

// image converts an <img>. It returns false when the image has no source.
func (c *conv) image(n *html.Node, linked string) (Node, bool) {
	src := strings.TrimSpace(attr(n, "src"))
	if src == "" || strings.HasPrefix(src, "data:") {
		// Lazy-loading plugins keep the real source elsewhere.
		if s := strings.TrimSpace(attr(n, "data-src")); s != "" {
			src = s
		}
	}
	if src == "" {
		c.warn(WarnDropped, "画像の場所（src）がないため、画像を取り込みませんでした", render(c, n))
		return Node{}, false
	}
	info := ImageInfo{}
	if c.opts.Image != nil {
		info = c.opts.Image(src)
	}
	alt := strings.TrimSpace(attr(n, "alt"))
	if alt == "" {
		alt = strings.TrimSpace(info.Alt)
	}
	if alt == "" {
		alt = altFromFilename(src)
		c.warn(WarnAltFallback, "画像に代替テキストがないため、仮に「"+alt+"」としました。内容に合わせて直してください", render(c, n))
	}
	if linked != "" && !looksLikeImage(linked) {
		c.warn(WarnLink, "画像に付いていたリンク（"+linked+"）は取り込めませんでした", render(c, n))
	}
	if info.Src != "" {
		src = info.Src
	}
	var mediaID any
	if info.MediaID != nil {
		mediaID = *info.MediaID
	}
	var title any
	if t := strings.TrimSpace(attr(n, "title")); t != "" {
		title = t
	}
	return Node{Type: "image", Attrs: map[string]any{"src": src, "alt": alt, "title": title, "mediaId": mediaID}}, true
}

var sizeSuffixRe = regexp.MustCompile(`-\d+x\d+$`)

// altFromFilename makes a placeholder alt text from an image URL:
// ".../2024/05/shop-300x225.jpg" gives "shop".
func altFromFilename(src string) string {
	p := src
	if u, err := url.Parse(src); err == nil {
		p = u.Path
	}
	name := path.Base(p)
	if dec, err := url.PathUnescape(name); err == nil {
		name = dec
	}
	name = strings.TrimSuffix(name, path.Ext(name))
	name = sizeSuffixRe.ReplaceAllString(name, "")
	if name == "" || name == "." || name == "/" {
		return "画像"
	}
	return name
}

func looksLikeImage(href string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".svg":
		return true
	}
	return false
}

// iframe converts YouTube and Google Maps iframes to embed nodes.
func (c *conv) iframe(n *html.Node) []Node {
	src := strings.TrimSpace(attr(n, "src"))
	if src == "" {
		src = strings.TrimSpace(attr(n, "data-src"))
	}
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}
	if p, ok := provider(src); ok {
		return []Node{embed(p, src)}
	}
	return c.raw(n, "iframe（"+src+"）は YouTube と Google マップ以外に対応していないため、カスタムHTMLとして残しました")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func isSpace(s string) bool {
	return strings.TrimSpace(s) == ""
}

// render serializes n back to HTML, restoring placeholders to their
// original source.
func render(c *conv, nodes ...*html.Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		if err := html.Render(&sb, n); err != nil {
			continue
		}
	}
	s := sb.String()
	if !strings.Contains(s, "<"+placeholderTag) {
		return s
	}
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		i, err := strconv.Atoi(placeholderRe.FindStringSubmatch(m)[1])
		if err != nil || i >= len(c.placeholders) {
			return m
		}
		return c.placeholders[i].src
	})
}

var placeholderRe = regexp.MustCompile(`<` + placeholderTag + ` data-i="(\d+)"></` + placeholderTag + `>`)
