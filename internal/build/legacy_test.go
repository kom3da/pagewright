package build_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/kom3da/pagewright/internal/build"
	"github.com/kom3da/pagewright/internal/db/dbtest"
)

func TestLegacyQueryRedirects(t *testing.T) {
	env := dbtest.New(t)
	s := newSeeder(t, env, "移行株式会社", "owner@example.com")
	at := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)

	// A post imported from WordPress keeps the WordPress ID as its number.
	s.admin("INSERT INTO contents (tenant_id, number, type, slug, source_format, source_body) VALUES ($1, 101, 'post', 'hello', 'wordpress_html', '<p>x</p>')", s.tenant())
	var imported int64
	if err := env.Admin.QueryRow(context.Background(), "SELECT id FROM contents WHERE tenant_id = $1 AND number = 101", s.tenant()).Scan(&imported); err != nil {
		t.Fatal(err)
	}
	r := s.revision(imported, "移行した記事", paragraph("本文"))
	s.publish(imported, r.ID, at)
	s.route("/2024/05/hello/", "canonical", &imported, nil)

	// A post written in Pagewright has no WordPress ID.
	native := s.content("post", ptr("native"), nil)
	nr := s.revision(native.ID, "新しい記事", paragraph("本文"))
	s.publish(native.ID, nr.ID, at)

	opts := build.Options{OutDir: t.TempDir()}
	if _, err := build.Build(context.Background(), env.DB, s.tenant(), opts); err != nil {
		t.Fatal(err)
	}
	files := siteFiles(t, filepath.Join(opts.OutDir, s.tenant().String()))

	var legacy map[string]map[string]string
	if err := json.Unmarshal([]byte(files["/_legacy.json"]), &legacy); err != nil {
		t.Fatalf("_legacy.json: %v", err)
	}
	want := map[string]map[string]string{"p": {"101": "/2024/05/hello/"}}
	if diff := cmp.Diff(want, legacy); diff != "" {
		t.Errorf("_legacy.json (-want +got):\n%s", diff)
	}
	if !strings.Contains(files["/_redirects.nginx"], `if ($arg_p = 101) { return 301 "/2024/05/hello/"; }`) {
		t.Errorf("_redirects.nginx lacks the ?p=101 rule:\n%s", files["/_redirects.nginx"])
	}
	if !strings.Contains(files["/index.html"], "/_legacy.json") {
		t.Error("home page lacks the forwarding script")
	}
}
