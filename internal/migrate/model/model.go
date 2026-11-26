// Package model is the CMS-neutral intermediate form that importers read
// into and that tenant export will write. A source-specific parser (WordPress
// WXR today; Ghost, Markdown and others later) produces an Export, and one
// importer per target turns an Export into Pagewright rows.
//
// Conventions shared by every source:
//
//   - Slugs and paths are already decoded (no %XX) and NFC-normalized, so
//     Japanese URLs compare reliably. The raw source value is kept next to
//     them where the difference matters.
//   - Times are UTC. A zero time means the source had no usable value.
//   - Bodies are kept verbatim in the source's own format (Item.Body with
//     Item.BodyFormat); converting them is the importer's job.
//   - IDs are the source's own numeric IDs (SourceID). References between
//     items use those IDs; references to terms use slugs.
package model

import "time"

// Export is everything read from one source site.
type Export struct {
	// Source names the producer, e.g. "wordpress".
	Source      string
	Site        Site
	Authors     []Author
	Categories  []Term
	Tags        []Term
	Items       []Item
	Attachments []Attachment
}

// Site describes the source site as a whole.
type Site struct {
	Title       string
	Description string
	Language    string // e.g. "ja"
	// URL is the public base URL, e.g. "https://example.jp" or
	// "https://example.jp/blog" when the site lived in a subdirectory.
	URL string
}

// Author is a user of the source site. Importers may map authors to
// Pagewright users; the model keeps whatever the source exposes.
type Author struct {
	SourceID    int64
	Login       string
	Email       string
	DisplayName string
}

// Term is a category or a tag.
type Term struct {
	SourceID int64
	// Slug is decoded and NFC-normalized. RawSlug is as found in the source.
	Slug    string
	RawSlug string
	Name    string
	// ParentSlug is the decoded slug of the parent category; empty for top
	// level and for tags.
	ParentSlug  string
	Description string
}

// ItemType is the kind of a content item. Sources may produce other values
// (custom post types); importers decide what to do with them.
type ItemType string

// Item types every importer understands.
const (
	TypePost ItemType = "post"
	TypePage ItemType = "page"
)

// Status is the normalized publication status of an item.
type Status string

// Statuses. Sources map their own values onto these; RawStatus keeps the
// original.
const (
	StatusPublished Status = "published"
	StatusScheduled Status = "scheduled" // goes live at Item.Date
	StatusDraft     Status = "draft"
	StatusPending   Status = "pending" // waiting for review
	StatusPrivate   Status = "private" // published but visible only to editors
	StatusTrash     Status = "trash"
	StatusOther     Status = "other" // auto-drafts, revisions, unknown values
)

// BodyFormat names the format of Item.Body.
type BodyFormat string

// Body formats.
const (
	FormatWordPressHTML BodyFormat = "wordpress_html" // HTML with Gutenberg comments, shortcodes and wpautop line breaks
	FormatMarkdown      BodyFormat = "markdown"
	FormatHTML          BodyFormat = "html"
)

// Item is a post, page or other content entry (not an attachment).
type Item struct {
	SourceID int64
	// Type is the source type ("post", "page", or e.g. "nav_menu_item").
	Type      ItemType
	Status    Status
	RawStatus string
	Title     string
	// Slug is the decoded, NFC-normalized last URL segment; empty when the
	// source had none (common for drafts). RawSlug is as found.
	Slug    string
	RawSlug string
	// ParentID is the SourceID of the parent item, 0 for none.
	ParentID int64
	// Date is the publication date (or scheduled date) in UTC.
	Date     time.Time
	Modified time.Time
	// Link is the item's public URL on the source site, verbatim.
	Link string
	// Author is the source author's login.
	Author     string
	Body       string
	BodyFormat BodyFormat
	Excerpt    string
	// Categories and Tags reference terms by decoded slug. Terms an item
	// uses but the export does not declare are also in TermNames.
	Categories []string
	Tags       []string
	// TermNames maps a term slug used by this item to its display name.
	TermNames map[string]string
	// Meta holds custom fields (WordPress postmeta, ACF values) as found.
	// Keys starting with "_" are internal to the source CMS.
	Meta      map[string]string
	MenuOrder int
	// Password is set when the source protected the item with a password.
	Password     string
	Sticky       bool
	CommentCount int
}

// Attachment is an uploaded file (media library entry).
type Attachment struct {
	SourceID int64
	Title    string
	// URL is the file's absolute URL on the source site.
	URL      string
	Alt      string
	Caption  string
	ParentID int64 // item the file was uploaded to, 0 for none
	Date     time.Time
}
