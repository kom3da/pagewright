package build

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// syncDir makes live identical to staging, touching only files whose
// content differs. Changed files are renamed from staging into live, so a
// reader never sees a half-written file. Returns the number of files and
// the changed and removed paths ("/a/index.html").
func syncDir(staging, live string) (files int, changed, removed []string, err error) {
	want, err := listFiles(staging)
	if err != nil {
		return 0, nil, nil, err
	}
	have, err := listFiles(live)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, nil, nil, err
	}

	// Remove first: a file may need to become a directory or vice versa.
	for _, rel := range slices.Sorted(keys(have)) {
		if _, ok := want[rel]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(live, filepath.FromSlash(rel))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, nil, nil, err
		}
		removed = append(removed, "/"+rel)
	}
	if err := removeEmptyDirs(live); err != nil {
		return 0, nil, nil, err
	}

	for _, rel := range slices.Sorted(keys(want)) {
		src := filepath.Join(staging, filepath.FromSlash(rel))
		dst := filepath.Join(live, filepath.FromSlash(rel))
		if _, ok := have[rel]; ok {
			same, err := sameContent(src, dst)
			if err != nil {
				return 0, nil, nil, err
			}
			if same {
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return 0, nil, nil, err
		}
		if err := os.Rename(src, dst); err != nil {
			return 0, nil, nil, err
		}
		changed = append(changed, "/"+rel)
	}
	return len(want), changed, removed, nil
}

func keys(m map[string]struct{}) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// listFiles returns the regular files under root as slash paths.
func listFiles(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	return out, err
}

// removeEmptyDirs removes empty directories below root, deepest first.
func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(dirs, func(a, b string) int {
		return strings.Count(b, string(filepath.Separator)) - strings.Count(a, string(filepath.Separator))
	})
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(d); err != nil {
				return err
			}
		}
	}
	return nil
}

func sameContent(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if !sb.Mode().IsRegular() || sa.Size() != sb.Size() {
		return false, nil
	}
	ha, err := hashFile(a)
	if err != nil {
		return false, err
	}
	hb, err := hashFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ha, hb), nil
}

func hashFile(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
