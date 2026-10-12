import { defineConfig, devices } from "@playwright/test";

// Explicitly opt-in via integration.yml run_live_web_api. This suite NEVER
// submits a Run; it only reads the completed real-provider Run created by
// aegis-live-check in the SAME workflow invocation.
export default defineConfig({
  testDir: "./e2e-hosted",
  testMatch: "*.spec.ts",
  workers: 1,
  retries: 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: "http://127.0.0.1:3230", trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [{
    command: "npm run dev -- -p 3230",
    url: "http://127.0.0.1:3230",
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      AEGIS_WEB_API_URL: "http://127.0.0.1:18686",
      AEGIS_WEB_API_TOKEN: "",
      AEGIS_WEB_ACCESS_PASSWORD: "",
      AEGIS_WEB_SESSION_SECRET: "",
      AEGIS_WEB_RESEARCH_MODE: "PORTFOLIO_REVIEW",
    },
  }],
});
