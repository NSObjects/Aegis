import { expect, test } from "@playwright/test";

// PROTOCOL_FIXTURE_ONLY: exercises browser cancel/reload UX with synthetic Go API.
// Real RunManager cancellation and durable storage are independently verified by #269.
test("cancel active research shows terminal state and never enables exports after reload", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "工作台", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "发起新研究", exact: true }).click();
  await expect(page.getByRole("heading", { name: "研究详情", exact: true })).toBeVisible();
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByLabel("研究关注点（可选）").fill("hold browser cancellation fixture");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
  const run = page.locator(".live-run-fields");
  await expect(run).toContainText("600519.SH");
  const href = await run.locator("a.live-permalink").first().getAttribute("href");
  expect(href?.startsWith("/?run=")).toBe(true);
  const id = new URL(href!, "http://localhost").searchParams.get("run");
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(run).toContainText("已取消", { timeout: 15_000 });
  await expect(page.locator("details.live-export-menu")).toHaveCount(0);
  const artifact = await page.request.get("/api/workbench/runs/" + id + "/artifact");
  expect(artifact.status()).not.toBe(200);
  await page.goto(href!);
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await expect(run).toContainText("已取消", { timeout: 15_000 });
  await expect(run).toContainText(id!);
  await expect(page.locator("details.live-export-menu")).toHaveCount(0);
});
