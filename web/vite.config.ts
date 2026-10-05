import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/",
  build: { outDir: "../internal/web/dist", emptyOutDir: true, assetsDir: "assets" },
  // Unit tests only; the end-to-end suite in e2e/ runs under Playwright.
  test: { include: ["src/**/*.test.{ts,tsx}"] },
});
