package render

// contactForm places the contact form. Its markup comes from the theme
// through Options.ContactForm.
func (r *renderer) contactForm() {
	r.hasForm = true
	if r.opts.ContactForm == nil {
		r.warn("contact form: forms are not configured; skipped")
		return
	}
	html, err := r.opts.ContactForm()
	if err != nil {
		r.warn("contact form: %v; skipped", err)
		return
	}
	r.w(string(html))
	r.text.WriteString(" お問い合わせフォーム ")
}
