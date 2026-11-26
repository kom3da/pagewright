import { defineConfig, devices } from "@playwright/test";

// PW_PORT lets several checkouts (worktrees) run E2E side by side.
const port = Number(process.env.PW_PORT) || 5175;

// E2E against a mock-mode build (MSW mock API, no Go server needed). A build is used
// instead of the dev server because the dev server's first-load dependency optimization
// reloads the page mid-test. `pnpm e2e` builds dist-mock first; the web server below only
// serves it.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: `http://localhost:${port}/admin/`,
    locale: "ja-JP",
    // Mantine skips transitions then, so axe never sees a half-faded dialog.
    reducedMotion: "reduce",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    // One process (exec, node directly, no pnpm/mise shims): Playwright stops it with a
    // signal and would otherwise wait forever on pipes held by a leftover child.
    command: `exec node node_modules/vite/bin/vite.js preview --mode mock --outDir dist-mock --port ${port} --strictPort`,
    url: `http://localhost:${port}/admin/`,
    gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
    stdout: "ignore",
    // A leftover preview server would serve a stale dist-mock.
    reuseExistingServer: false,
  },
});
