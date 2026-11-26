package body

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty doc", `{"type":"doc","content":[]}`, false},
		{"paragraph with ruby", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"東京","marks":[{"type":"ruby","attrs":{"rt":"とうきょう"}}]}]}]}`, false},
		{"heading h2", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"見出し"}]}]}`, false},
		{"nested list", `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]},{"type":"orderedList","attrs":{"start":1},"content":[{"type":"listItem","content":[{"type":"paragraph"}]}]}]}]}]}`, false},
		{"image", `{"type":"doc","content":[{"type":"image","attrs":{"src":"/media/a.jpg","alt":"社屋の外観","mediaId":3}}]}`, false},
		{"table", `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph"}]}]}]}]}`, false},
		{"rawHtml", `{"type":"doc","content":[{"type":"rawHtml","attrs":{"html":"[gallery]"}}]}`, false},
		{"link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"https://example.com","target":"_blank"}}]}]}]}`, false},

		{"not json", `{`, true},
		{"not doc", `{"type":"paragraph"}`, true},
		{"h1", `{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"x"}]}]}`, true},
		{"unknown node", `{"type":"doc","content":[{"type":"iframe"}]}`, true},
		{"text at top", `{"type":"doc","content":[{"type":"text","text":"x"}]}`, true},
		{"image without alt", `{"type":"doc","content":[{"type":"image","attrs":{"src":"/media/a.jpg","alt":" "}}]}`, true},
		{"javascript link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`, true},
		{"tel link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"tel:03-1234-5678"}}]}]}]}`, false},
		{"tel link with text", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"tel:call-me"}}]}]}]}`, true},
		{"color mark", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"textStyle","attrs":{"color":"red"}}]}]}]}`, true},
		{"ruby without rt", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"ruby","attrs":{"rt":""}}]}]}]}`, true},
		{"embed other site", `{"type":"doc","content":[{"type":"embed","attrs":{"provider":"vimeo","url":"https://vimeo.com/1"}}]}`, true},
		{"empty text", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}`, true},
		{"child in leaf", `{"type":"doc","content":[{"type":"horizontalRule","content":[{"type":"paragraph"}]}]}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRawHTML(t *testing.T) {
	doc, err := Parse([]byte(`{"type":"doc","content":[{"type":"rawHtml","attrs":{"html":"a"}},{"type":"blockquote","content":[{"type":"rawHtml","attrs":{"html":"b"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"a", "b"}, RawHTML(doc)); diff != "" {
		t.Errorf("RawHTML (-want +got):\n%s", diff)
	}
}
