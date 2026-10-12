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

test("real Go run fails closed without provider credentials and remains inspectable after reload", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30_000 });
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("失败", { timeout: 30_000 });

  const href = await page.locator(".live-run-fields a.live-permalink").first().getAttribute("href");
  expect(href).toMatch(/^\\/\\?run=[a-f0-9]{32}$/);
  const runId = new URL(href!, "http://localhost").searchParams.get("run");
  expect(runId).toBeTruthy();

  const artifact = await page.request.get(`/api/workbench/runs/${runId}/artifact`);
  expect(artifact.status()).not.toBe(200);
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();

  await page.goto(href!);
  await expect(page.locator(".live-run-fields")).toContainText("失败", { timeout: 15_000 });
  await expect(page.locator(".live-run-fields")).toContainText(runId!);
});
