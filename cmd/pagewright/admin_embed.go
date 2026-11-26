//go:build embedadmin

package main

import (
	"io/fs"

	"github.com/kom3da/pagewright/internal/config"
	webadmin "github.com/kom3da/pagewright/web/admin"
)

// adminFS returns the admin UI embedded in the binary.
func adminFS(config.Config) fs.FS {
	sub, err := fs.Sub(webadmin.FS, "dist")
	if err != nil {
		panic(err) // dist is always embedded
	}
	return sub
}
