import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig(({ mode }) => ({
  base: "/admin/",
  plugins: [react(), tailwindcss()],
  // The MSW service worker is only served in `pnpm dev:mock`, never shipped in dist/.
  publicDir: mode === "mock" ? "mock-public" : false,
  // An admin app behind login; one ~170 kB (gzip) main chunk is acceptable.
  build: { chunkSizeWarningLimit: 600 },
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
