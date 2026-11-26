package build

import (
	"context"
	"fmt"
	"os/exec"
)

// searchDir is where Pagefind writes its index and UI, under the site.
const searchDir = "pagefind"

// searchPath is the public search page.
const searchPath = "/search/"

// runPagefind indexes the staged site. Only elements marked
// data-pagefind-body (content pages) are indexed.
func runPagefind(ctx context.Context, bin, site string) error {
	cmd := exec.CommandContext(ctx, bin, "--site", site, "--output-subdir", searchDir, "--force-language", "ja", "--silent")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pagefind: %w: %s", err, out)
	}
	return nil
}
