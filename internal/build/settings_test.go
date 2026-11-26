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
		logo_media_id = (SELECT id FROM media WHERE storage_key = 'logo.png') WHERE id = $1`, s.tenant())
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
