// Package build writes a tenant's public site as static files.
//
// Every build renders the whole site into a staging directory, then
// compares it file by file (SHA-256) with the previous output and moves
// over only what changed. Pages do not track dependencies on each other:
// a "latest news" box touches nearly every page anyway, and the slow part
// of publishing is deploying and purging, which this keeps minimal.
//
// Only published revisions are rendered; drafts never reach the output.
package build

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/site"
)

// DefaultBaseURL is the development preview server (ServeDir).
const DefaultBaseURL = "http://localhost:8081"

// baseURL is the site's own public URL, else fallback, else DefaultBaseURL.
func baseURL(own *string, fallback string) string {
	if u := site.PublicURL(own, fallback); u != "" {
		return u
	}
	return DefaultBaseURL
}

// Options configure a build.
type Options struct {
	// OutDir holds one directory per tenant; the site goes to
	// OutDir/<tenantID>/. Staging directories are created under OutDir so
	// finished files can be renamed into place.
	OutDir string
	// MediaRoot holds uploaded files at <MediaRoot>/<tenantID>/<storage_key>.
	MediaRoot string
	// BaseURL is the public origin used for absolute URLs (sitemap, feed,
	// canonical links) of a site that has no URL of its own
	// (tenants.public_base_url). Default DefaultBaseURL.
	BaseURL string
	// Logger receives warnings. Default slog.Default().
	Logger *slog.Logger
	// ThemeFS, when set, is used instead of the site's chosen theme (for
	// tests and theme development).
	ThemeFS fs.FS
	// FormBaseURL is the origin of the Pagewright server that receives
	// contact form posts (/f/<tenant>/contact). Empty leaves forms out.
	FormBaseURL string
	// Pagefind is the path of the Pagefind binary (the extended build, for
	// Japanese). Empty disables site search.
	Pagefind string
}

// Result describes what a build changed. Paths are public file paths
// (decoded, NFC) with a leading slash, e.g. /posts/hello/index.html, sorted.
type Result struct {
	Files    int      // files in the site
	Changed  []string // added or modified
	Removed  []string
	Warnings []string // content left out or problems that did not stop the build
	Duration time.Duration
}

// Build renders the tenant's site and updates OutDir/<tenantID>/. Builds
// of the same tenant run one at a time, also across processes sharing the
// database (see lockTenant).
func Build(ctx context.Context, d *db.DB, tenantID uuid.UUID, opts Options) (Result, error) {
	start := time.Now()
	if tenantID == uuid.Nil {
		return Result{}, errors.New("build: no tenant")
	}
	if opts.OutDir == "" {
		return Result{}, errors.New("build: OutDir is required")
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("tenant_id", tenantID)

	release, err := lockTenant(ctx, d, tenantID)
	if err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}
	unlock := sync.OnceFunc(release)
	defer unlock()

	snap, err := load(db.WithTenant(ctx, tenantID), d, false)
	if err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}
	th, err := pickTheme(opts.ThemeFS, snap.tenant.Settings)
	if err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}
	opts.BaseURL = baseURL(snap.tenant.PublicBaseUrl, opts.BaseURL)

	stagingRoot := filepath.Join(opts.OutDir, ".staging")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}
	// Leftovers of a crashed build of this tenant (we hold its lock).
	if old, _ := filepath.Glob(filepath.Join(stagingRoot, tenantID.String()+"-*")); len(old) > 0 {
		for _, dir := range old {
			_ = os.RemoveAll(dir)
		}
	}
	staging := filepath.Join(stagingRoot, tenantID.String()+"-"+randomSuffix())
	defer func() {
		if err := os.RemoveAll(staging); err != nil {
			log.Warn("remove staging directory", "dir", staging, "err", err)
		}
	}()

	var res Result
	g := &generator{
		snap: snap, opts: opts, tenantID: tenantID, theme: th, out: newOutput(staging),
		warn: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			res.Warnings = append(res.Warnings, msg)
			log.Warn("build: " + msg)
		},
	}
	if err := g.run(); err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}
	if opts.Pagefind != "" {
		if err := runPagefind(ctx, opts.Pagefind, staging); err != nil {
			// Search is optional: publish the site without it rather than
			// not at all (e.g. a CPU the Pagefind binary cannot run on).
			log.Error("site search failed; building without search", "err", err)
			unlock()
			opts.Pagefind = ""
			res, err := Build(ctx, d, tenantID, opts)
			res.Warnings = append(res.Warnings, "サイト内検索の索引を作れなかったため、検索なしで書き出しました")
			return res, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	live := filepath.Join(opts.OutDir, tenantID.String())
	res.Files, res.Changed, res.Removed, err = syncDir(staging, live)
	if err != nil {
		return Result{}, fmt.Errorf("build: deploy: %w", err)
	}
	res.Duration = time.Since(start)
	log.Info("site built", "files", res.Files, "changed", len(res.Changed),
		"removed", len(res.Removed), "warnings", len(res.Warnings), "duration", res.Duration)
	return res, nil
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
