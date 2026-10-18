import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [sveltekit()],
  // The CSP has no font-src, so default-src 'self' would block data: fonts: never inline assets.
  build: { assetsInlineLimit: 0 },
  server: { proxy: { "/v1": "http://127.0.0.1:8790", "/auth": "http://127.0.0.1:8790", "/health": "http://127.0.0.1:8790" } },
  // Unit tests only; the end-to-end suite in e2e/ runs under Playwright.
  test: { include: ["src/**/*.test.ts"], environment: "jsdom", environmentOptions: { jsdom: { url: "http://localhost/" } }, setupFiles: ["./vitest.setup.ts"] },
  resolve: process.env.VITEST ? { conditions: ["browser"] } : undefined,
});
