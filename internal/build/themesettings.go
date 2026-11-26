package build

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kom3da/pagewright/themes"
)

const defaultSettingMaxLength = 100

var (
	settingKey = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,39}$`)
	telValue   = regexp.MustCompile(`^[0-9+()\- ]{1,30}$`)
)

// checkSettings validates the declarations in theme.json.
func (m Manifest) checkSettings() error {
	seen := map[string]bool{}
	for i, d := range m.Settings {
		where := fmt.Sprintf("theme %s: theme.json settings[%d]", m.Name, i)
		if !settingKey.MatchString(d.Key) {
			return fmt.Errorf("%s: key %q must be letters and digits, starting with a letter", where, d.Key)
		}
		if seen[d.Key] {
			return fmt.Errorf("%s: key %q is declared twice", where, d.Key)
		}
		seen[d.Key] = true
		if d.Label == "" {
			return fmt.Errorf("%s (%s): label is missing", where, d.Key)
		}
		switch d.Type {
		case "text", "tel", "url", "page":
		default:
			return fmt.Errorf("%s (%s): type must be text, tel, url or page, not %q", where, d.Key, d.Type)
		}
		if d.MaxLength < 0 || d.MaxLength > 1000 {
			return fmt.Errorf("%s (%s): maxLength must be between 1 and 1000", where, d.Key)
		}
	}
	return nil
}

// ThemeInfo describes a built-in theme for the admin.
type ThemeInfo struct {
	Name, Title, Description string
	Settings                 []SettingDecl
}

// Themes lists the built-in themes. Themes that fail to load are left out.
func Themes() []ThemeInfo {
	var out []ThemeInfo
	for _, name := range themes.Names() {
		if t, err := themeNamed(name); err == nil {
			out = append(out, t.info())
		}
	}
	return out
}

// ThemeByName describes a built-in theme ("" is the default).
func ThemeByName(name string) (ThemeInfo, error) {
	t, err := themeNamed(name)
	if err != nil {
		return ThemeInfo{}, err
	}
	return t.info(), nil
}

func (t *theme) info() ThemeInfo {
	m := t.manifest
	return ThemeInfo{Name: m.Name, Title: m.Title, Description: m.Description, Settings: m.Settings}
}

// SettingError is a theme setting value that does not fit its declaration.
type SettingError struct {
	Key     string
	Message string // Japanese, for the admin
}

func (e *SettingError) Error() string { return e.Key + ": " + e.Message }

// CheckSettings validates and normalizes values for the theme's declared
// settings. Unknown keys are refused. Page values are returned as int64;
// whether the page exists is for the caller to check.
func (i ThemeInfo) CheckSettings(values map[string]any) (map[string]any, error) {
	decls := map[string]SettingDecl{}
	for _, d := range i.Settings {
		decls[d.Key] = d
	}
	out := map[string]any{}
	for key, v := range values {
		d, ok := decls[key]
		if !ok {
			return nil, &SettingError{key, "このテーマにはない設定です"}
		}
		if v == nil {
			continue
		}
		if d.Type == "page" {
			f, ok := v.(float64)
			if !ok {
				if n, isInt := v.(int64); isInt {
					f, ok = float64(n), true
				}
			}
			if !ok || f <= 0 || f != math.Trunc(f) {
				return nil, &SettingError{key, d.Label + "には固定ページを選んでください"}
			}
			out[key] = int64(f)
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, &SettingError{key, d.Label + "は文字で入力してください"}
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		max := d.MaxLength
		if max == 0 {
			max = defaultSettingMaxLength
		}
		if utf8.RuneCountInString(s) > max || !oneLine(s) {
			return nil, &SettingError{key, fmt.Sprintf("%sは改行なしの%d文字以内で入力してください", d.Label, max)}
		}
		switch d.Type {
		case "tel":
			if !telValue.MatchString(s) {
				return nil, &SettingError{key, d.Label + "は数字とハイフンで入力してください（例：03-1234-5678）"}
			}
		case "url":
			if !settingURL(s) {
				return nil, &SettingError{key, d.Label + "は https:// か / で始まるURLで入力してください"}
			}
		}
		out[key] = s
	}
	return out, nil
}

// settingURL accepts http(s) URLs and site paths.
func settingURL(s string) bool {
	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		return true
	}
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")
}

func oneLine(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// themeValues are the values of the theme's declared settings for
// templates: strings as they are, pages as *Link when published. Values
// that no longer fit their declaration are left out with a warning.
func (g *generator) themeValues() map[string]any {
	stored := g.settings.ThemeSettings[g.theme.manifest.Name]
	values, err := g.theme.info().CheckSettings(stored)
	if err != nil {
		g.warn("theme settings ignored: %v", err)
		return map[string]any{}
	}
	out := map[string]any{}
	for key, v := range values {
		if id, ok := v.(int64); ok {
			if it := g.itemByID[id]; it != nil && it.row.Type == "page" {
				out[key] = &Link{Title: g.title(it), URL: it.path}
			}
			continue
		}
		out[key] = v
	}
	return out
}
