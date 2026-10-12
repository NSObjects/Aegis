import { defineConfig, devices } from "@playwright/test";

// REAL GO SERVER: no fake Go API, no paid research, local-only no-auth mode.
const baseURL = "http://127.0.0.1:3210";
const apiURL = "http://127.0.0.1:18787";
export default defineConfig({
  testDir: "./e2e-real-go",
  testMatch: "*.spec.ts",
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL, trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "cd .. && go run ./cmd/aegis-web -dev -listen 127.0.0.1:18787 -store .aegis/ci-real-go-runs -credential-store .aegis/ci-real-go-credentials.enc -evidence-store .aegis/ci-real-go-evidence",
      url: apiURL + "/api/v1/research/runs",
      reuseExistingServer: false,
      timeout: 120_000,
      env: { AEGIS_WEB_API_TOKEN: "", AEGIS_CREDENTIAL_MASTER_KEY: "" },
    },
    {
      command: "npm run dev -- -p 3210",
      url: baseURL,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        AEGIS_WEB_API_URL: apiURL,
        AEGIS_WEB_API_TOKEN: "",
        AEGIS_WEB_ACCESS_PASSWORD: "",
        AEGIS_WEB_SESSION_SECRET: "",
        AEGIS_WEB_RESEARCH_MODE: "PORTFOLIO_REVIEW",
      },
    },
  ],
});
