import { defineConfig, devices } from "@playwright/test";

const port = 5175;

// E2E against a mock-mode build (MSW mock API, no Go server needed). A build is used
// instead of the dev server because the dev server's first-load dependency optimization
// reloads the page mid-test.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: `http://localhost:${port}/admin/`,
    locale: "ja-JP",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: `vite build --mode mock --outDir dist-mock && vite preview --mode mock --outDir dist-mock --port ${port} --strictPort`,
    url: `http://localhost:${port}/admin/`,
    reuseExistingServer: !process.env.CI,
  },
});
