// Package htmlpm converts WordPress post HTML into the ProseMirror (Tiptap)
// document JSON described in schema/body-v1.md.
//
// It understands Gutenberg block comments, classic-editor text that relies
// on wpautop (blank lines are paragraphs, single newlines are line breaks)
// and shortcodes. Whatever has no counterpart in the schema is kept as a
// rawHtml node holding the original fragment, and reported as a Warning, so
// no text is dropped silently.
package htmlpm

// SchemaVersion is the body schema version Convert produces.
const SchemaVersion = 1

// Node is a ProseMirror node. It marshals to the JSON stored in
// content_revisions.body.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

// Mark is a ProseMirror mark on a text node.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Warning describes something that was not converted faithfully.
type Warning struct {
	// Code is a stable identifier, one of the Warn* constants.
	Code string
	// Message is Japanese text for the person running the import.
	Message string
	// Fragment is the source fragment concerned, shortened.
	Fragment string
}

// Warning codes.
const (
	WarnRawHTML     = "raw_html"     // kept as a rawHtml node
	WarnAltFallback = "alt_fallback" // image alt text was missing
	WarnDropped     = "dropped"      // something without text was left out
	WarnStyle       = "style"        // inline formatting was removed, text kept
	WarnLink        = "link"         // a link was removed, text kept
)

func doc(content []Node) Node {
	return Node{Type: "doc", Content: content}
}

func paragraph(content ...Node) Node {
	return Node{Type: "paragraph", Content: content}
}

func text(s string, marks ...Mark) Node {
	return Node{Type: "text", Text: s, Marks: marks}
}

func rawHTML(html string) Node {
	return Node{Type: "rawHtml", Attrs: map[string]any{"html": html}}
}

func embed(provider, url string) Node {
	return Node{Type: "embed", Attrs: map[string]any{"provider": provider, "url": url}}
}
