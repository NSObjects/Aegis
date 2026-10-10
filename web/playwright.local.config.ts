import { defineConfig, devices } from "@playwright/test";

const baseURL = "http://127.0.0.1:3200";
const backendURL = "http://127.0.0.1:18686";

export default defineConfig({
  testDir: "./e2e-local",
  testMatch: "*.spec.ts",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL, trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "node e2e/fake-go-server.mjs",
      url: `${backendURL}/healthz`,
      reuseExistingServer: !process.env.CI,
      timeout: 25_000,
      env: { AEGIS_TEST_FIXTURE_LOCAL_DEV: "1" },
    },
    {
      command: "npm run dev:live -- -p 3200",
      url: baseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        AEGIS_WEB_API_URL: backendURL,
        AEGIS_WEB_RESEARCH_MODE: "PORTFOLIO_REVIEW",
        AEGIS_WEB_API_TOKEN: "",
        AEGIS_WEB_ACCESS_PASSWORD: "",
        AEGIS_WEB_SESSION_SECRET: "",
      },
    },
  ],
});
