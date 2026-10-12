import { expect, test } from "@playwright/test";

// Real compiled Go process is killed and replaced on the same isolated store.
// Missing credentials force a real FAILED Run; no paid providers are invoked.
test("browser preserves failed Run and forbids export across actual Go process restart", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30000 });
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  const details = page.locator(".live-run-fields");
  await expect(details).toContainText("失败", { timeout: 30000 });
  const link = await details.locator("a.live-permalink").first().getAttribute("href");
  expect(link?.startsWith("/?run=")).toBe(true);
  const runId = new URL(link!, "http://localhost").searchParams.get("run");
  expect(runId).toMatch(/^[a-f0-9]{32}$/);

  const restart = await page.request.post("http://127.0.0.1:18788/restart", { timeout: 30000 });
  expect(restart.status()).toBe(200);
  await page.goto(link!);
  await expect(details).toContainText("失败", { timeout: 20000 });
  await expect(details).toContainText(runId!);
  await expect(details).toContainText("600519.SH");
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();

  const report = await page.request.get("/api/workbench/runs/" + runId + "/artifact");
  expect(report.status()).not.toBe(200);
  const history = await page.request.get("/api/workbench/runs");
  expect(history.status()).toBe(200);
  expect(await history.text()).toContain(runId!);
});
