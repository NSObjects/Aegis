import { expect, test } from "@playwright/test";

// Runs against the real go binary. No task submission, no provider credentials,
// and no fixture service; tests the actual browser -> Next proxy -> Go boundary.
test("real Go loopback server powers the live console without a provider", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30_000 });
  const session = await page.request.get("/api/workbench/session");
  expect(session.status()).toBe(200);
  expect(await session.json()).toMatchObject({ configured: true, authenticated: true, local_dev: true });
  const history = await page.request.get("/api/workbench/runs");
  expect(history.status()).toBe(200);
  const response = await history.json();
  expect(response).toHaveProperty("runs");
  await expect(page.getByText("DEMO MODE", { exact: false })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
});
