package model

import (
	"net/url"

	"golang.org/x/text/unicode/norm"
)

// DecodeSlug percent-decodes a slug or path segment (WordPress stores
// Japanese slugs as %E3%81%8A...) and NFC-normalizes it. Invalid escapes
// are left as they are.
func DecodeSlug(raw string) string {
	s, err := url.PathUnescape(raw)
	if err != nil {
		s = raw
	}
	return norm.NFC.String(s)
}
