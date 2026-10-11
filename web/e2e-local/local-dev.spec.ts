import { expect, test } from "@playwright/test";

test("zero-secret loopback Live Web opens without login and can run a task", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible({ timeout: 20_000 });
  expect(await page.getByRole("heading", { name: "AEGIS · 研究工作台" }).count()).toBe(0);
  const session = await page.request.get("/api/workbench/session");
  expect(session.status()).toBe(200);
  const info = await session.json();
  expect(info).toMatchObject({ authenticated: true, local_dev: true, configured: true });

  const list = await page.request.get("/api/workbench/runs");
  expect(list.status()).toBe(200);

  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 20_000 });
  await expect(page.getByText("PROTOCOL_FIXTURE_ONLY", { exact: false }).first()).toBeVisible({ timeout: 20_000 });
});

test("zero-secret dev Web can save provider configuration without login", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.getByRole("heading", { name: "模型与数据源凭证" })).toBeVisible();
  const secret = "fixture-local-browser-key-never-used-for-payments";
  await page.getByLabel("DeepSeek API Key").fill(secret);
  const responsePromise = page.waitForResponse(r => r.url().endsWith("/api/workbench/credentials") && r.request().method() === "PUT");
  await page.getByRole("button", { name: "保存凭证" }).click();
  expect((await responsePromise).status()).toBe(200);
  await expect(page.locator(".provider-settings-form .live-info")).toContainText("已保存至 Go 服务端");
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  expect(await page.locator("body").innerText()).not.toContain(secret);
  await page.reload();
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.locator(".provider-settings-row").filter({ hasText: "DeepSeek" })).toContainText("已配置");
});

test("default local dev entry exposes the new operations overview rather than the legacy demo", async ({ page }) => {
  await page.goto("/?view=overview");
  await expect(page.getByRole("heading", { name: "研究任务总览" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: "研究概览", exact: true })).toBeVisible();
  const session = await page.request.get("/api/workbench/session");
  expect((await session.json()).local_dev).toBe(true);
  expect(await page.getByText("DEMO MODE", { exact: false }).count()).toBe(0);
});
