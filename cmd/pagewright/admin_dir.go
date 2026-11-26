//go:build !embedadmin

package main

import (
	"io/fs"

	"github.com/kom3da/pagewright/internal/adminui"
	"github.com/kom3da/pagewright/internal/config"
)

// adminFS serves the admin UI from disk during development.
func adminFS(cfg config.Config) fs.FS {
	return adminui.Dir(cfg.AdminDir)
}
