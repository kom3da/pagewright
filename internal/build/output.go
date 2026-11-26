package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// output writes the files of one build into a staging directory. Each
// file can be claimed once; a second claim is a URL conflict, reported and
// skipped so one bad route does not stop the whole site.
type output struct {
	dir     string
	claimed map[string]string // relative file path → what claimed it
}

const systemOwner = "system"

func newOutput(dir string) *output {
	return &output{dir: dir, claimed: map[string]string{}}
}

// reserve claims a file for something written later by the build itself.
func (o *output) reserve(rel string) { o.claimed[rel] = systemOwner }

// claim records that who will write rel. It fails when another owner has
// it already.
func (o *output) claim(rel, who string) error {
	if !validRel(rel) {
		return fmt.Errorf("unsafe output path %q for %s", rel, who)
	}
	if prev, ok := o.claimed[rel]; ok && prev != who {
		return fmt.Errorf("%s: /%s is already used by %s", who, rel, prev)
	}
	o.claimed[rel] = who
	return nil
}

func (o *output) target(rel string) (string, error) {
	p := filepath.Join(o.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, nil
}

func (o *output) write(rel, who string, data []byte) error {
	if err := o.claim(rel, who); err != nil {
		return err
	}
	p, err := o.target(rel)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// link places an existing file (a media original) into the output. A hard
// link avoids copying large files; it falls back to a copy across devices.
func (o *output) link(rel, who, src string) error {
	if err := o.claim(rel, who); err != nil {
		return err
	}
	p, err := o.target(rel)
	if err != nil {
		return err
	}
	if err := os.Link(src, p); err == nil {
		return nil
	}
	return copyFile(src, p)
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
