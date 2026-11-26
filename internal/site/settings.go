// Package site holds per-site settings shared by the admin API and the
// build: colors, font, time zone, contact address and theme values.
package site

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"regexp"
	"strconv"
	"time"

	"github.com/kom3da/pagewright/themes"
)

// Settings is the JSON stored in tenants.settings. Empty fields use the
// theme's defaults.
type Settings struct {
	// PrimaryColor colors links and buttons (#RRGGBB). It must contrast
	// with white by at least 4.5:1, as text sits on white and white text
	// sits on it.
	PrimaryColor string `json:"primaryColor,omitempty"`
	// FooterColor is the footer background (#RRGGBB) under white text.
	FooterColor string `json:"footerColor,omitempty"`
	// Font is "gothic" (default) or "mincho".
	Font string `json:"font,omitempty"`
	// TimeZone is an IANA name for dates on the site. Default Asia/Tokyo.
	TimeZone string `json:"timeZone,omitempty"`
	// ContactEmail receives contact form messages.
	ContactEmail string `json:"contactEmail,omitempty"`
	// Theme is the name of a built-in theme; empty is the default.
	Theme string `json:"theme,omitempty"`
	// FrontPageID is the page shown at the top of the home page (its
	// title, lead and first image, then its body).
	FrontPageID int64 `json:"frontPageId,omitempty"`
	// PrivacyPageID is the page about handling personal data, which
	// contact forms link to.
	PrivacyPageID int64 `json:"privacyPageId,omitempty"`
	// ThemeSettings holds the values of the settings each theme declares in
	// its theme.json, by theme name, so switching themes keeps them. The
	// build package checks them against the declarations.
	ThemeSettings map[string]map[string]any `json:"themeSettings,omitempty"`
}

// DefaultTimeZone is used when TimeZone is empty.
const DefaultTimeZone = "Asia/Tokyo"

// MinContrast is the WCAG AA ratio for normal text (JIS X 8341-3).
const MinContrast = 4.5

// FieldError names the invalid setting, with a message for the admin.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Parse decodes stored settings. Unknown or broken JSON yields defaults.
func Parse(raw []byte) Settings {
	var s Settings
	_ = json.Unmarshal(raw, &s)
	return s
}

// Validate checks s and returns a *FieldError for the first problem.
func (s Settings) Validate() error {
	for _, c := range []struct{ field, value, name string }{
		{"primaryColor", s.PrimaryColor, "テーマカラー"},
		{"footerColor", s.FooterColor, "フッターの色"},
	} {
		if c.value == "" {
			continue
		}
		if !hexColor.MatchString(c.value) {
			return &FieldError{c.field, c.name + "は #1a73e8 のような6桁の色コードで指定してください"}
		}
		if r := Contrast(c.value, "#ffffff"); r < MinContrast {
			// Round down so 4.46 never reads as an acceptable 4.5.
			shown := math.Floor(r*10) / 10
			return &FieldError{c.field, fmt.Sprintf("%sが明るすぎて、白い文字や背景との差が足りません（コントラスト比 %.1f:1、4.5:1 以上が必要です）。もっと濃い色を選んでください", c.name, shown)}
		}
	}
	if s.Font != "" && s.Font != "gothic" && s.Font != "mincho" {
		return &FieldError{"font", "文字の書体はゴシック体か明朝体から選んでください"}
	}
	if s.TimeZone != "" {
		if _, err := time.LoadLocation(s.TimeZone); err != nil {
			return &FieldError{"timeZone", "タイムゾーンが正しくありません"}
		}
	}
	if s.Theme != "" {
		if _, ok := themes.Lookup(s.Theme); !ok {
			return &FieldError{"theme", "そのテーマは入っていません"}
		}
	}
	if s.ContactEmail != "" {
		if a, err := mail.ParseAddress(s.ContactEmail); err != nil || a.Address != s.ContactEmail {
			return &FieldError{"contactEmail", "お問い合わせの送り先メールアドレスの形式が正しくありません"}
		}
	}
	return nil
}

// Location returns the site's time zone.
func (s Settings) Location() *time.Location {
	name := s.TimeZone
	if name == "" {
		name = DefaultTimeZone
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.FixedZone("JST", 9*60*60)
}

// Contrast is the WCAG contrast ratio of two #RRGGBB colors.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(1) + 0.7152*ch(3) + 0.0722*ch(5)
}

// Darken returns hex with each channel scaled by f (0..1), for hover states.
func Darken(hex string, f float64) string {
	out := "#"
	for i := 1; i < 7; i += 2 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		out += fmt.Sprintf("%02x", int(math.Round(float64(v)*f)))
	}
	return out
}
