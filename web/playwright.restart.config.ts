import { defineConfig, devices } from "@playwright/test";

// Isolated real Go process restart acceptance; no fixture or provider credentials.
export default defineConfig({
  testDir: "./e2e-real-go",
  testMatch: "restart-browser.spec.ts",
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: "http://127.0.0.1:3220", trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    { command: "node e2e-real-go/restartable-server.mjs", url: "http://127.0.0.1:18788/health", reuseExistingServer: false, timeout: 150_000 },
    { command: "npm run dev -- -p 3220", url: "http://127.0.0.1:3220", reuseExistingServer: false, timeout: 120_000,
      env: { AEGIS_WEB_API_URL: "http://127.0.0.1:18789", AEGIS_WEB_API_TOKEN: "",
        AEGIS_WEB_ACCESS_PASSWORD: "", AEGIS_WEB_SESSION_SECRET: "", AEGIS_WEB_RESEARCH_MODE: "PORTFOLIO_REVIEW" } },
  ],
});
