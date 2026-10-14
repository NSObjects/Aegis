import { expect, test } from "@playwright/test";

// Uses the existing fixed Go-protocol fixture in playwright.config.ts, never a paid model.
test("completed report expands and collapses only the structured chapters", async ({ page }) => {
  test.setTimeout(60_000); // Includes fixture submission, persistence, report render, hash navigation and keyboard interaction.
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();

  await page.locator("details.live-create").evaluate((el: HTMLDetailsElement) => { el.open = true; }, { timeout: 8_000 });
  await expect(page.getByLabel("股票代码")).toBeVisible({ timeout: 8_000 });
  await page.getByLabel("股票代码").fill("600519.SH", { timeout: 8_000 });
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; }, { timeout: 8_000 });
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 12_000 });

  const chapters = page.locator("details.live-brief-chapter");
  await test.step("wait for source-owned report chapters", async () => {
    await expect(chapters.first()).toBeVisible({ timeout: 8_000 });
  });
  const count = await chapters.count();
  expect(count).toBeGreaterThan(0);
  const raw = page.locator("details.live-report-source-details");

  await test.step("expand report chapters without opening raw JSON", async () => {
  await page.getByRole("button", { name: "展开所有章节" }).click();
  expect(await chapters.evaluateAll(nodes => nodes.every(node => (node as HTMLDetailsElement).open))).toBe(true);
  await expect(raw).not.toHaveAttribute("open");
  });

  await page.getByRole("button", { name: "收起所有章节" }).click();
  expect(await chapters.evaluateAll(nodes => nodes.every(node => !(node as HTMLDetailsElement).open))).toBe(true);

  await test.step("hash navigation reveals collapsed chapters", async () => {
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
  });
  await test.step("native summary toggles only the selected chapter", async () => {
  // Individual chapters still use native keyboard-accessible details/summary.
  await chapters.first().locator("summary").click();
  await expect(chapters.first()).toHaveAttribute("open");
  await expect(chapters.nth(1)).not.toHaveAttribute("open");
  });
});
