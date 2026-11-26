package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/site"
)

var errNotExist = fs.ErrNotExist

func (s *Server) buildSite(w http.ResponseWriter, r *http.Request, sess *session) error {
	ctx := r.Context()
	err := s.DB.InTenantTx(ctx, func(tx pgx.Tx, _ *dbgen.Queries) error {
		return s.enqueueBuild(ctx, tx, sess.tenantID)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}

type menuItemJSON struct {
	Label     *string `json:"label"`
	ContentID *int64  `json:"contentId"`
	URL       *string `json:"url"`
}

// siteSettingsInput is what PUT /api/site accepts.
type siteSettingsInput struct {
	Name        string `json:"name"`
	LogoMediaID *int64 `json:"logoMediaId"`
	site.Settings
	Menu []menuItemJSON `json:"menu"`
}

type siteSettingsJSON struct {
	siteSettingsInput
	Logo *mediaJSON `json:"logo"`
	// Themes lists the installed themes and the settings each declares.
	Themes []themeJSON `json:"themes"`
}

type themeJSON struct {
	Name        string              `json:"name"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Settings    []build.SettingDecl `json:"settings"`
	// SettingsTitle and SettingsDescription head the theme's settings
	// (empty: the admin's default heading).
	SettingsTitle       string `json:"settingsTitle"`
	SettingsDescription string `json:"settingsDescription"`
}

const maxMenuItems = 20

var menuURL = regexp.MustCompile(`^(/|https?://)\S*$`)

func loadSite(ctx context.Context, q *dbgen.Queries) (siteSettingsJSON, error) {
	row, err := q.GetSite(ctx)
	if err != nil {
		return siteSettingsJSON{}, err
	}
	out := siteSettingsJSON{siteSettingsInput: siteSettingsInput{
		Name: row.Name, LogoMediaID: row.LogoMediaID, Settings: site.Parse(row.Settings), Menu: []menuItemJSON{},
	}, Themes: []themeJSON{}}
	for _, t := range build.Themes() {
		out.Themes = append(out.Themes, themeJSON{Name: t.Name, Title: t.Title, Description: t.Description, Settings: nonNil(t.Settings),
			SettingsTitle: t.SettingsTitle, SettingsDescription: t.SettingsDescription})
	}
	if row.LogoMediaID != nil {
		m, err := q.GetMedia(ctx, *row.LogoMediaID)
		if err != nil {
			return siteSettingsJSON{}, err
		}
		mj := toMedia(m)
		out.Logo = &mj
	}
	items, err := q.ListMenuItems(ctx)
	if err != nil {
		return siteSettingsJSON{}, err
	}
	for _, it := range items {
		out.Menu = append(out.Menu, menuItemJSON{Label: it.Label, ContentID: it.ContentID, URL: it.Url})
	}
	return out, nil
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request, _ *session) error {
	var out siteSettingsJSON
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		var err error
		out, err = loadSite(r.Context(), q)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) putSite(w http.ResponseWriter, r *http.Request, sess *session) error {
	if sess.role != "admin" {
		return &apiError{http.StatusForbidden, "forbidden", "サイトの設定を変えられるのは管理者だけです"}
	}
	var in siteSettingsInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 100 || !plainText(name, false) {
		return invalid("invalid_name", "サイトの名前を100文字以内で入力してください")
	}
	if err := in.Validate(); err != nil {
		return err
	}
	if len(in.Menu) > maxMenuItems {
		return invalid("invalid_menu", fmt.Sprintf("メニューは%d項目までです", maxMenuItems))
	}
	for i, it := range in.Menu {
		if (it.ContentID == nil) == (it.URL == nil) {
			return invalid("invalid_menu", fmt.Sprintf("メニューの%d番目は、ページかURLのどちらか一方を指定してください", i+1))
		}
		if it.Label != nil {
			l := strings.TrimSpace(*it.Label)
			if l == "" || len([]rune(l)) > 50 || !plainText(l, false) {
				return invalid("invalid_menu", fmt.Sprintf("メニューの%d番目の表示名を50文字以内で入力してください", i+1))
			}
			in.Menu[i].Label = &l
		}
		if it.URL != nil && (!menuURL.MatchString(*it.URL) || it.Label == nil) {
			return invalid("invalid_menu", fmt.Sprintf("メニューの%d番目のURLは / か https:// で始め、表示名も入れてください", i+1))
		}
	}

	ctx := r.Context()
	var out siteSettingsJSON
	err := s.DB.InTenantTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := checkThemeSettings(ctx, q, &in.Settings); err != nil {
			return err
		}
		for _, ref := range []struct {
			id         int64
			code, what string
		}{
			{in.FrontPageID, "invalid_front_page", "トップページ"},
			{in.PrivacyPageID, "invalid_privacy_page", "個人情報の取り扱いのページ"},
		} {
			if ref.id == 0 {
				continue
			}
			c, err := q.GetContent(ctx, ref.id)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Type != "page") {
				return invalid(ref.code, ref.what+"には固定ページを選んでください")
			} else if err != nil {
				return err
			}
		}
		if in.LogoMediaID != nil {
			if _, err := q.GetMedia(ctx, *in.LogoMediaID); errors.Is(err, pgx.ErrNoRows) {
				return invalid("invalid_logo", "ロゴの画像が見つかりません")
			} else if err != nil {
				return err
			}
		}
		settings, err := json.Marshal(in.Settings)
		if err != nil {
			return err
		}
		if err := q.UpdateSite(ctx, dbgen.UpdateSiteParams{Name: name, Settings: settings, LogoMediaID: in.LogoMediaID}); err != nil {
			return err
		}
		if err := q.ClearMenu(ctx); err != nil {
			return err
		}
		for i, it := range in.Menu {
			if it.ContentID != nil {
				if _, err := q.GetContent(ctx, *it.ContentID); errors.Is(err, pgx.ErrNoRows) {
					return invalid("invalid_menu", fmt.Sprintf("メニューの%d番目のページが見つかりません", i+1))
				} else if err != nil {
					return err
				}
			}
			if err := q.AddMenuItem(ctx, dbgen.AddMenuItemParams{Position: int64(i), Label: it.Label, ContentID: it.ContentID, Url: it.URL}); err != nil {
				return err
			}
		}
		if err := s.enqueueBuild(ctx, tx, sess.tenantID); err != nil {
			return err
		}
		out, err = loadSite(ctx, q)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

type buildJSON struct {
	ID         int64     `json:"id"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Files      int64     `json:"files"`
	Changed    int64     `json:"changed"`
	Removed    int64     `json:"removed"`
	Warnings   []string  `json:"warnings"`
	Error      *string   `json:"error"`
}

func (s *Server) listBuilds(w http.ResponseWriter, r *http.Request, _ *session) error {
	limit := int32(5)
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 || n > 50 {
			return invalid("invalid_limit", "件数は1から50で指定してください")
		}
		limit = int32(n)
	}
	out := []buildJSON{}
	err := s.DB.InTenant(r.Context(), func(q *dbgen.Queries) error {
		rows, err := q.ListBuilds(r.Context(), limit)
		for _, b := range rows {
			var warnings []string
			_ = json.Unmarshal(b.Warnings, &warnings)
			out = append(out, buildJSON{ID: b.ID, StartedAt: b.StartedAt, FinishedAt: b.FinishedAt, Files: b.Files,
				Changed: b.Changed, Removed: b.Removed, Warnings: nonNil(warnings), Error: b.Error})
		}
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// checkThemeSettings validates the values of each theme's declared
// settings and normalizes them in place. Page values must be pages of this
// site.
func checkThemeSettings(ctx context.Context, q *dbgen.Queries, s *site.Settings) error {
	for name, values := range s.ThemeSettings {
		info, err := build.ThemeByName(name)
		if err != nil {
			return &site.FieldError{Field: "themeSettings", Message: "入っていないテーマの設定です"}
		}
		clean, err := info.CheckSettings(values)
		var se *build.SettingError
		if errors.As(err, &se) {
			return &site.FieldError{Field: "themeSettings." + se.Key, Message: se.Message}
		}
		if err != nil {
			return err
		}
		for key, v := range clean {
			id, ok := v.(int64)
			if !ok {
				continue
			}
			c, err := q.GetContent(ctx, id)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Type != "page") {
				return &site.FieldError{Field: "themeSettings." + key, Message: "固定ページを選んでください"}
			} else if err != nil {
				return err
			}
		}
		s.ThemeSettings[name] = clean
	}
	return nil
}
