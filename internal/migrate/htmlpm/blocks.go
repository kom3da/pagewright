package htmlpm

import (
	"encoding/json"
	"regexp"
	"strings"
)

// block is a Gutenberg block parsed from its comment delimiters
// (<!-- wp:name {"attrs"} --> ... <!-- /wp:name -->).
type block struct {
	name  string // "core/paragraph"; "" for the root
	attrs json.RawMessage
	src   string // original text, delimiters included
	start int
	inner []piece
}

// piece is HTML text or a nested block.
type piece struct {
	html  string
	block *block
}

var delimRe = regexp.MustCompile(`^<!--\s+(/)?wp:([a-z][a-z0-9_-]*/)?([a-z][a-z0-9_-]*)\s+(\{[\s\S]*\}\s+)?(/)?-->$`)

type delim struct {
	start, end   int
	closer, void bool
	name         string
	attrs        json.RawMessage
}

// nextDelim finds the next block delimiter at or after pos. WordPress
// escapes "--" inside attributes, so the first "-->" ends the comment.
func nextDelim(s string, pos int) (delim, bool) {
	for {
		i := strings.Index(s[pos:], "<!--")
		if i < 0 {
			return delim{}, false
		}
		start := pos + i
		j := strings.Index(s[start:], "-->")
		if j < 0 {
			return delim{}, false
		}
		end := start + j + len("-->")
		m := delimRe.FindStringSubmatch(s[start:end])
		if m == nil {
			pos = end
			continue
		}
		ns := m[2]
		if ns == "" {
			ns = "core/"
		}
		return delim{
			start:  start,
			end:    end,
			closer: m[1] != "",
			void:   m[5] != "",
			name:   ns + m[3],
			attrs:  json.RawMessage(strings.TrimSpace(m[4])),
		}, true
	}
}

// parseBlocks splits s into blocks. ok is false when s has no block
// delimiters at all (classic editor content).
func parseBlocks(s string) (root *block, ok bool) {
	root = &block{}
	stack := []*block{root}
	pos := 0
	for {
		d, found := nextDelim(s, pos)
		if !found {
			break
		}
		ok = true
		top := stack[len(stack)-1]
		top.addHTML(s[pos:d.start])
		pos = d.end
		switch {
		case d.void:
			top.inner = append(top.inner, piece{block: &block{name: d.name, attrs: d.attrs, src: s[d.start:d.end]}})
		case !d.closer:
			stack = append(stack, &block{name: d.name, attrs: d.attrs, start: d.start})
		case len(stack) > 1 && top.name == d.name:
			top.src = s[top.start:d.end]
			stack = stack[:len(stack)-1]
			parent := stack[len(stack)-1]
			parent.inner = append(parent.inner, piece{block: top})
		default:
			// A stray closer: keep it as (invisible) HTML.
			top.addHTML(s[d.start:d.end])
		}
	}
	stack[len(stack)-1].addHTML(s[pos:])
	// Close blocks left open at the end of the content.
	for len(stack) > 1 {
		top := stack[len(stack)-1]
		top.src = s[top.start:]
		stack = stack[:len(stack)-1]
		parent := stack[len(stack)-1]
		parent.inner = append(parent.inner, piece{block: top})
	}
	return root, ok
}

// innerText is the block's own HTML without nested blocks' delimiters.
func (b *block) innerText() string {
	var sb strings.Builder
	for _, p := range b.inner {
		if p.block != nil {
			sb.WriteString(p.block.innerText())
			continue
		}
		sb.WriteString(p.html)
	}
	return sb.String()
}

func (b *block) addHTML(s string) {
	if s != "" {
		b.inner = append(b.inner, piece{html: s})
	}
}

// attr returns a string attribute of the block.
func (b *block) attr(key string) string {
	var m map[string]any
	if json.Unmarshal(b.attrs, &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// htmlBlocks are blocks whose saved HTML maps onto the schema directly;
// their HTML is converted like any other HTML.
var htmlBlocks = map[string]bool{
	"core/paragraph": true,
	"core/heading":   true,
	"core/list":      true,
	"core/list-item": true,
	"core/quote":     true,
	"core/image":     true,
	"core/table":     true,
	"core/separator": true,
	"core/group":     true,
	"core/html":      true,
	"core/freeform":  true,
	"core/missing":   true,
	"core/classic":   true,
}

// blocksHTML flattens blocks back to HTML for the DOM converter. Blocks
// that need their own handling become placeholder elements.
func (c *conv) blocksHTML(b *block) string {
	var sb strings.Builder
	for _, p := range b.inner {
		if p.block == nil {
			sb.WriteString(c.shortcodes(p.html))
			continue
		}
		sb.WriteString(c.blockHTML(p.block))
	}
	return sb.String()
}

func (c *conv) blockHTML(b *block) string {
	switch {
	case htmlBlocks[b.name]:
		return c.blocksHTML(b)
	case b.name == "core/more":
		return "" // "read more" marker, no text
	case b.name == "core/nextpage":
		c.warn(WarnDropped, "ページ分割（wp:nextpage）は使えないため、1ページにまとめました", b.src)
		return ""
	case b.name == "core/embed" || strings.HasPrefix(b.name, "core-embed/"):
		url := b.attr("url")
		if p, ok := provider(url); ok {
			return c.placeholder(b.src, embed(p, url))
		}
		c.warn(WarnRawHTML, "埋め込み（"+url+"）は YouTube と Google マップ以外に対応していないため、カスタムHTMLとして残しました", b.src)
		return c.placeholder(b.src, rawHTML(b.src))
	case b.name == "core/shortcode":
		code := strings.TrimSpace(b.innerText())
		c.warn(WarnRawHTML, "ショートコード（"+firstLine(code)+"）は変換できないため、カスタムHTMLとして残しました", code)
		return c.placeholder(b.src, rawHTML(code))
	}
	c.warn(WarnRawHTML, "ブロック（wp:"+strings.TrimPrefix(b.name, "core/")+"）は変換できないため、カスタムHTMLとして残しました", b.src)
	return c.placeholder(b.src, rawHTML(b.src))
}

// shortcodeRe matches an opening shortcode tag: [name attrs] or [name/].
var shortcodeRe = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9_-]*)((?:\s[^\[\]]*)?)/?\]`)

// knownShortcodes are treated as shortcodes even without attributes or a
// closing tag. Other bracketed words such as [PR] or [注] stay text.
var knownShortcodes = map[string]bool{
	"caption": true, "wp_caption": true, "gallery": true, "embed": true,
	"audio": true, "video": true, "playlist": true, "contact-form-7": true,
	"contact-form": true, "gravityform": true, "mwform_formkey": true, "toc": true,
}

// shortcodes replaces shortcodes in s with placeholders holding a rawHtml
// node (or an embed for [embed]URL[/embed]).
func (c *conv) shortcodes(s string) string {
	var sb strings.Builder
	pos := 0
	for pos < len(s) {
		loc := shortcodeRe.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		name := s[pos+loc[2] : pos+loc[3]]
		attrs := strings.TrimSpace(s[pos+loc[4] : pos+loc[5]])
		escaped := start > 0 && s[start-1] == '[' && end < len(s) && s[end] == ']'
		closer := "[/" + name + "]"
		closeAt := -1
		if !strings.HasSuffix(s[start:end], "/]") {
			closeAt = strings.Index(s[end:], closer)
		}
		isShortcode := knownShortcodes[name] || closeAt >= 0 || strings.ContainsAny(attrs, `="'`)
		if escaped || !isShortcode || insideTag(s, start) {
			sb.WriteString(s[pos:end])
			pos = end
			continue
		}
		inner := ""
		if closeAt >= 0 {
			inner = s[end : end+closeAt]
			end += closeAt + len(closer)
		}
		sb.WriteString(s[pos:start])
		src := s[start:end]
		if name == "embed" {
			url := strings.TrimSpace(inner)
			if p, ok := provider(url); ok {
				sb.WriteString(c.placeholder(src, embed(p, url)))
				pos = end
				continue
			}
		}
		c.warn(WarnRawHTML, "ショートコード（["+name+"]）は変換できないため、カスタムHTMLとして残しました", src)
		sb.WriteString(c.placeholder(src, rawHTML(src)))
		pos = end
	}
	sb.WriteString(s[pos:])
	return sb.String()
}

// insideTag reports whether position i of s is inside an HTML tag (for
// example in an attribute value), where a placeholder would break markup.
func insideTag(s string, i int) bool {
	lt := strings.LastIndexByte(s[:i], '<')
	return lt >= 0 && strings.LastIndexByte(s[:i], '>') < lt
}
