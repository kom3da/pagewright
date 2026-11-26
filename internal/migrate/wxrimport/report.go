package wxrimport

import (
	"fmt"
	"io"
	"strings"

	"github.com/kom3da/pagewright/internal/migrate/htmlpm"
)

// Outcome is what happened to one entry of the export.
type Outcome string

// Outcomes.
const (
	Created  Outcome = "created"  // newly imported
	Existing Outcome = "existing" // already there (an earlier import); left untouched
	Skipped  Outcome = "skipped"  // deliberately not imported (trash, menus, ...)
	Failed   Outcome = "failed"   // an error rolled this entry back
)

// Entry kinds besides WordPress post types.
const (
	KindCategory = "category"
	KindTag      = "tag"
	KindMedia    = "media"
)

// Warning codes added by the importer (htmlpm adds its own).
const (
	WarnURL    = "url"    // an old URL could not be kept or a path collided
	WarnSlug   = "slug"   // the slug could not be used
	WarnParent = "parent" // a parent page or category was not found
	WarnStatus = "status" // the status was changed on import
	WarnTerm   = "term"   // a category or tag was not found
	WarnField  = "field"  // a custom field was not imported
)

// Entry reports on one item, term or attachment.
type Entry struct {
	// Kind is the WordPress post type ("post", "page", "nav_menu_item"...)
	// or one of the Kind* constants.
	Kind     string
	SourceID int64
	Title    string
	Outcome  Outcome
	// Reason explains Skipped, Existing and Failed outcomes.
	Reason string
	// Path is the canonical public path given to the entry, if any.
	Path     string
	Warnings []htmlpm.Warning
}

func (e *Entry) warn(code, msg, fragment string) {
	e.Warnings = append(e.Warnings, htmlpm.Warning{Code: code, Message: msg, Fragment: fragment})
}

// Counts tallies outcomes of one kind.
type Counts struct {
	Created, Existing, Skipped, Failed int
}

func (c *Counts) add(o Outcome) {
	switch o {
	case Created:
		c.Created++
	case Existing:
		c.Existing++
	case Skipped:
		c.Skipped++
	case Failed:
		c.Failed++
	}
}

// Report is the result of an import or a dry run. A dry run produces the
// same report as the real import would.
type Report struct {
	DryRun  bool
	Site    string
	SiteURL string

	Posts, Pages, Other Counts
	Categories, Tags    Counts
	Media               Counts
	Published, Drafts   int
	Scheduled           int
	Routes, Redirects   int
	Entries             []Entry
	Notes               []string
}

func (r *Report) add(e Entry) {
	switch e.Kind {
	case "post":
		r.Posts.add(e.Outcome)
	case "page":
		r.Pages.add(e.Outcome)
	case KindCategory:
		r.Categories.add(e.Outcome)
	case KindTag:
		r.Tags.add(e.Outcome)
	case KindMedia:
		r.Media.add(e.Outcome)
	default:
		r.Other.add(e.Outcome)
	}
	r.Entries = append(r.Entries, e)
}

// Warnings returns the number of warnings over all entries.
func (r *Report) Warnings() int {
	n := 0
	for _, e := range r.Entries {
		n += len(e.Warnings)
	}
	return n
}

var kindNames = map[string]string{
	"post":       "記事",
	"page":       "固定ページ",
	KindCategory: "カテゴリ",
	KindTag:      "タグ",
	KindMedia:    "画像",
}

func kindName(k string) string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return "その他（" + k + "）"
}

// WriteText prints the report as Japanese text: a summary, then every
// entry that needs a look (warnings, skips, failures).
func (r *Report) WriteText(w io.Writer) error {
	var b strings.Builder
	if r.DryRun {
		b.WriteString("WordPress からの取り込み（ドライラン：データベースには何も書き込んでいません）\n")
	} else {
		b.WriteString("WordPress からの取り込み結果\n")
	}
	if r.Site != "" || r.SiteURL != "" {
		fmt.Fprintf(&b, "サイト：%s（%s）\n", r.Site, r.SiteURL)
	}
	b.WriteString("\n")
	line := func(label string, c Counts, existing string) {
		fmt.Fprintf(&b, "%s：新規 %d、%s %d、対象外 %d、失敗 %d\n", label, c.Created, existing, c.Existing, c.Skipped, c.Failed)
	}
	line("記事", r.Posts, "取り込み済み")
	line("固定ページ", r.Pages, "取り込み済み")
	if r.Other != (Counts{}) {
		line("その他", r.Other, "取り込み済み")
	}
	line("カテゴリ", r.Categories, "既存")
	line("タグ", r.Tags, "既存")
	line("画像", r.Media, "既存")
	fmt.Fprintf(&b, "状態：公開 %d、下書き %d、予約 %d\n", r.Published, r.Drafts, r.Scheduled)
	fmt.Fprintf(&b, "URL：公開URL %d、リダイレクト %d\n", r.Routes, r.Redirects)
	fmt.Fprintf(&b, "確認が必要な点：%d 件\n", r.Warnings())

	var attention []Entry
	for _, e := range r.Entries {
		if e.Outcome != Created || len(e.Warnings) > 0 {
			attention = append(attention, e)
		}
	}
	if len(attention) > 0 {
		b.WriteString("\n■ 確認が必要なもの\n")
	}
	for _, e := range attention {
		fmt.Fprintf(&b, "\n[%s %d] %s", kindName(e.Kind), e.SourceID, e.Title)
		if e.Path != "" {
			fmt.Fprintf(&b, "　%s", e.Path)
		}
		b.WriteString("\n")
		switch e.Outcome {
		case Skipped:
			fmt.Fprintf(&b, "  対象外：%s\n", e.Reason)
		case Existing:
			fmt.Fprintf(&b, "  取り込み済みのため変更していません：%s\n", e.Reason)
		case Failed:
			fmt.Fprintf(&b, "  失敗（この項目は取り込まれていません）：%s\n", e.Reason)
		}
		for _, wn := range e.Warnings {
			fmt.Fprintf(&b, "  - %s\n", wn.Message)
			if wn.Fragment != "" && !strings.Contains(wn.Message, wn.Fragment) {
				fmt.Fprintf(&b, "      元の内容：%s\n", oneLine(wn.Fragment))
			}
		}
	}
	if len(r.Notes) > 0 {
		b.WriteString("\n■ 補足\n")
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
