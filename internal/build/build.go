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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kom3da/pagewright/internal/db"
)

// DefaultBaseURL is the development preview server (ServeDir).
const DefaultBaseURL = "http://localhost:8081"

// Options configure a build.
type Options struct {
	// OutDir holds one directory per tenant; the site goes to
	// OutDir/<tenantID>/. Staging directories are created under OutDir so
	// finished files can be renamed into place.
	OutDir string
	// MediaRoot holds uploaded files at <MediaRoot>/<tenantID>/<storage_key>.
	MediaRoot string
	// BaseURL is the public origin used for absolute URLs (sitemap, feed,
	// canonical links). Default DefaultBaseURL.
	BaseURL string
	// Logger receives warnings. Default slog.Default().
	Logger *slog.Logger
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
// of the same tenant in this process run one at a time.
func Build(ctx context.Context, d *db.DB, tenantID uuid.UUID, opts Options) (Result, error) {
	start := time.Now()
	if tenantID == uuid.Nil {
		return Result{}, errors.New("build: no tenant")
	}
	if opts.OutDir == "" {
		return Result{}, errors.New("build: OutDir is required")
	}
	opts.BaseURL = strings.TrimSuffix(opts.BaseURL, "/")
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("tenant_id", tenantID)

	unlock := lockTenant(tenantID)
	defer unlock()

	th, err := defaultTheme()
	if err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}
	snap, err := load(db.WithTenant(ctx, tenantID), d)
	if err != nil {
		return Result{}, fmt.Errorf("build: %w", err)
	}

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

var tenantLocks sync.Map // uuid.UUID → *sync.Mutex

func lockTenant(id uuid.UUID) func() {
	m, _ := tenantLocks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
