package wxr_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/kom3da/pagewright/internal/migrate/model"
	"github.com/kom3da/pagewright/internal/migrate/wxr"
)

func readSample(t *testing.T) *model.Export {
	t.Helper()
	f, err := os.Open("testdata/sample.wxr")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	exp, err := wxr.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return exp
}

func TestReadAllSiteAuthorsTerms(t *testing.T) {
	exp := readSample(t)

	wantSite := model.Site{Title: "やまだ商店", Description: "町のパン屋さん", Language: "ja", URL: "https://example.jp"}
	if diff := cmp.Diff(wantSite, exp.Site); diff != "" {
		t.Errorf("site (-want +got):\n%s", diff)
	}
	wantAuthors := []model.Author{{SourceID: 1, Login: "yamada", Email: "yamada@example.jp", DisplayName: "山田 太郎"}}
	if diff := cmp.Diff(wantAuthors, exp.Authors); diff != "" {
		t.Errorf("authors (-want +got):\n%s", diff)
	}
	wantCats := []model.Term{
		{SourceID: 2, Slug: "news", RawSlug: "news", Name: "お知らせ"},
		{SourceID: 3, Slug: "event", RawSlug: "event", Name: "イベント", ParentSlug: "news"},
		{SourceID: 4, Slug: "お店", RawSlug: "%e3%81%8a%e5%ba%97", Name: "お店"},
	}
	if diff := cmp.Diff(wantCats, exp.Categories); diff != "" {
		t.Errorf("categories (-want +got):\n%s", diff)
	}
	wantTags := []model.Term{
		{SourceID: 5, Slug: "sale", RawSlug: "sale", Name: "セール"},
		{SourceID: 6, Slug: "新商品", RawSlug: "%e6%96%b0%e5%95%86%e5%93%81", Name: "新商品"},
	}
	if diff := cmp.Diff(wantTags, exp.Tags); diff != "" {
		t.Errorf("tags (-want +got):\n%s", diff)
	}
}

func TestReadAllItems(t *testing.T) {
	exp := readSample(t)

	type summary struct {
		ID       int64
		Type     model.ItemType
		Status   model.Status
		Slug     string
		ParentID int64
		Date     time.Time
	}
	var got []summary
	for _, it := range exp.Items {
		got = append(got, summary{it.SourceID, it.Type, it.Status, it.Slug, it.ParentID, it.Date})
	}
	d := func(s string) time.Time {
		tm, err := time.Parse(time.DateTime, s)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	want := []summary{
		{101, "post", model.StatusPublished, "開店のお知らせ", 0, d("2024-05-01 01:00:00")},
		{102, "post", model.StatusPublished, "summer-sale", 0, d("2024-07-01 00:00:00")},
		// No GMT date on drafts: the local date is used.
		{103, "post", model.StatusDraft, "", 0, d("2024-08-01 12:00:00")},
		{104, "post", model.StatusTrash, "old-post__trashed", 0, d("2024-03-01 03:00:00")},
		{105, "post", model.StatusOther, "", 0, d("2024-09-01 12:00:00")},
		{106, "post", model.StatusScheduled, "winter", 0, d("2026-12-01 00:00:00")},
		{202, "page", model.StatusPublished, "access", 201, d("2024-04-02 00:00:00")},
		{201, "page", model.StatusPublished, "company", 0, d("2024-04-01 00:00:00")},
		{203, "page", model.StatusPrivate, "internal", 0, d("2024-04-03 00:00:00")},
		{401, "nav_menu_item", model.StatusPublished, "401", 0, d("2024-04-01 00:00:00")},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("items (-want +got):\n%s", diff)
	}

	first := exp.Items[0]
	if !strings.HasPrefix(first.Body, "<!-- wp:paragraph -->") {
		t.Errorf("content:encoded = %q...", first.Body[:40])
	}
	wantFirst := model.Item{
		Title:        "開店のお知らせ",
		RawSlug:      "%e9%96%8b%e5%ba%97%e3%81%ae%e3%81%8a%e7%9f%a5%e3%82%89%e3%81%9b",
		Link:         "https://example.jp/2024/05/%e9%96%8b%e5%ba%97%e3%81%ae%e3%81%8a%e7%9f%a5%e3%82%89%e3%81%9b/",
		Author:       "yamada",
		Excerpt:      "開店しました。",
		RawStatus:    "publish",
		BodyFormat:   model.FormatWordPressHTML,
		Categories:   []string{"news", "event"},
		Tags:         []string{"sale"},
		TermNames:    map[string]string{"news": "お知らせ", "event": "イベント", "sale": "セール"},
		Meta:         map[string]string{"_edit_last": "1", "price": "1200", "_price": "field_65a1b2c3d4e5f"},
		CommentCount: 1,
	}
	ignore := cmpopts.IgnoreFields(model.Item{}, "SourceID", "Type", "Status", "Slug", "Date", "Modified", "Body")
	if diff := cmp.Diff(wantFirst, first, ignore); diff != "" {
		t.Errorf("first item (-want +got):\n%s", diff)
	}

	wantAtt := []model.Attachment{{
		SourceID: 301,
		Title:    "shop",
		URL:      "https://example.jp/wp-content/uploads/2024/05/shop.jpg",
		Alt:      "店の外観",
		Caption:  "店の外観",
		ParentID: 101,
		Date:     d("2024-05-01 00:30:00"),
	}}
	if diff := cmp.Diff(wantAtt, exp.Attachments); diff != "" {
		t.Errorf("attachments (-want +got):\n%s", diff)
	}
}

func TestReaderStreamsSiteFirst(t *testing.T) {
	f, err := os.Open("testdata/sample.wxr")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r := wxr.NewReader(f)
	var kinds []string
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch e.(type) {
		case wxr.SiteEntry:
			kinds = append(kinds, "site")
		case wxr.AuthorEntry:
			kinds = append(kinds, "author")
		case wxr.CategoryEntry:
			kinds = append(kinds, "category")
		case wxr.TagEntry:
			kinds = append(kinds, "tag")
		case wxr.ItemEntry:
			kinds = append(kinds, "item")
		case wxr.AttachmentEntry:
			kinds = append(kinds, "attachment")
		}
	}
	want := []string{"site", "author", "category", "category", "category", "tag", "tag",
		"item", "item", "item", "item", "item", "item", "item", "item", "item", "attachment", "item"}
	if diff := cmp.Diff(want, kinds); diff != "" {
		t.Errorf("entries (-want +got):\n%s", diff)
	}
}

func TestReadAllErrors(t *testing.T) {
	_, err := wxr.ReadAll(strings.NewReader(`<rss><channel><item><title>x</title>`))
	if err == nil {
		t.Fatal("truncated file: want error")
	}
}

func TestReadAllEmpty(t *testing.T) {
	exp, err := wxr.ReadAll(strings.NewReader(`<rss><channel><title>空</title></channel></rss>`))
	if err != nil {
		t.Fatal(err)
	}
	if exp.Site.Title != "空" || len(exp.Items) != 0 {
		t.Errorf("got %+v", exp)
	}
}
