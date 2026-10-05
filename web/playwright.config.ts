import { defineConfig, devices } from "@playwright/test";

// The end-to-end test: the built binary in fake mode, signing in through e2e/stub-hub.mjs.
// make e2e builds the binary and starts the throwaway Postgres serve.sh needs.
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: { baseURL: "http://127.0.0.1:8790", timezoneId: "UTC", trace: "retain-on-failure" },
  webServer: [
    { command: "node e2e/stub-hub.mjs", url: "http://127.0.0.1:8791/api/auth/jwks", reuseExistingServer: false },
    { command: "sh e2e/serve.sh", url: "http://127.0.0.1:8790/health", reuseExistingServer: false },
  ],
  projects: [
    { name: "seed", testMatch: /seed\.setup\.ts/ },
    { name: "chromium", dependencies: ["seed"], testMatch: /.*\.spec\.ts/, use: { ...devices["Desktop Chrome"] } },
  ],
});
