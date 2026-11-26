package render

// contactForm writes the fixed contact form. It works without JavaScript;
// the server answers with its own result page. The "website" field is a
// honeypot hidden from people.
func (r *renderer) contactForm() {
	if r.opts.ContactFormAction == "" {
		r.warn("contact form: no form address configured; skipped")
		return
	}
	r.w(`<form class="contact-form" id="contact" method="post" action="`, esc(r.opts.ContactFormAction), `">
<input type="hidden" name="page" value="`, esc(r.opts.PagePath), `">
<p class="contact-form__note"><span class="required">必須</span>の項目は必ず入力してください。</p>
<div class="form-field">
<label for="cf-name">お名前<span class="required">必須</span></label>
<input id="cf-name" name="name" type="text" required maxlength="100" autocomplete="name">
</div>
<div class="form-field">
<label for="cf-kana">ふりがな</label>
<input id="cf-kana" name="kana" type="text" maxlength="100">
</div>
<div class="form-field">
<label for="cf-email">メールアドレス<span class="required">必須</span></label>
<input id="cf-email" name="email" type="email" required maxlength="254" autocomplete="email" inputmode="email">
</div>
<div class="form-field">
<label for="cf-phone">電話番号</label>
<input id="cf-phone" name="phone" type="tel" maxlength="30" autocomplete="tel" inputmode="tel">
</div>
<div class="form-field">
<label for="cf-message">お問い合わせ内容<span class="required">必須</span></label>
<textarea id="cf-message" name="message" required maxlength="5000" rows="8"></textarea>
</div>
<div class="form-field form-field--check">
<input id="cf-consent" name="consent" type="checkbox" value="yes" required>
<label for="cf-consent">個人情報の取り扱いに同意します<span class="required">必須</span></label>
</div>
<div class="hp" aria-hidden="true">
<label for="cf-website">この欄には何も入力しないでください</label>
<input id="cf-website" name="website" type="text" tabindex="-1" autocomplete="off">
</div>
<p><button type="submit" class="button">送信する</button></p>
</form>
`)
	r.text.WriteString(" お問い合わせフォーム ")
}
