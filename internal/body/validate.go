// Package body checks body JSON against schema/body-v1.md.
package body

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// SchemaVersion is the body schema version this package validates.
const SchemaVersion = 1

// EmptyDoc is an empty body.
var EmptyDoc = json.RawMessage(`{"type":"doc","content":[]}`)

// Node is one ProseMirror node.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Text    *string        `json:"text,omitempty"`
}

// Mark is one ProseMirror mark.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Error is a validation error with a message for the admin UI.
type Error struct {
	Path    string // e.g. "content[2].content[0]"
	Message string // Japanese, shown to the user
}

func (e *Error) Error() string { return e.Path + ": " + e.Message }

// Parse decodes and validates a body. It returns the decoded document.
func Parse(raw []byte) (Node, error) {
	var doc Node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Node{}, &Error{Path: "", Message: "本文の形式が正しくありません"}
	}
	if doc.Type != "doc" {
		return Node{}, &Error{Path: "", Message: "本文の形式が正しくありません"}
	}
	if err := checkChildren(doc, "", blockKind); err != nil {
		return Node{}, err
	}
	return doc, nil
}

// RawHTML returns the html of every rawHtml node in doc, in order.
func RawHTML(doc Node) []string {
	var out []string
	var walk func(Node)
	walk = func(n Node) {
		if n.Type == "rawHtml" {
			h, _ := n.Attrs["html"].(string)
			out = append(out, h)
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(doc)
	return out
}

type kind int

const (
	blockKind kind = iota
	inlineKind
	listItemKind
	rowKind
	cellKind
)

// children lists, per node type, the kind of nodes it may contain.
// Leaf nodes are absent.
var children = map[string]kind{
	"doc":         blockKind,
	"paragraph":   inlineKind,
	"heading":     inlineKind,
	"bulletList":  listItemKind,
	"orderedList": listItemKind,
	"listItem":    blockKind,
	"blockquote":  blockKind,
	"table":       rowKind,
	"tableRow":    cellKind,
	"tableHeader": blockKind,
	"tableCell":   blockKind,
}

var kinds = map[kind]map[string]bool{
	blockKind: {
		"paragraph": true, "heading": true, "bulletList": true, "orderedList": true,
		"blockquote": true, "horizontalRule": true, "image": true, "table": true,
		"embed": true, "rawHtml": true, "contactForm": true,
	},
	inlineKind:   {"text": true, "hardBreak": true},
	listItemKind: {"listItem": true},
	rowKind:      {"tableRow": true},
	cellKind:     {"tableHeader": true, "tableCell": true},
}

func checkChildren(n Node, path string, k kind) error {
	for i, c := range n.Content {
		p := fmt.Sprintf("%scontent[%d]", prefix(path), i)
		if !kinds[k][c.Type] {
			return &Error{Path: p, Message: fmt.Sprintf("「%s」はここには置けません", c.Type)}
		}
		if err := checkNode(c, p); err != nil {
			return err
		}
	}
	return nil
}

func prefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "."
}

func checkNode(n Node, path string) error {
	fail := func(msg string) error { return &Error{Path: path, Message: msg} }
	if n.Type != "text" && (n.Text != nil || len(n.Marks) > 0) {
		return fail("文字の装飾は文字にだけ付けられます")
	}
	switch n.Type {
	case "text":
		if n.Text == nil || *n.Text == "" {
			return fail("空の文字は保存できません")
		}
		for _, m := range n.Marks {
			if err := checkMark(m); err != nil {
				return &Error{Path: path, Message: err.Error()}
			}
		}
		return nil
	case "heading":
		if l, ok := number(n.Attrs["level"]); !ok || l < 2 || l > 4 {
			return fail("見出しは大・中・小（h2〜h4）から選んでください")
		}
	case "orderedList":
		if v, present := n.Attrs["start"]; present && v != nil {
			if s, ok := number(v); !ok || s < 1 {
				return fail("番号付きリストの開始番号が正しくありません")
			}
		}
	case "image":
		src, _ := n.Attrs["src"].(string)
		alt, _ := n.Attrs["alt"].(string)
		if src == "" {
			return fail("画像が選ばれていません")
		}
		if strings.TrimSpace(alt) == "" {
			return fail("画像の代替テキスト（画像の説明）を入力してください")
		}
		if v, present := n.Attrs["mediaId"]; present && v != nil {
			if _, ok := number(v); !ok {
				return fail("画像の指定が正しくありません")
			}
		}
	case "embed":
		p, _ := n.Attrs["provider"].(string)
		u, _ := n.Attrs["url"].(string)
		if p != "youtube" && p != "googlemap" {
			return fail("埋め込めるのはYouTubeとGoogleマップだけです")
		}
		if !httpsURL(u) {
			return fail("埋め込みのURLが正しくありません")
		}
	case "rawHtml":
		if _, ok := n.Attrs["html"].(string); !ok {
			return fail("HTMLブロックの中身がありません")
		}
	case "tableHeader", "tableCell":
		for _, a := range []string{"colspan", "rowspan"} {
			if v, present := n.Attrs[a]; present && v != nil {
				if s, ok := number(v); !ok || s < 1 {
					return fail("表のセルの結合が正しくありません")
				}
			}
		}
	}
	k, hasChildren := children[n.Type]
	if !hasChildren {
		if len(n.Content) > 0 {
			return fail(fmt.Sprintf("「%s」の中には何も入れられません", n.Type))
		}
		return nil
	}
	return checkChildren(n, path, k)
}

func checkMark(m Mark) error {
	switch m.Type {
	case "bold", "italic", "code":
		return nil
	case "link":
		href, _ := m.Attrs["href"].(string)
		if !safeHref(href) {
			return errors.New("リンク先のURLが正しくありません")
		}
		if t, present := m.Attrs["target"]; present && t != nil && t != "_blank" {
			return errors.New("リンクの開き方が正しくありません")
		}
		return nil
	case "ruby":
		if rt, _ := m.Attrs["rt"].(string); strings.TrimSpace(rt) == "" {
			return errors.New("ルビ（ふりがな）を入力してください")
		}
		return nil
	}
	return fmt.Errorf("「%s」という装飾は使えません", m.Type)
}

func number(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok && f == float64(int64(f))
}

func httpsURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// safeHref allows http(s), mailto and site-relative links.
func safeHref(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") {
		return true
	}
	if strings.HasPrefix(s, "#") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Host != ""
	case "mailto":
		return u.Opaque != ""
	}
	return false
}
