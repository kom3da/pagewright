//go:build embedadmin

// Package webadmin embeds the built admin UI. Build with -tags embedadmin
// after `mise run admin:build` (mise run build does both).
package webadmin

import "embed"

// FS holds dist/.
//
//go:embed all:dist
var FS embed.FS
