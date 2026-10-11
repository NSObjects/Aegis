import { expect, test } from "@playwright/test";

test("completed live research exposes only backend-linked evidence and argument navigation", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  const runId = await page.locator(".live-run-fields code").innerText();
  expect(runId).toMatch(/^[0-9a-f]{32}$/);

  const inspector = page.locator("#report-evidence-inspector");
  await expect(inspector.getByRole("heading", { name: "论点与证据检查器" })).toBeVisible({ timeout: 25_000 });
  await expect(inspector).toContainText("fixture-a1");
  await expect(inspector).toContainText("UNRESOLVED");
  await inspector.getByRole("button", { name: /fixture-a1/ }).click();
  await expect(inspector).toContainText("当前选中");
  await expect(inspector).toContainText("FIXTURE_UNRESOLVED_ARGUMENT");
  await expect(inspector).toContainText("领域级引用");
  await expect(inspector).toContainText("Ledger 没有记录针对该论点的反驳");
  await expect(inspector).toContainText("没有通过 argument_ids 明确连接的经理观点");
  // The fixture contains only a domain reference, never an external source URL.
  await expect(inspector.locator("a")).toHaveCount(0);

  await page.getByRole("button", { name: "研究历史" }).click();
  await expect(inspector).toHaveCount(0);
  await page.goto("/?run=" + runId);
  await expect(inspector.getByRole("heading", { name: "论点与证据检查器" })).toBeVisible();
});

test("a nonterminal run cannot display the selected study's evidence inspector", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
  await page.getByLabel("研究关注点（可选）").fill("hold fixture");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  await expect(page.locator(".live-run-fields")).toContainText("600519.SH");
  await expect(page.locator("#report-evidence-inspector")).toHaveCount(0);
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  await expect(page.locator("#report-evidence-inspector")).toHaveCount(0);
});
