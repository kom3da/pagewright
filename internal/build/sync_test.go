package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSyncDirKeepsURLsAnswering checks after every change to the live
// directory that each file present both before and after the sync can be
// read, with its old or its new content.
func TestSyncDirKeepsURLsAnswering(t *testing.T) {
	dir := t.TempDir()
	staging, live := filepath.Join(dir, "staging"), filepath.Join(dir, "live")
	before := map[string]string{
		"index.html":   "home",
		"a/index.html": "a",
		"b/index.html": "b old",
		"c/index.html": "c",
		"d":            "d was a file",
		"e/index.html": "e was a directory",
		"style.css":    "css old",
	}
	after := map[string]string{
		"index.html":     "home",
		"a/index.html":   "a",
		"b/index.html":   "b new",
		"new/index.html": "new",
		"d/index.html":   "d is a directory",
		"e":              "e is a file",
		"style.css":      "css new",
	}
	writeTree(t, live, before)
	writeTree(t, staging, after)

	// Old files in the way of new ones (d, e/index.html) must go early.
	inTheWay := map[string]bool{"d": true, "e/index.html": true}
	read := func(rel string) (string, bool) {
		b, err := os.ReadFile(filepath.Join(live, filepath.FromSlash(rel)))
		return string(b), err == nil
	}
	steps := 0
	syncStep = func() {
		steps++
		allPlaced := true
		for rel, now := range after {
			if got, ok := read(rel); !ok || got != now {
				allPlaced = false
			}
		}
		for rel, old := range before {
			got, ok := read(rel)
			now, stays := after[rel]
			switch {
			case stays && !ok:
				t.Errorf("step %d: /%s unreadable", steps, rel)
			case stays && got != old && got != now:
				t.Errorf("step %d: /%s = %q", steps, rel, got)
			case !stays && !ok && !allPlaced && !inTheWay[rel]:
				// Old pages may still link to it.
				t.Errorf("step %d: stale /%s removed before every new file was placed", steps, rel)
			}
		}
	}
	t.Cleanup(func() { syncStep = func() {} })

	files, changed, removed, err := syncDir(staging, live)
	if err != nil {
		t.Fatal(err)
	}
	if steps == 0 {
		t.Fatal("syncStep never called")
	}
	if files != len(after) {
		t.Errorf("files = %d, want %d", files, len(after))
	}
	if diff := cmp.Diff([]string{"/b/index.html", "/d/index.html", "/e", "/new/index.html", "/style.css"}, changed); diff != "" {
		t.Errorf("changed (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"/c/index.html", "/d", "/e/index.html"}, removed); diff != "" {
		t.Errorf("removed (-want +got):\n%s", diff)
	}
	have, err := listFiles(live)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for rel := range have {
		b, err := os.ReadFile(filepath.Join(live, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		got[rel] = string(b)
	}
	if diff := cmp.Diff(after, got); diff != "" {
		t.Errorf("live (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(filepath.Join(live, "c")); !os.IsNotExist(err) {
		t.Errorf("empty directory c left behind: %v", err)
	}
}
