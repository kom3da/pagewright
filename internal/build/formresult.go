package build

import (
	"context"
	"html/template"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db"
)

// FormResult is what the theme's "form" template shows after a contact
// form was sent (Page.Form).
type FormResult struct {
	Sent     bool         // the message was received
	Messages []string     // what happened, one paragraph each
	Errors   []FieldError // input to fix, in form order
	// BackURL is the page the form was on, on the public site.
	BackURL string
	// Form is the contact form again, filled in with what was sent and
	// marked with Errors, when there are errors and the theme has the
	// "contact-form" part.
	Form template.HTML
}

// FieldError is a problem with one field of the contact form. Field is
// the field's name: name, kana, email, phone, message or consent.
type FieldError struct {
	Field, Message string
}

// ContactValues are what a visitor typed into the contact form.
type ContactValues struct {
	Name, Kana, Email, Phone, Message string
	Consent                           bool
}

// FormResultOptions configures RenderFormResult.
type FormResultOptions struct {
	// PublicBaseURL and FormBaseURL are as in PreviewOptions.
	PublicBaseURL, FormBaseURL string
	// SelfURL is the URL the result is served at (the form's action), for
	// in-page links under the <base>.
	SelfURL string
	// Page is the public path of the page the form was on.
	Page   string
	Title  string
	Result FormResult
	// Values fill the form again when Result has errors.
	Values ContactValues
}

// RenderFormResult renders the result of a contact form post in the frame
// of the site's theme: site name, logo, menu and colors, with a <base>
// pointing at the public site so its stylesheet, images and links work.
// It returns the page and that base URL.
func RenderFormResult(ctx context.Context, d *db.DB, tenantID uuid.UUID, opts FormResultOptions) ([]byte, string, error) {
	ctx = db.WithTenant(ctx, tenantID)
	snap, err := load(ctx, d, true)
	if err != nil {
		return nil, "", err
	}
	th, err := pickTheme(nil, snap.tenant.Settings)
	if err != nil {
		return nil, "", err
	}
	base := baseURL(snap.tenant.PublicBaseUrl, opts.PublicBaseURL)
	g := &generator{
		snap: snap, opts: Options{BaseURL: base, FormBaseURL: opts.FormBaseURL}, tenantID: tenantID, theme: th,
		warn: func(string, ...any) {},
	}
	g.prepare()
	g.index()
	g.buildSite()

	res := opts.Result
	if len(res.Errors) > 0 {
		if action := g.contactAction(); action != "" {
			errs := map[string]string{}
			for _, e := range res.Errors {
				errs[e.Field] = e.Message
			}
			form, ok, err := th.part("contact-form", ContactForm{
				Action: action, Page: opts.Page, PrivacyURL: g.privacyPath(), Values: opts.Values, Errors: errs,
			})
			if err != nil {
				return nil, "", err
			}
			if ok {
				res.Form = form
			}
		}
	}

	// No path of its own: the menu marks no link as the current page.
	pg := g.page("form", "")
	pg.Title = opts.Title
	pg.NoIndex = true
	pg.SelfURL = opts.SelfURL
	pg.Form = &res
	crumbs := []Link{{Title: th.manifest.Labels.Home, URL: "/"}}
	if it := g.itemByPath[opts.Page]; it != nil && opts.Page != "/" {
		crumbs = append(crumbs, Link{Title: g.title(it), URL: g.urlOf(it)})
	}
	pg.Breadcrumbs = append(crumbs, Link{Title: opts.Title})
	b, err := th.render("form", pg)
	if err != nil {
		return nil, "", err
	}
	return inject(b, `<base href="`+template.HTMLEscapeString(base+"/")+`">`, "", ""), base, nil
}
