import { defineConfig, devices } from "@playwright/test";

// The end-to-end test: the built binary in fake mode, signing in through e2e/stub-hub.mjs.
// make e2e builds the binary and starts the throwaway Postgres serve.sh needs.
export default defineConfig({
  testDir: "./e2e",
  // The sweep runs four at a time; the other specs share recorded verdicts, so they run one by one.
  workers: 4,
  retries: 0,
  reporter: [["list"]],
  use: { baseURL: "http://127.0.0.1:8790", timezoneId: "UTC", trace: "retain-on-failure" },
  webServer: [
    { command: "node e2e/stub-hub.mjs", url: "http://127.0.0.1:8791/api/auth/jwks", reuseExistingServer: false },
    { command: "sh e2e/serve.sh", url: "http://127.0.0.1:8790/health", reuseExistingServer: false },
  ],
  // One sign-in (sweep-auth) is shared by the sweep, routes.spec and phone.spec: the server allows 20
  // sign-in requests in a burst. The sweep runs after the other specs, whose sign-ins it would
  // otherwise race at the stub hub. `npx playwright test --project=sweep` runs it with what it needs.
  projects: [
    { name: "seed", testMatch: /seed\.setup\.ts/ },
    { name: "chromium", dependencies: ["sweep-auth"], testMatch: /.*\.spec\.ts/, testIgnore: /sweep\.spec\.ts/, workers: 1, use: { ...devices["Desktop Chrome"] } },
    { name: "sweep-auth", dependencies: ["seed"], testMatch: /sweep\.setup\.ts/ },
    { name: "sweep", dependencies: ["chromium"], testMatch: /sweep\.spec\.ts/, workers: 4, fullyParallel: true, use: { ...devices["Desktop Chrome"] } },
  ],
});
