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
  expect(href?.startsWith("/?run=")).toBe(true);
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

test("two real Go failed Runs retain distinct identities and no cross-Run artifact", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30_000 });
  const ids: string[] = [];
  for (const symbol of ["600519.SH", "000001.SZ"]) {
    await page.getByLabel("股票代码").fill(symbol);
    await page.getByRole("button", { name: "启动真实研究" }).click();
    await expect(page.locator(".live-run-fields")).toContainText("失败", { timeout: 30_000 });
    const link = await page.locator(".live-run-fields a.live-permalink").first().getAttribute("href");
    const id = new URL(link!, "http://localhost").searchParams.get("run");
    expect(id).toMatch(/^[a-f0-9]{32}$/);
    ids.push(id!);
  }
  expect(ids[0]).not.toBe(ids[1]);
  for (const [i, symbol] of ["600519.SH", "000001.SZ"].entries()) {
    await page.goto("/?run=" + ids[i]);
    await expect(page.locator(".live-run-fields")).toContainText(symbol, { timeout: 15_000 });
    await expect(page.locator(".live-run-fields")).toContainText(ids[i]);
    await expect(page.locator(".live-run-fields")).toContainText("失败");
    await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();
    const artifact = await page.request.get("/api/workbench/runs/" + ids[i] + "/artifact");
    expect(artifact.status()).not.toBe(200);
  }
});

test("unknown Run deep link never renders another Run or exposes artifact", async ({ page }) => {
  const unknown = "f".repeat(32);
  await page.goto("/?run=" + unknown);
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30_000 });
  await expect(page.locator(".live-run-detail")).toContainText("找不到任务", { timeout: 15_000 });
  await expect(page.locator(".live-run-fields")).toHaveCount(0);
  const missing = await page.request.get("/api/workbench/runs/" + unknown);
  expect(missing.status()).toBe(404);
  const artifact = await page.request.get("/api/workbench/runs/" + unknown + "/artifact");
  expect(artifact.status()).not.toBe(200);
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
});

test("transient API outage preserves Run selection and recovers on manual refresh", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30_000 });
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  const fields = page.locator(".live-run-fields");
  await expect(fields).toContainText("失败", { timeout: 30_000 });
  const href = await fields.locator("a.live-permalink").first().getAttribute("href");
  const runId = new URL(href!, "http://localhost").searchParams.get("run");
  expect(runId).toMatch(/^[a-f0-9]{32}$/);

  await page.route("**/api/workbench/runs", async route => {
    await route.abort("failed");
  });
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(page.getByRole("alert").first()).toBeVisible({ timeout: 15_000 });
  await expect(fields).toContainText(runId!);
  await expect(fields).toContainText("失败");
  await page.unroute("**/api/workbench/runs");

  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(fields).toContainText(runId!, { timeout: 15_000 });
  await expect(fields).toContainText("600519.SH");
  await page.goto(href!);
  await expect(fields).toContainText(runId!, { timeout: 15_000 });
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();
});

test("real Go browser rerun creates a distinct failed Run with provenance and no artifact", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 30_000 });
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  const fields = page.locator(".live-run-fields");
  await expect(fields).toContainText("失败", { timeout: 30_000 });
  const parentHref = await fields.locator("a.live-permalink").first().getAttribute("href");
  const parent = new URL(parentHref!, "http://localhost").searchParams.get("run");
  expect(parent).toMatch(/^[a-f0-9]{32}$/);

  await page.getByRole("button", { name: "再次研究该标的" }).click();
  await expect(page.locator(".live-rerun-notice")).toContainText("再次研究");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(fields).toContainText("失败", { timeout: 30_000 });
  const childHref = await fields.locator("a.live-permalink").first().getAttribute("href");
  const child = new URL(childHref!, "http://localhost").searchParams.get("run");
  expect(child).toMatch(/^[a-f0-9]{32}$/);
  expect(child).not.toBe(parent);
  const result = await page.request.get("/api/workbench/runs/" + child);
  expect(result.status()).toBe(200);
  const run = await result.json();
  expect(run.rerun_of).toBe(parent);
  expect(run.symbol).toBe("600519.SH");
  expect(run.state).toBe("FAILED");
  const artifact = await page.request.get("/api/workbench/runs/" + child + "/artifact");
  expect(artifact.status()).not.toBe(200);
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeDisabled();
});
