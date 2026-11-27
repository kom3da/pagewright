// Package corporate is the standard theme for small company sites.
//
// The files follow the theme contract in docs/themes/: theme.json,
// templates/ (page templates and parts/), and assets/.
package corporate

import "embed"

// FS holds theme.json, templates/ and assets/.
//
//go:embed theme.json templates/*.html templates/parts/*.html assets/*
var FS embed.FS
