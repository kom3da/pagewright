// Package corporate is the standard theme for small company sites.
//
// templates/ holds Go html/template files. base.html defines the page
// frame ("base") and each page kind (home, single, list, 404) defines
// "main". assets/ is copied to /_assets/ on the public site.
package corporate

import "embed"

// FS holds templates/ and assets/.
//
//go:embed templates/*.html assets/*
var FS embed.FS
