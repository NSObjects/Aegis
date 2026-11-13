import { expect, test } from "@playwright/test";

// Actual Go process + Next.js BFF; no fake-go-server and no paid providers.
// Verify browser navigation uses persisted Run metadata, not synthetic reports.
test("real Go navigation keeps selected failed Run across overview, history and browser back", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "工作台", exact: true })).toBeVisible({ timeout: 30_000 });
  await page.getByRole("button", { name: "发起新研究", exact: true }).click();
  await expect(page.getByRole("heading", { name: "研究详情", exact: true })).toBeVisible();

  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("失败", { timeout: 30_000 });
  const id = await page.locator(".live-run-fields code").innerText();
  expect(id).toMatch(/^[a-f0-9]{32}$/);
  await expect(page).toHaveURL(new RegExp("run=" + id + "$"));

  await page.getByRole("button", { name: "工作台", exact: true }).click();
  await expect(page).toHaveURL(/view=overview$/);
  await expect(page.getByRole("region", { name: "研究任务概览" })).toBeVisible();
  await page.getByRole("button", { name: "研究档案" }).click();
  await expect(page).toHaveURL(/view=history$/);
  await expect(page.locator(".live-history-row").filter({ hasText: id.slice(0, 12) })).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/view=overview$/);
  await page.goBack();
  await expect(page).toHaveURL(new RegExp("run=" + id + "$"));
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator(".live-run-fields")).toContainText("失败");
  await expect(page.locator("details.live-export-menu")).toHaveCount(0);
});
