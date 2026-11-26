package build

import (
	"html/template"
	"strings"

	"github.com/kom3da/pagewright/internal/site"
)

// mincho is the font stack for the mincho setting.
const mincho = `"Hiragino Mincho ProN", "BIZ UDPMincho", "Noto Serif JP", "Yu Mincho", serif`

// applySettings reads the site settings into the theme data.
func (g *generator) applySettings() {
	s := g.settings
	var vars []string
	// Colors were checked for contrast and format when saved; check the
	// format again since they end up in CSS.
	if err := s.Validate(); err != nil {
		g.warn("site settings ignored: %v", err)
		return
	}
	if c := s.PrimaryColor; c != "" {
		vars = append(vars, "--color-accent: "+c, "--color-accent-dark: "+site.Darken(c, 0.75), "--color-focus: "+c)
	}
	if c := s.FooterColor; c != "" {
		vars = append(vars, "--color-footer-bg: "+c)
	}
	if s.Font == "mincho" {
		vars = append(vars, "--font-sans: "+mincho)
	}
	if len(vars) > 0 {
		g.site.StyleVars = template.CSS(strings.Join(vars, "; ") + ";")
	}
	if id := g.snap.tenant.LogoMediaID; id != nil {
		for _, m := range g.snap.media {
			if m.ID == *id {
				g.site.LogoURL = "/media/" + m.StorageKey
				g.logoKey = m.StorageKey
			}
		}
	}
}

// configuredMenu is the menu set in the admin. Entries pointing at
// unpublished contents are left out.
func (g *generator) configuredMenu() []Link {
	var out []Link
	for _, m := range g.snap.menu {
		switch {
		case m.ContentID != nil:
			it, ok := g.itemByID[*m.ContentID]
			if !ok {
				continue
			}
			l := Link{Title: g.title(it), URL: it.path}
			if m.Label != nil {
				l.Title = *m.Label
			}
			out = append(out, l)
		case m.Url != nil && m.Label != nil:
			out = append(out, Link{Title: *m.Label, URL: *m.Url})
		default:
			g.warn("menu item %d has no target", m.ID)
		}
	}
	return out
}
