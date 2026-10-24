import { expect, test } from "@playwright/test";

// Fixture-backed browser coverage, not a Hosted Provider end-to-end assertion.
test("mobile cancelled research stays cancelled after deep-link reload", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "工作台", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "发起新研究", exact: true }).click();
  await expect(page.getByRole("heading", { name: "研究详情", exact: true })).toBeVisible();

  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByLabel("研究关注点（可选）").fill("hold fixture");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await expect.poll(async () => page.locator(".live-run-fields code").innerText(), { timeout: 15_000 }).toMatch(/^[a-f0-9]{32}$/);
  const id = await page.locator(".live-run-fields code").innerText();
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  await expect(page.locator("details.live-export-menu")).toHaveCount(0);
  await expect(page.locator("details.live-export-menu")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toHaveCount(0);

  await page.goto("/?run=" + id);
  await expect(page.locator(".live-run-fields")).toContainText("已取消");
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator("details.live-export-menu")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toHaveCount(0);
});
