package build_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

func TestSiteSettingsInBuild(t *testing.T) {
	env := dbtest.New(t)
	s := newSeeder(t, env, "設定株式会社", "owner@example.com")
	// 2026-01-01 20:00 UTC is 2026-01-02 in Tokyo but still 2026-01-01 in New York.
	at := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	about := s.content("page", ptr("about"), nil)
	s.publish(about.ID, s.revision(about.ID, "会社概要", paragraph("x")).ID, at)
	post := s.content("post", ptr("hello"), nil)
	s.publish(post.ID, s.revision(post.ID, "記事", paragraph("x")).ID, at)

	s.admin(`INSERT INTO media (tenant_id, storage_key, filename, content_type, size_bytes) VALUES ($1, 'logo.png', 'logo.png', 'image/png', 1)`, s.tenant())
	s.admin(`UPDATE tenants SET settings = '{"primaryColor":"#8a1538","font":"mincho","timeZone":"America/New_York"}',
		logo_media_id = (SELECT id FROM media WHERE tenant_id = $1 AND storage_key = 'logo.png') WHERE id = $1`, s.tenant())
	s.admin(`INSERT INTO menu_items (tenant_id, position, label, url) VALUES ($1, 0, '採用情報', 'https://example.com/jobs')`, s.tenant())
	s.admin(`INSERT INTO menu_items (tenant_id, position, label, content_id) VALUES ($1, 1, '私たちについて', $2)`, s.tenant(), about.ID)

	opts := build.Options{OutDir: t.TempDir()}
	if _, err := build.Build(context.Background(), env.DB, s.tenant(), opts); err != nil {
		t.Fatal(err)
	}
	home := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))["/index.html"]
	for _, want := range []string{
		"--color-accent: #8a1538", "Mincho", `class="site-logo" src="/media/logo.png" alt="設定株式会社"`,
		"2026年1月1日",
	} {
		if !strings.Contains(home, want) {
			t.Errorf("home page lacks %q", want)
		}
	}
	jobs := strings.Index(home, "採用情報")
	us := strings.Index(home, "私たちについて")
	if jobs < 0 || us < 0 || jobs > us {
		t.Errorf("menu order: 採用情報 at %d, 私たちについて at %d; want both, in that order", jobs, us)
	}
}

func TestCorporateHome(t *testing.T) {
	env := dbtest.New(t)
	s := newSeeder(t, env, "デモ株式会社", "owner@example.com")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	front := s.content("page", ptr("top"), nil)
	s.publish(front.ID, s.revision(front.ID, "地域の暮らしを支える", paragraph("住まいの設備工事と保守を通じて、毎日を支えています。")).ID, at)
	service := s.content("page", ptr("service"), nil)
	s.publish(service.ID, s.revision(service.ID, "事業紹介", paragraph("給湯器・水まわりの交換に対応します。")).ID, at)
	contact := s.content("page", ptr("contact"), nil)
	s.publish(contact.ID, s.revision(contact.ID, "お問い合わせ", `{"type":"doc","content":[{"type":"contactForm"}]}`).ID, at)
	s.admin(`UPDATE tenants SET settings = jsonb_build_object('frontPageId', $2::bigint, 'themeSettings',
		jsonb_build_object('corporate', jsonb_build_object('phone', '03-1234-5678', 'address', '東京都千代田区1-1', 'businessHours', '平日 9:00〜18:00')))
		WHERE id = $1`, s.tenant(), front.ID)

	opts := build.Options{OutDir: t.TempDir(), FormBaseURL: "https://admin.example"}
	if _, err := build.Build(context.Background(), env.DB, s.tenant(), opts); err != nil {
		t.Fatal(err)
	}
	files := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))
	home := files["/index.html"]
	for _, want := range []string{
		`<h1 class="hero__title">地域の暮らしを支える</h1>`,
		"住まいの設備工事と保守を通じて",
		`class="card" href="/service/"`,
		`href="/contact/" class="nav-button"`,
		"東京都千代田区1-1", `href="tel:03-1234-5678"`, "お気軽にご相談ください",
		`<span class="site-mark" aria-hidden="true">デ</span>`,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("home page lacks %q", want)
		}
	}
	_, homeBody, _ := strings.Cut(home, "<body")
	if n := strings.Count(homeBody, "住まいの設備工事と保守を通じて"); n != 1 {
		t.Errorf("the lead appears %d times in the home page body, want once", n)
	}
	for _, unwanted := range []string{`class="card" href="/top/"`, `class="card" href="/contact/"`} {
		if strings.Contains(home, unwanted) {
			t.Errorf("home page has %q", unwanted)
		}
	}
	// The front page is read on the home page: its own URL only redirects there.
	if top := files["/top/index.html"]; !strings.Contains(top, `<meta http-equiv="refresh" content="0; url=/">`) {
		t.Errorf("/top/ is not a redirect to /:\n%s", top)
	}
	if !strings.Contains(files["/_redirects.nginx"], `location = "/top/" { return 301 "/"; }`) {
		t.Errorf("_redirects.nginx lacks /top/:\n%s", files["/_redirects.nginx"])
	}
	if strings.Contains(files["/sitemap.xml"], "/top/") {
		t.Errorf("sitemap lists /top/:\n%s", files["/sitemap.xml"])
	}
	if !strings.Contains(files["/sitemap.xml"], "/service/") {
		t.Errorf("sitemap lacks /service/:\n%s", files["/sitemap.xml"])
	}
	if !strings.Contains(files["/contact/index.html"], `<form class="contact-form"`) {
		t.Error("the contact page has no form")
	}
	if strings.Contains(files["/contact/index.html"], "お気軽にご相談ください") {
		t.Error("the contact page repeats the contact band")
	}
}
