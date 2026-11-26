import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig(({ mode }) => ({
  base: "/admin/",
  plugins: [react()],
  // The MSW service worker is only served in `pnpm dev:mock`, never shipped in dist/.
  publicDir: mode === "mock" ? "mock-public" : false,
  // An admin app behind login; one ~270 kB (gzip) main chunk (React, Mantine, router) is acceptable.
  build: { chunkSizeWarningLimit: 900 },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
}));
