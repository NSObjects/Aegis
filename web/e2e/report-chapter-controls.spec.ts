import { expect, test } from "@playwright/test";

// Uses the existing fixed Go-protocol fixture in playwright.config.ts, never a paid model.
test("completed report expands and collapses only the structured chapters", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();

  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });

  const chapters = page.locator("details.live-brief-chapter");
  await expect(chapters.first()).toBeVisible();
  const count = await chapters.count();
  expect(count).toBeGreaterThan(0);
  const raw = page.locator("details.live-report-source-details");

  await page.getByRole("button", { name: "展开所有章节" }).click();
  expect(await chapters.evaluateAll(nodes => nodes.every(node => (node as HTMLDetailsElement).open))).toBe(true);
  await expect(raw).not.toHaveAttribute("open");

  await page.getByRole("button", { name: "收起所有章节" }).click();
  expect(await chapters.evaluateAll(nodes => nodes.every(node => !(node as HTMLDetailsElement).open))).toBe(true);

  // Direct hash navigation and browser hash changes reveal a collapsed chapter.
  const firstID = await chapters.first().getAttribute("id");
  expect(firstID).toMatch(/^chapter-[a-z-]+$/);
  const hash = "#report-" + firstID!.slice("chapter-".length);
  await page.evaluate(nextHash => { window.location.hash = nextHash; }, hash);
  await expect(chapters.first()).toHaveAttribute("open");
  await page.getByRole("button", { name: "收起所有章节" }).click();
  await expect(chapters.first()).not.toHaveAttribute("open");
  await page.evaluate(nextHash => {
    window.location.hash = "#other";
    window.location.hash = nextHash;
  }, hash);
  await expect(chapters.first()).toHaveAttribute("open");

  await page.getByRole("button", { name: "收起所有章节" }).click();
  // Individual chapters still use native keyboard-accessible details/summary.
  await chapters.first().locator("summary").click();
  await expect(chapters.first()).toHaveAttribute("open");
  await expect(chapters.nth(1)).not.toHaveAttribute("open");
});
