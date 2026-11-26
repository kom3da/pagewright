package render_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/kom3da/pagewright/internal/render"
)

// doc wraps block JSON in a doc node.
func doc(blocks ...string) string {
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

func para(inline ...string) string {
	return `{"type":"paragraph","content":[` + strings.Join(inline, ",") + `]}`
}

func text(s string, marks ...string) string {
	if len(marks) == 0 {
		return `{"type":"text","text":` + quote(s) + `}`
	}
	return `{"type":"text","text":` + quote(s) + `,"marks":[` + strings.Join(marks, ",") + `]}`
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

func TestBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantHTML string
		wantText string
	}{
		{
			name:     "empty doc",
			body:     doc(),
			wantHTML: "",
		},
		{
			name:     "paragraph escapes text",
			body:     doc(para(text(`<script>alert("x")</script> & 'q'`))),
			wantHTML: "<p>&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &amp; &#39;q&#39;</p>\n",
			wantText: `<script>alert("x")</script> & 'q'`,
		},
		{
			name:     "headings are clamped to h2-h4",
			body:     doc(`{"type":"heading","attrs":{"level":1},"content":[`+text("a")+`]}`, `{"type":"heading","attrs":{"level":3},"content":[`+text("b")+`]}`, `{"type":"heading","attrs":{"level":6},"content":[`+text("c")+`]}`),
			wantHTML: "<h2>a</h2>\n<h3>b</h3>\n<h4>c</h4>\n",
			wantText: "a\nb\nc",
		},
		{
			name:     "marks",
			body:     doc(para(text("太字", `{"type":"bold"}`), text("斜体", `{"type":"italic"}`), text("x<y", `{"type":"code"}`))),
			wantHTML: "<p><strong>太字</strong><em>斜体</em><code>x&lt;y</code></p>\n",
			wantText: "太字斜体x<y",
		},
		{
			name:     "ruby",
			body:     doc(para(text("東京", `{"type":"ruby","attrs":{"rt":"とうきょう"}}`), text("の会社"))),
			wantHTML: "<p><ruby>東京<rp>(</rp><rt>とうきょう</rt><rp>)</rp></ruby>の会社</p>\n",
			wantText: "東京の会社",
		},
		{
			name:     "ruby without rt keeps text",
			body:     doc(para(text("東京", `{"type":"ruby","attrs":{"rt":" "}}`))),
			wantHTML: "<p>東京</p>\n",
			wantText: "東京",
		},
		{
			name:     "ruby is inside other marks and rt is escaped",
			body:     doc(para(text("漢", `{"type":"ruby","attrs":{"rt":"<b>"}}`, `{"type":"bold"}`))),
			wantHTML: "<p><strong><ruby>漢<rp>(</rp><rt>&lt;b&gt;</rt><rp>)</rp></ruby></strong></p>\n",
			wantText: "漢",
		},
		{
			name:     "link spans adjacent nodes",
			body:     doc(para(text("a", `{"type":"link","attrs":{"href":"https://example.com/?a=1&b=2"}}`), text("b", `{"type":"link","attrs":{"href":"https://example.com/?a=1&b=2"}}`, `{"type":"bold"}`))),
			wantHTML: `<p><a href="https://example.com/?a=1&amp;b=2">a<strong>b</strong></a></p>` + "\n",
			wantText: "ab",
		},
		{
			name:     "link target blank gets rel",
			body:     doc(para(text("a", `{"type":"link","attrs":{"href":"/company/","target":"_blank"}}`))),
			wantHTML: `<p><a href="/company/" target="_blank" rel="noopener noreferrer">a</a></p>` + "\n",
			wantText: "a",
		},
		{
			name:     "other link targets are ignored",
			body:     doc(para(text("a", `{"type":"link","attrs":{"href":"mailto:info@example.com","target":"_top"}}`))),
			wantHTML: `<p><a href="mailto:info@example.com">a</a></p>` + "\n",
			wantText: "a",
		},
		{
			name:     "javascript link is dropped",
			body:     doc(para(text("a", `{"type":"link","attrs":{"href":" JavaScript:alert(1)"}}`))),
			wantHTML: "<p>a</p>\n",
			wantText: "a",
		},
		{
			name:     "data link is dropped",
			body:     doc(para(text("a", `{"type":"link","attrs":{"href":"data:text/html,<b>"}}`))),
			wantHTML: "<p>a</p>\n",
			wantText: "a",
		},
		{
			name:     "link with control characters is dropped",
			body:     doc(para(text("a", `{"type":"link","attrs":{"href":"java\tscript:alert(1)"}}`))),
			wantHTML: "<p>a</p>\n",
			wantText: "a",
		},
		{
			name:     "unknown mark and attrs are dropped",
			body:     doc(para(text("a", `{"type":"textStyle","attrs":{"color":"red"}}`, `{"type":"bold","attrs":{"onclick":"x"}}`))),
			wantHTML: "<p><strong>a</strong></p>\n",
			wantText: "a",
		},
		{
			name:     "hard break keeps marks open",
			body:     doc(para(text("a", `{"type":"bold"}`), `{"type":"hardBreak"}`, text("b", `{"type":"bold"}`))),
			wantHTML: "<p><strong>a<br>b</strong></p>\n",
			wantText: "a\nb",
		},
		{
			name:     "lists",
			body:     doc(`{"type":"bulletList","content":[{"type":"listItem","content":[`+para(text("a"))+`]}]}`, `{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[`+para(text("b"))+`]},{"type":"paragraph"}]}`),
			wantHTML: "<ul>\n<li><p>a</p>\n</li>\n</ul>\n<ol start=\"3\">\n<li><p>b</p>\n</li>\n</ol>\n",
			wantText: "a\nb",
		},
		{
			name:     "blockquote and rule",
			body:     doc(`{"type":"blockquote","content":[`+para(text("q"))+`]}`, `{"type":"horizontalRule"}`),
			wantHTML: "<blockquote>\n<p>q</p>\n</blockquote>\n<hr>\n",
			wantText: "q",
		},
		{
			name:     "image",
			body:     doc(`{"type":"image","attrs":{"src":"/media/a/b.jpg","alt":"社屋の\"外観\"","mediaId":7,"title":null}}`),
			wantHTML: `<figure class="image"><img src="/media/a/b.jpg" alt="社屋の&#34;外観&#34;" width="800" height="600" loading="lazy" decoding="async"></figure>` + "\n",
		},
		{
			name:     "image without alt is dropped",
			body:     doc(`{"type":"image","attrs":{"src":"/media/a.jpg","alt":""}}`),
			wantHTML: "",
		},
		{
			name:     "image with javascript src is dropped",
			body:     doc(`{"type":"image","attrs":{"src":"javascript:alert(1)","alt":"x"}}`),
			wantHTML: "",
		},
		{
			name:     "image with unknown attrs",
			body:     doc(`{"type":"image","attrs":{"src":"https://example.com/a.png","alt":"a","onerror":"alert(1)","title":"t"}}`),
			wantHTML: `<figure class="image"><img src="https://example.com/a.png" alt="a" title="t" loading="lazy" decoding="async"></figure>` + "\n",
		},
		{
			name: "table",
			body: doc(`{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":2,"rowspan":1},"content":[` + para(text("h")) + `]}]},` +
				`{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":1,"rowspan":1,"style":"x"},"content":[` + para(text("c")) + `]}]}]}`),
			wantHTML: "<div class=\"table-wrap\"><table>\n<tbody>\n<tr><th colspan=\"2\"><p>h</p>\n</th></tr>\n<tr><td><p>c</p>\n</td></tr>\n</tbody>\n</table></div>\n",
			wantText: "h\nc",
		},
		{
			name:     "youtube embed",
			body:     doc(`{"type":"embed","attrs":{"provider":"youtube","url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10"}}`),
			wantHTML: `<div class="embed embed-youtube"><iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ" title="YouTube動画" width="560" height="315" loading="lazy" referrerpolicy="strict-origin-when-cross-origin" allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe></div>` + "\n",
		},
		{
			name:     "youtube embed with bad id is dropped",
			body:     doc(`{"type":"embed","attrs":{"provider":"youtube","url":"https://www.youtube.com/watch?v=\"><script>"}}`),
			wantHTML: "",
		},
		{
			name:     "google map embed",
			body:     doc(`{"type":"embed","attrs":{"provider":"googlemap","url":"https://www.google.com/maps/embed?pb=!1m18!1m12\"><x"}}`),
			wantHTML: `<div class="embed embed-map"><iframe src="https://www.google.com/maps/embed?pb=%211m18%211m12%22%3E%3Cx" title="Googleマップ" width="600" height="450" loading="lazy" referrerpolicy="no-referrer-when-downgrade"></iframe></div>` + "\n",
		},
		{
			name:     "embed from other provider is dropped",
			body:     doc(`{"type":"embed","attrs":{"provider":"iframe","url":"https://evil.example/"}}`),
			wantHTML: "",
		},
		{
			name:     "raw html is sanitized",
			body:     doc(`{"type":"rawHtml","attrs":{"html":"<p onclick=\"x()\">a &amp; b<script>alert(1)</script></p><iframe src=\"https://evil.example/\"></iframe>"}}`),
			wantHTML: `<div class="raw-html"><p>a &amp; b</p></div>` + "\n",
			wantText: "a & b",
		},
		{
			name:     "unknown block is dropped",
			body:     doc(`{"type":"button","attrs":{"href":"/x"},"content":[`+text("x")+`]}`, para(text("ok"))),
			wantHTML: "<p>ok</p>\n",
			wantText: "ok",
		},
		{
			name:     "text at block level is dropped",
			body:     doc(text("<b>")),
			wantHTML: "",
		},
	}
	opts := render.Options{MediaSize: func(id int64) (int64, int64, bool) { return 800, 600, id == 7 }}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := render.Body([]byte(tt.body), opts)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.wantHTML, string(got.HTML)); diff != "" {
				t.Errorf("HTML mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantText, got.Text); diff != "" {
				t.Errorf("Text mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBodyMedia(t *testing.T) {
	body := doc(
		`{"type":"image","attrs":{"src":"/media/2026/%E5%86%99%E7%9C%9F.jpg","alt":"a","mediaId":3}}`,
		`{"type":"image","attrs":{"src":"https://example.com/x.png","alt":"b","mediaId":null}}`,
	)
	got, err := render.Body([]byte(body), render.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int64{3}, got.MediaIDs); diff != "" {
		t.Errorf("MediaIDs (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"2026/写真.jpg"}, got.MediaKeys); diff != "" {
		t.Errorf("MediaKeys (-want +got):\n%s", diff)
	}
}

func TestBodyErrors(t *testing.T) {
	for _, body := range []string{``, `[]`, `{"type":"paragraph"}`} {
		if _, err := render.Body([]byte(body), render.Options{}); err == nil {
			t.Errorf("Body(%q) = nil error, want error", body)
		}
	}
}

func TestBodyDepthLimit(t *testing.T) {
	inner := para(text("deep"))
	for range render.MaxDepth + 5 {
		inner = `{"type":"blockquote","content":[` + inner + `]}`
	}
	got, err := render.Body([]byte(doc(inner)), render.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got.HTML), "deep") || len(got.Warnings) == 0 {
		t.Errorf("deep content rendered or not reported: warnings %v", got.Warnings)
	}
}

func TestYouTubeID(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?si=abc", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ/", "dQw4w9WgXcQ"},
		{"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://evil.example/watch?v=dQw4w9WgXcQ", ""},
		{"javascript:alert(1)//youtu.be/dQw4w9WgXcQ", ""},
		{"https://youtu.be/short", ""},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ/../x", ""},
	}
	for _, tt := range tests {
		got, _ := render.YouTubeID(tt.url)
		if got != tt.want {
			t.Errorf("YouTubeID(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestGoogleMapEmbedURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.google.com/maps/embed?pb=!1m18", "https://www.google.com/maps/embed?pb=%211m18"},
		{"https://www.google.co.jp/maps?q=東京駅", "https://www.google.com/maps?output=embed&q=%E6%9D%B1%E4%BA%AC%E9%A7%85"},
		{"https://maps.google.com/?q=Tokyo", "https://www.google.com/maps?output=embed&q=Tokyo"},
		{"https://www.google.com.evil.example/maps/embed?pb=x", ""},
		{"https://www.google.com/search?q=x", ""},
		{"https://www.google.com/maps/embed", ""},
	}
	for _, tt := range tests {
		got, _ := render.GoogleMapEmbedURL(tt.url)
		if got != tt.want {
			t.Errorf("GoogleMapEmbedURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
