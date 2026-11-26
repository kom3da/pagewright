package htmlpm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func p(content ...Node) Node          { return paragraph(content...) }
func tx(s string, marks ...Mark) Node { return text(s, marks...) }
func h(level int, content ...Node) Node {
	return Node{Type: "heading", Attrs: map[string]any{"level": level}, Content: content}
}
func ul(items ...Node) Node { return Node{Type: "bulletList", Content: items} }
func ol(start int, items ...Node) Node {
	return Node{Type: "orderedList", Attrs: map[string]any{"start": start}, Content: items}
}
func li(content ...Node) Node { return Node{Type: "listItem", Content: content} }
func img(src, alt string, mediaID any) Node {
	return Node{Type: "image", Attrs: map[string]any{"src": src, "alt": alt, "title": nil, "mediaId": mediaID}}
}
func cell(typ string, content ...Node) Node {
	return Node{Type: typ, Attrs: map[string]any{"colspan": 1, "rowspan": 1}, Content: content}
}

var (
	bold   = Mark{Type: "bold"}
	italic = Mark{Type: "italic"}
	code   = Mark{Type: "code"}
	br     = Node{Type: "hardBreak"}
)

func ruby(rt string) Mark { return Mark{Type: "ruby", Attrs: map[string]any{"rt": rt}} }

func TestConvert(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  []Node
		warns []string // warning codes
	}{
		{
			name: "classic wpautop paragraphs and line breaks",
			in:   "一行目\n二行目\n\n二段落目",
			want: []Node{p(tx("一行目"), br, tx("二行目")), p(tx("二段落目"))},
		},
		{
			name: "classic text around block elements",
			in:   "前の文\n<h2>見出し</h2>\n後の文 <strong>太字</strong>",
			want: []Node{p(tx("前の文")), h(2, tx("見出し")), p(tx("後の文 "), tx("太字", bold))},
		},
		{
			name: "gutenberg paragraph ignores source newlines",
			in:   "<!-- wp:paragraph -->\n<p>こんにちは\n世界</p>\n<!-- /wp:paragraph -->",
			want: []Node{p(tx("こんにちは 世界"))},
		},
		{
			name: "headings are mapped to 2 to 4",
			in:   "<h1>一</h1><h2>二</h2><h3>三</h3><h4>四</h4><h5>五</h5><h6>六</h6>",
			want: []Node{h(2, tx("一")), h(2, tx("二")), h(3, tx("三")), h(4, tx("四")), h(4, tx("五")), h(4, tx("六"))},
		},
		{
			name: "marks",
			in:   `<p><b>太</b><em>斜</em><i>斜</i><code>x</code><a href="/about/" target="_blank"><strong>リンク</strong></a></p>`,
			want: []Node{p(
				tx("太", bold), tx("斜斜", italic), tx("x", code),
				tx("リンク", Mark{Type: "link", Attrs: map[string]any{"href": "/about/", "target": "_blank"}}, bold),
			)},
		},
		{
			name: "ruby with rp and several bases",
			in:   `<p><ruby>漢<rp>(</rp><rt>かん</rt><rp>)</rp>字<rt>じ</rt></ruby>を読む</p>`,
			want: []Node{p(tx("漢", ruby("かん")), tx("字", ruby("じ")), tx("を読む"))},
		},
		{
			name: "nested lists",
			in:   "<ul><li>果物<ul><li>りんご</li><li>みかん</li></ul></li><li>野菜</li></ul><ol start=\"3\"><li>三</li></ol>",
			want: []Node{
				ul(
					li(p(tx("果物")), ul(li(p(tx("りんご"))), li(p(tx("みかん"))))),
					li(p(tx("野菜"))),
				),
				ol(3, li(p(tx("三")))),
			},
		},
		{
			name: "list nested directly in list joins the previous item",
			in:   "<ul><li>親</li><ul><li>子</li></ul></ul>",
			want: []Node{ul(li(p(tx("親")), ul(li(p(tx("子"))))))},
		},
		{
			name: "gutenberg nested list blocks",
			in:   "<!-- wp:list -->\n<ul class=\"wp-block-list\"><!-- wp:list-item -->\n<li>平日<!-- wp:list -->\n<ul class=\"wp-block-list\"><!-- wp:list-item -->\n<li>水曜定休</li>\n<!-- /wp:list-item --></ul>\n<!-- /wp:list --></li>\n<!-- /wp:list-item --></ul>\n<!-- /wp:list -->",
			want: []Node{ul(li(p(tx("平日")), ul(li(p(tx("水曜定休"))))))},
		},
		{
			name: "blockquote and hr",
			in:   "<blockquote><p>引用文</p><cite>出典</cite></blockquote><hr />",
			want: []Node{{Type: "blockquote", Content: []Node{p(tx("引用文")), p(tx("出典"))}}, {Type: "horizontalRule"}},
		},
		{
			name: "table with header, span and caption",
			in:   `<table><caption>料金表</caption><thead><tr><th>品名</th><th>価格</th></tr></thead><tbody><tr><td colspan="2">食パン <b>300円</b></td></tr></tbody></table>`,
			want: []Node{
				p(tx("料金表")),
				{Type: "table", Content: []Node{
					{Type: "tableRow", Content: []Node{cell("tableHeader", p(tx("品名"))), cell("tableHeader", p(tx("価格")))}},
					{Type: "tableRow", Content: []Node{{Type: "tableCell", Attrs: map[string]any{"colspan": 2, "rowspan": 1}, Content: []Node{p(tx("食パン "), tx("300円", bold))}}}},
				}},
			},
		},
		{
			name: "image inside paragraph splits it",
			in:   `<p>前<img src="https://example.jp/a.jpg" alt="写真">後</p>`,
			want: []Node{p(tx("前")), img("https://example.jp/a.jpg", "写真", nil), p(tx("後"))},
		},
		{
			name:  "image without alt falls back to file name",
			in:    `<figure><img src="https://example.jp/wp-content/uploads/2024/05/%E5%BA%97-300x200.jpg"><figcaption>店の写真</figcaption></figure>`,
			want:  []Node{img("https://example.jp/wp-content/uploads/2024/05/%E5%BA%97-300x200.jpg", "店", nil), p(tx("店の写真"))},
			warns: []string{WarnAltFallback},
		},
		{
			name:  "linked image loses a page link",
			in:    `<a href="/shop/"><img src="/a.png" alt="店"></a>`,
			want:  []Node{img("/a.png", "店", nil)},
			warns: []string{WarnLink},
		},
		{
			name: "image linked to its full size is fine",
			in:   `<a href="/a.png"><img src="/a-300x300.png" alt="店"></a>`,
			want: []Node{img("/a-300x300.png", "店", nil)},
		},
		{
			name:  "image without src is dropped with a warning",
			in:    `<p>文<img alt="x"></p>`,
			want:  []Node{p(tx("文"))},
			warns: []string{WarnDropped},
		},
		{
			name: "youtube and google maps iframes",
			in:   `<iframe src="https://www.youtube.com/embed/abc"></iframe><p><iframe src="//www.google.com/maps/embed?pb=1"></iframe></p>`,
			want: []Node{
				embed("youtube", "https://www.youtube.com/embed/abc"),
				embed("googlemap", "https://www.google.com/maps/embed?pb=1"),
			},
		},
		{
			name:  "other iframes stay raw",
			in:    `<iframe src="https://evil.example/x"></iframe>`,
			want:  []Node{rawHTML(`<iframe src="https://evil.example/x"></iframe>`)},
			warns: []string{WarnRawHTML},
		},
		{
			name: "gutenberg embed block",
			in:   "<!-- wp:embed {\"url\":\"https://youtu.be/abc\",\"providerNameSlug\":\"youtube\"} -->\n<figure class=\"wp-block-embed\"><div class=\"wp-block-embed__wrapper\">\nhttps://youtu.be/abc\n</div></figure>\n<!-- /wp:embed -->",
			want: []Node{embed("youtube", "https://youtu.be/abc")},
		},
		{
			name:  "unsupported embed block stays raw",
			in:    "<!-- wp:embed {\"url\":\"https://x.com/a/status/1\"} -->\n<figure>https://x.com/a/status/1</figure>\n<!-- /wp:embed -->",
			want:  []Node{rawHTML("<!-- wp:embed {\"url\":\"https://x.com/a/status/1\"} -->\n<figure>https://x.com/a/status/1</figure>\n<!-- /wp:embed -->")},
			warns: []string{WarnRawHTML},
		},
		{
			name: "classic URL alone on a line becomes an embed",
			in:   "動画です。\n\nhttps://www.youtube.com/watch?v=abc\n\nどうぞ。",
			want: []Node{p(tx("動画です。")), embed("youtube", "https://www.youtube.com/watch?v=abc"), p(tx("どうぞ。"))},
		},
		{
			name:  "shortcode in classic text",
			in:    "前の文\n\n[contact-form-7 id=\"5\" title=\"問い合わせ\"]\n\n後の文",
			want:  []Node{p(tx("前の文")), rawHTML(`[contact-form-7 id="5" title="問い合わせ"]`), p(tx("後の文"))},
			warns: []string{WarnRawHTML},
		},
		{
			name:  "enclosing caption shortcode spans tags",
			in:    `[caption id="a" width="300"]<img src="/a.jpg" alt="" /> 説明[/caption]続き`,
			want:  []Node{rawHTML(`[caption id="a" width="300"]<img src="/a.jpg" alt="" /> 説明[/caption]`), p(tx("続き"))},
			warns: []string{WarnRawHTML},
		},
		{
			name: "bracketed words are not shortcodes",
			in:   "<p>[PR] 本日のおすすめ [注]</p>",
			want: []Node{p(tx("[PR] 本日のおすすめ [注]"))},
		},
		{
			name: "embed shortcode with youtube",
			in:   "[embed]https://www.youtube.com/watch?v=abc[/embed]",
			want: []Node{embed("youtube", "https://www.youtube.com/watch?v=abc")},
		},
		{
			name:  "gutenberg shortcode block",
			in:    "<!-- wp:shortcode -->\n[gallery ids=\"1,2\"]\n<!-- /wp:shortcode -->",
			want:  []Node{rawHTML(`[gallery ids="1,2"]`)},
			warns: []string{WarnRawHTML},
		},
		{
			name:  "unknown block stays raw with its comments",
			in:    "<!-- wp:paragraph --><p>前</p><!-- /wp:paragraph --><!-- wp:latest-posts {\"postsToShow\":3} /--><!-- wp:acme/price-table --><div>価格表</div><!-- /wp:acme/price-table -->",
			want:  []Node{p(tx("前")), rawHTML(`<!-- wp:latest-posts {"postsToShow":3} /-->`), rawHTML(`<!-- wp:acme/price-table --><div>価格表</div><!-- /wp:acme/price-table -->`)},
			warns: []string{WarnRawHTML, WarnRawHTML},
		},
		{
			name:  "script stays raw",
			in:    "<p>文</p><script>alert(1)</script>",
			want:  []Node{p(tx("文")), rawHTML("<script>alert(1)</script>")},
			warns: []string{WarnRawHTML},
		},
		{
			name:  "strike-through keeps the paragraph raw",
			in:    "<p>価格 <del>1000円</del> 800円</p>",
			want:  []Node{rawHTML("<p>価格 <del>1000円</del> 800円</p>")},
			warns: []string{WarnRawHTML},
		},
		{
			name:  "colored text keeps the text",
			in:    `<p><span style="color:red">赤い</span>文字</p>`,
			want:  []Node{p(tx("赤い文字"))},
			warns: []string{WarnStyle},
		},
		{
			name:  "pre stays raw",
			in:    "<pre>a\n  b</pre>",
			want:  []Node{rawHTML("<pre>a\n  b</pre>")},
			warns: []string{WarnRawHTML},
		},
		{
			name:  "javascript link is removed",
			in:    `<p><a href="javascript:alert(1)">押す</a></p>`,
			want:  []Node{p(tx("押す"))},
			warns: []string{WarnLink},
		},
		{
			name: "empty and nbsp paragraphs are dropped",
			in:   "<p>&nbsp;</p><p></p><p>本文</p>",
			want: []Node{p(tx("本文"))},
		},
		{
			name: "more tag is dropped",
			in:   "前<!--more-->後",
			want: []Node{p(tx("前後"))},
		},
		{
			name: "div is transparent",
			in:   "<div><p>中身</p>外</div>",
			want: []Node{p(tx("中身")), p(tx("外"))},
		},
		{
			name: "line break in gutenberg paragraph",
			in:   "<!-- wp:paragraph -->\n<p>一<br>二</p>\n<!-- /wp:paragraph -->",
			want: []Node{p(tx("一"), br, tx("二"))},
		},
		{
			name: "empty input",
			in:   "",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warns := Convert(tt.in, Options{})
			if diff := cmp.Diff(doc(tt.want), got); diff != "" {
				t.Errorf("doc (-want +got):\n%s", diff)
			}
			var codes []string
			for _, w := range warns {
				codes = append(codes, w.Code)
			}
			if diff := cmp.Diff(tt.warns, codes); diff != "" {
				t.Errorf("warnings (-want +got):\n%s\n%v", diff, warns)
			}
		})
	}
}

func TestConvertImageOption(t *testing.T) {
	id := int64(7)
	opts := Options{Image: func(src string) ImageInfo {
		if strings.Contains(src, "shop") {
			return ImageInfo{Src: "/media/wp/2024/05/shop.jpg", MediaID: &id, Alt: "店の外観"}
		}
		return ImageInfo{}
	}}
	got, warns := Convert(`<img src="https://example.jp/shop-300x200.jpg" alt=""><img src="/b.png" alt="自前">`, opts)
	want := doc([]Node{img("/media/wp/2024/05/shop.jpg", "店の外観", int64(7)), img("/b.png", "自前", nil)})
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("doc (-want +got):\n%s", diff)
	}
	if len(warns) != 0 {
		t.Errorf("warnings: %v", warns)
	}
}

func TestConvertJSON(t *testing.T) {
	got, _ := Convert(`<p><ruby>東京<rt>とうきょう</rt></ruby>の会社です。</p>`, Options{})
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"東京","marks":[{"type":"ruby","attrs":{"rt":"とうきょう"}}]},{"type":"text","text":"の会社です。"}]}]}`
	if string(b) != want {
		t.Errorf("json:\n got %s\nwant %s", b, want)
	}
}

func TestConvertKeepsAllText(t *testing.T) {
	// Every character of visible text must survive conversion, either in
	// text nodes or inside rawHtml.
	in := "<p>あ<span>い</span><u>う</u><sup>え</sup></p><dl><dt>お</dt></dl><video>か</video>き\n\nく"
	got, _ := Convert(in, Options{})
	var sb strings.Builder
	var walk func(Node)
	walk = func(n Node) {
		sb.WriteString(n.Text)
		if h, ok := n.Attrs["html"].(string); ok {
			sb.WriteString(h)
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(got)
	for _, r := range "あいうえおかきく" {
		if !strings.ContainsRune(sb.String(), r) {
			t.Errorf("text %q lost; got %q", r, sb.String())
		}
	}
}
