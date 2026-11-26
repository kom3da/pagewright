// Package wxr reads WordPress eXtended RSS (WXR) export files into the
// intermediate model (internal/migrate/model). It streams with
// encoding/xml: each author, term and item is decoded on its own, so memory
// does not grow with the document tree. ReadAll collects everything for
// importers that need the whole export at once.
//
// WXR versions 1.0 to 1.2 are accepted; elements are matched by local name.
package wxr

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kom3da/pagewright/internal/migrate/model"
)

// Entry is one of SiteEntry, AuthorEntry, CategoryEntry, TagEntry,
// ItemEntry or AttachmentEntry.
type Entry interface{ isEntry() }

// Entry types returned by Reader.Next.
type (
	SiteEntry       struct{ model.Site }
	AuthorEntry     struct{ model.Author }
	CategoryEntry   struct{ model.Term }
	TagEntry        struct{ model.Term }
	ItemEntry       struct{ model.Item }
	AttachmentEntry struct{ model.Attachment }
)

func (SiteEntry) isEntry()       {}
func (AuthorEntry) isEntry()     {}
func (CategoryEntry) isEntry()   {}
func (TagEntry) isEntry()        {}
func (ItemEntry) isEntry()       {}
func (AttachmentEntry) isEntry() {}

// Reader returns the entries of a WXR file one at a time.
type Reader struct {
	dec      *xml.Decoder
	site     model.Site
	siteDone bool
	pending  Entry
}

// NewReader returns a Reader for r.
func NewReader(r io.Reader) *Reader {
	dec := xml.NewDecoder(r)
	// WordPress exports are not always strictly valid XML (HTML entities,
	// stray characters); be lenient like WordPress's own importer.
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	return &Reader{dec: dec}
}

// Next returns the next entry, or io.EOF after the last one. The SiteEntry
// comes first, once: WXR puts the site elements before everything else.
func (r *Reader) Next() (Entry, error) {
	if r.pending != nil {
		e := r.pending
		r.pending = nil
		return e, nil
	}
	for {
		tok, err := r.dec.Token()
		if errors.Is(err, io.EOF) {
			if !r.siteDone {
				r.siteDone = true
				return SiteEntry{r.site}, nil
			}
			return nil, io.EOF
		}
		if err != nil {
			return nil, fmt.Errorf("wxr: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		entry, err := r.element(se)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			continue
		}
		if !r.siteDone {
			r.siteDone = true
			r.pending = entry
			return SiteEntry{r.site}, nil
		}
		return entry, nil
	}
}

// ReadAll reads the whole export.
func ReadAll(rd io.Reader) (*model.Export, error) {
	r := NewReader(rd)
	exp := &model.Export{Source: "wordpress"}
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			return exp, nil
		}
		if err != nil {
			return nil, err
		}
		switch e := e.(type) {
		case SiteEntry:
			exp.Site = e.Site
		case AuthorEntry:
			exp.Authors = append(exp.Authors, e.Author)
		case CategoryEntry:
			exp.Categories = append(exp.Categories, e.Term)
		case TagEntry:
			exp.Tags = append(exp.Tags, e.Term)
		case ItemEntry:
			exp.Items = append(exp.Items, e.Item)
		case AttachmentEntry:
			exp.Attachments = append(exp.Attachments, e.Attachment)
		}
	}
}

// element handles a start element outside items. It returns nil for
// elements that are not entries.
func (r *Reader) element(se xml.StartElement) (Entry, error) {
	switch se.Name.Local {
	case "rss", "channel":
		return nil, nil
	case "item":
		return r.item(se)
	case "author":
		var a rawAuthor
		if err := r.dec.DecodeElement(&a, &se); err != nil {
			return nil, fmt.Errorf("wxr: author: %w", err)
		}
		return AuthorEntry{model.Author{
			SourceID:    atoi(a.ID),
			Login:       a.Login,
			Email:       a.Email,
			DisplayName: a.DisplayName,
		}}, nil
	case "category":
		var c rawCategory
		if err := r.dec.DecodeElement(&c, &se); err != nil {
			return nil, fmt.Errorf("wxr: category: %w", err)
		}
		return CategoryEntry{model.Term{
			SourceID:    atoi(c.ID),
			Slug:        model.DecodeSlug(strings.TrimSpace(c.Slug)),
			RawSlug:     c.Slug,
			Name:        c.Name,
			ParentSlug:  model.DecodeSlug(strings.TrimSpace(c.Parent)),
			Description: c.Description,
		}}, nil
	case "tag":
		var t rawTag
		if err := r.dec.DecodeElement(&t, &se); err != nil {
			return nil, fmt.Errorf("wxr: tag: %w", err)
		}
		return TagEntry{model.Term{
			SourceID:    atoi(t.ID),
			Slug:        model.DecodeSlug(strings.TrimSpace(t.Slug)),
			RawSlug:     t.Slug,
			Name:        t.Name,
			Description: t.Description,
		}}, nil
	}

	// Channel-level site fields. Anything else (wp:term for menus and
	// custom taxonomies, the RSS image block, generator...) is skipped.
	target := r.siteField(se.Name.Local)
	if target == nil {
		return nil, r.dec.Skip()
	}
	var s string
	if err := r.dec.DecodeElement(&s, &se); err != nil {
		return nil, fmt.Errorf("wxr: %s: %w", se.Name.Local, err)
	}
	if *target == "" {
		*target = strings.TrimSpace(s)
	}
	return nil, nil
}

func (r *Reader) siteField(local string) *string {
	switch local {
	case "title":
		return &r.site.Title
	case "description":
		return &r.site.Description
	case "language":
		return &r.site.Language
	case "link", "base_blog_url":
		return &r.site.URL
	}
	return nil
}

type rawAuthor struct {
	ID          string `xml:"author_id"`
	Login       string `xml:"author_login"`
	Email       string `xml:"author_email"`
	DisplayName string `xml:"author_display_name"`
}

type rawCategory struct {
	ID          string `xml:"term_id"`
	Slug        string `xml:"category_nicename"`
	Parent      string `xml:"category_parent"`
	Name        string `xml:"cat_name"`
	Description string `xml:"category_description"`
}

type rawTag struct {
	ID          string `xml:"term_id"`
	Slug        string `xml:"tag_slug"`
	Name        string `xml:"tag_name"`
	Description string `xml:"tag_description"`
}

type rawMeta struct {
	Key   string `xml:"meta_key"`
	Value string `xml:"meta_value"`
}

// rawItem collects the child elements of an <item> by local name.
type rawItem struct {
	fields     map[string]string
	categories []rawTermRef
	meta       []rawMeta
	comments   int
}

type rawTermRef struct {
	domain, nicename, name string
}

// item decodes one <item>. Children are read one by one because WXR uses
// the same local name in two namespaces (content:encoded, excerpt:encoded).
func (r *Reader) item(start xml.StartElement) (Entry, error) {
	ri := rawItem{fields: map[string]string{}}
	for {
		tok, err := r.dec.Token()
		if err != nil {
			return nil, fmt.Errorf("wxr: item: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return ri.entry(), nil
			}
		case xml.StartElement:
			if err := r.itemChild(&ri, t); err != nil {
				return nil, fmt.Errorf("wxr: item: %w", err)
			}
		}
	}
}

func (r *Reader) itemChild(ri *rawItem, se xml.StartElement) error {
	switch se.Name.Local {
	case "postmeta":
		var m rawMeta
		if err := r.dec.DecodeElement(&m, &se); err != nil {
			return err
		}
		ri.meta = append(ri.meta, m)
		return nil
	case "comment":
		ri.comments++
		return r.dec.Skip()
	case "category":
		var s string
		if err := r.dec.DecodeElement(&s, &se); err != nil {
			return err
		}
		ref := rawTermRef{name: s}
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "domain":
				ref.domain = a.Value
			case "nicename":
				ref.nicename = a.Value
			}
		}
		ri.categories = append(ri.categories, ref)
		return nil
	}
	var s string
	if err := r.dec.DecodeElement(&s, &se); err != nil {
		return err
	}
	key := se.Name.Local
	if key == "encoded" {
		// content:encoded or excerpt:encoded.
		key = "content"
		if strings.Contains(se.Name.Space, "excerpt") {
			key = "excerpt"
		}
	}
	ri.fields[key] = s
	return nil
}

func (ri *rawItem) entry() Entry {
	f := ri.fields
	meta := make(map[string]string, len(ri.meta))
	for _, m := range ri.meta {
		meta[m.Key] = m.Value
	}
	date := parseGMT(f["post_date_gmt"], f["post_date"])

	if f["post_type"] == "attachment" {
		return AttachmentEntry{model.Attachment{
			SourceID: atoi(f["post_id"]),
			Title:    f["title"],
			URL:      strings.TrimSpace(f["attachment_url"]),
			Alt:      meta["_wp_attachment_image_alt"],
			Caption:  f["excerpt"],
			ParentID: atoi(f["post_parent"]),
			Date:     date,
		}}
	}

	it := model.Item{
		SourceID:     atoi(f["post_id"]),
		Type:         model.ItemType(f["post_type"]),
		Status:       status(f["status"]),
		RawStatus:    f["status"],
		Title:        f["title"],
		Slug:         model.DecodeSlug(strings.TrimSpace(f["post_name"])),
		RawSlug:      f["post_name"],
		ParentID:     atoi(f["post_parent"]),
		Date:         date,
		Modified:     parseGMT(f["post_modified_gmt"], f["post_modified"]),
		Link:         strings.TrimSpace(f["link"]),
		Author:       f["creator"],
		Body:         f["content"],
		BodyFormat:   model.FormatWordPressHTML,
		Excerpt:      f["excerpt"],
		Meta:         meta,
		MenuOrder:    int(atoi(f["menu_order"])),
		Password:     f["post_password"],
		Sticky:       f["is_sticky"] == "1",
		CommentCount: ri.comments,
	}
	for _, c := range ri.categories {
		slug := model.DecodeSlug(strings.TrimSpace(c.nicename))
		if slug == "" {
			continue
		}
		switch c.domain {
		case "category":
			it.Categories = append(it.Categories, slug)
		case "post_tag":
			it.Tags = append(it.Tags, slug)
		default:
			continue // post formats, custom taxonomies
		}
		if it.TermNames == nil {
			it.TermNames = map[string]string{}
		}
		it.TermNames[slug] = c.name
	}
	return ItemEntry{it}
}

func status(s string) model.Status {
	switch s {
	case "publish":
		return model.StatusPublished
	case "future":
		return model.StatusScheduled
	case "draft":
		return model.StatusDraft
	case "pending":
		return model.StatusPending
	case "private":
		return model.StatusPrivate
	case "trash":
		return model.StatusTrash
	}
	return model.StatusOther
}

const wpTime = "2006-01-02 15:04:05"

// parseGMT parses post_date_gmt. Drafts have "0000-00-00 00:00:00" there;
// then the local post_date is used as if it were UTC, since WXR does not
// carry the site's time zone.
func parseGMT(gmt, local string) time.Time {
	for _, s := range []string{gmt, local} {
		t, err := time.Parse(wpTime, strings.TrimSpace(s))
		if err == nil && t.Year() > 1 {
			return t.UTC()
		}
	}
	return time.Time{}
}

func atoi(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
