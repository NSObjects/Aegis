import { expect, test } from "@playwright/test";

// Fixture-backed browser regression: no paid model/provider requests.
test("mobile failed Run remains failed after refresh and never exposes a report", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();

  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByLabel("研究关注点（可选）").fill("fail provider fixture");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await expect.poll(async () => page.locator(".live-run-fields code").innerText(), { timeout: 15_000 }).toMatch(/^[a-f0-9]{32}$/);
  const id = await page.locator(".live-run-fields code").innerText();
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toHaveCount(0);

  await page.goto("/?run=" + id);
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED");
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toHaveCount(0);
});
