package content

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/kom3da/pagewright/internal/db/dbgen"
)

func ptr[T any](v T) *T { return &v }

func TestSettingsRoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	for _, s := range []Settings{
		{},
		{SlugSet: true},
		{SlugSet: true, Slug: ptr("news"), ParentSet: true},
		{ParentSet: true, ParentID: ptr(int64(3)), CategoryIDs: &[]int64{}, TagIDs: &[]int64{1, 2}, PublishedAt: &at},
	} {
		b, err := s.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		got, err := Pending(dbgen.Content{PendingSettings: b})
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(s, got); diff != "" {
			t.Errorf("round trip of %s (-want +got):\n%s", b, diff)
		}
	}
}

func TestOverAndChangesFrom(t *testing.T) {
	at := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	live := Settings{SlugSet: true, Slug: ptr("old"), ParentSet: true, CategoryIDs: &[]int64{1}, TagIDs: &[]int64{}, PublishedAt: &at}
	pending := Settings{SlugSet: true, Slug: ptr("new"), TagIDs: &[]int64{2}}

	// Setting the slug back and the category again leaves only the tags.
	input := Settings{SlugSet: true, Slug: ptr("old"), CategoryIDs: &[]int64{1}}
	got := input.Over(pending).ChangesFrom(live)
	if diff := cmp.Diff(Settings{TagIDs: &[]int64{2}}, got); diff != "" {
		t.Errorf("merged changes (-want +got):\n%s", diff)
	}
	if !(Settings{SlugSet: true, Slug: ptr("old")}).ChangesFrom(live).Empty() {
		t.Error("live values left changes")
	}
	shown := pending.Over(live)
	if *shown.Slug != "new" || !cmp.Equal(*shown.TagIDs, []int64{2}) || !cmp.Equal(*shown.CategoryIDs, []int64{1}) {
		t.Errorf("shown settings = %+v", shown)
	}
}
