import { defineConfig, devices } from "@playwright/test";

// Contract-level browser E2E using a synthetic Go-API fixture; this does not
// call paid LLM providers or establish real-market evidence correctness.
const baseURL = "http://127.0.0.1:3100";
const apiURL = "http://127.0.0.1:18686";
const token = "ci-go-api-fixture-only-secret-32-characters";
const password = "ci-browser-access-password";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "*.spec.ts",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL, trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    { command: "node e2e/fake-go-server.mjs", url: `${apiURL}/healthz`, reuseExistingServer: !process.env.CI, timeout: 20_000 },
    {
      command: "npm run start -- -p 3100 -H 127.0.0.1",
      url: baseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 90_000,
      env: {
        AEGIS_WEB_MODE: "live",
        AEGIS_WEB_API_URL: apiURL,
        AEGIS_WEB_API_TOKEN: token,
        AEGIS_WEB_ACCESS_PASSWORD: password,
        AEGIS_WEB_SESSION_SECRET: "ci-session-secret-for-browser-e2e-never-use-live",
        AEGIS_WEB_RESEARCH_MODE: "PORTFOLIO_REVIEW",
      },
    },
  ],
});
