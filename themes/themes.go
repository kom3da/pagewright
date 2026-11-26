// Package themes lists the themes built into Pagewright.
package themes

import (
	"io/fs"
	"slices"

	"github.com/kom3da/pagewright/themes/corporate"
)

// Default is the theme a site uses until it picks one.
const Default = "corporate"

var builtin = map[string]fs.FS{
	"corporate": corporate.FS,
}

// Lookup returns a built-in theme by name.
func Lookup(name string) (fs.FS, bool) {
	fsys, ok := builtin[name]
	return fsys, ok
}

// Names lists the built-in themes.
func Names() []string {
	names := make([]string, 0, len(builtin))
	for n := range builtin {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}
