import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

// PROTOCOL_FIXTURE_ONLY: validates authenticated UI, downloads and durable
// Run deep-link against the synthetic Go protocol server, not a paid provider.
test("completed run exports Markdown and JSON for the same durable Run ID", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
  await page.locator("details.live-create").evaluate((el: HTMLDetailsElement) => { el.open = true; }, { timeout: 8_000 });
  await page.getByLabel("股票代码").fill("600519.SH", { timeout: 8_000 });
  await page.getByRole("button", { name: "启动真实研究" }).click();
  // Anchor to the submitted run first: a previously completed fixture run may
  // be selected initially and would otherwise satisfy the wait immediately.
  await expect(page).toHaveURL(/run=/, { timeout: 8_000 });
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });

  const permalink = page.locator(".live-run-fields a.live-permalink").first();
  const href = await permalink.getAttribute("href");
  expect(href).toMatch(/^\/\?run=[a-f0-9]{32}$/);
  const runId = new URL(href!, "http://localhost").searchParams.get("run");
  expect(runId).toBeTruthy();

  const [md] = await test.step("download Markdown for persisted Run", async () => Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "导出 Markdown" }).click(),
  ]));
  expect(md.suggestedFilename()).toBe(`aegis-${runId}-report.md`);
  const mdText = await readFile(await md.path(), "utf8");
  expect(mdText).toContain("# Aegis 研究报告");
  expect(mdText).toContain(runId!);
  expect(mdText).toContain("600519.SH");
  expect(mdText).toContain("NOT\\_PROVIDED");
  expect(mdText).toContain("研究辅助材料");

  const [json] = await test.step("download JSON for same Run", async () => Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "导出真实 JSON" }).click(),
  ]));
  expect(json.suggestedFilename()).toBe(`aegis-${runId}-artifact.json`);
  const artifact: unknown = JSON.parse(await readFile(await json.path(), "utf8"));
  expect(artifact).toMatchObject({ research_artifact: { symbol: "600519.SH" } });
  expect(JSON.stringify(artifact)).not.toContain("ci-browser-access-password");

  await page.goto(href!);
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; }, { timeout: 8_000 });
  await expect(page.locator(".live-run-fields")).toContainText(runId!, { timeout: 10_000 });
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeEnabled();
});
