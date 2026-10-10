import { expect, test, type Page } from "@playwright/test";

const password = "ci-browser-access-password";

async function signIn(page: Page) {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await page.getByLabel("工作台访问口令").fill(password);
  const loginResponsePromise = page.waitForResponse(r => r.url().endsWith("/api/workbench/session") && r.request().method() === "POST");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  const loginResponse = await loginResponsePromise;
  expect(loginResponse.status(), "login POST must succeed before showing the Live workspace").toBe(200);
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
}

async function submit(page: Page, query = "") {
  await page.getByLabel("股票代码").fill("600519.SH");
  if (query) await page.getByLabel("研究关注点（可选）").fill(query);
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  const id = await page.locator(".live-run-fields code").innerText();
  expect(id).toMatch(/^[a-f0-9]{32}$/);
  return id;
}

test("anonymous browser cannot read ResearchRun or bypass Web login", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  const response = await page.request.get("/api/workbench/runs");
  expect(response.status()).toBe(401);
  await page.getByLabel("工作台访问口令").fill("incorrect-unauthorized-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByText("访问口令不正确")).toBeVisible();
});

test("real browser submits a run, receives stage SSE, reads artifact, and restores history", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page.locator(".live-event-item").filter({ hasText: "Market" }).first()).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("PROTOCOL_FIXTURE_ONLY", { exact: false }).first()).toBeVisible({ timeout: 25_000 });
  await expect(page.getByText("FIXTURE_NO_REAL_ACCOUNT").first()).toBeVisible();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  const artifact = await page.request.get(`/api/workbench/runs/${id}/artifact`);
  expect(artifact.status()).toBe(200);
  expect((await artifact.json()).research_artifact.synthesis.summary).toContain("PROTOCOL_FIXTURE_ONLY");
  const events = await page.request.get(`/api/workbench/runs/${id}/events`);
  const history = (await events.json()).events as Array<{ seq: number; status: string }>;
  expect(history.length).toBeGreaterThan(2);
  expect(history.at(-1)?.status).toBe("COMPLETED");
  const stream = await page.request.get(`/api/workbench/runs/${id}/stream`, { headers: { "Last-Event-ID": "2" } });
  expect(stream.status()).toBe(200);
  expect(stream.headers()["content-type"]).toContain("text/event-stream");
  const replayed = await stream.text();
  expect(replayed).not.toContain("id: 1\n");
  expect(replayed).toContain("event: research_stage");
  await page.reload();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
  await page.getByRole("button", { name: "研究历史" }).click();
  await expect(page.locator(".live-history-row").filter({ hasText: id.slice(0,12) }).first()).toBeVisible();
  await page.locator(".live-history-row").filter({ hasText: id.slice(0,12) }).click();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.getByText("PROTOCOL_FIXTURE_ONLY", { exact: false }).first()).toBeVisible();
});

test("cancelled research never exposes an artifact", async ({ page }) => {
  await signIn(page);
  const id = await submit(page, "hold fixture");
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  const artifact = await page.request.get(`/api/workbench/runs/${id}/artifact`);
  expect(artifact.status()).toBe(409);
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
});

test("provider failure is terminal and cannot be presented as success", async ({ page }) => {
  await signIn(page);
  const id = await submit(page, "fail provider fixture");
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  const artifact = await page.request.get(`/api/workbench/runs/${id}/artifact`);
  expect(artifact.status()).toBe(409);
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
});

test("settings lets a signed-in operator save provider credentials without echoing keys", async ({ page }) => {
  await signIn(page);
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.getByRole("heading", { name: "模型与数据源凭证" })).toBeVisible();
  const key = "synthetic-browser-credential-no-provider-calls";
  await page.getByLabel("DeepSeek API Key").fill(key);
  const response = page.waitForResponse(r => r.url().endsWith("/api/workbench/credentials") && r.request().method() === "PUT");
  await page.getByRole("button", { name: "保存凭证" }).click();
  expect((await response).status()).toBe(200);
  await expect(page.getByRole("status")).toContainText("已保存至 Go 服务端");
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  expect(await page.locator("body").innerText()).not.toContain(key);
  await page.reload();
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  await expect(page.locator(".provider-settings-row").filter({ hasText: "DeepSeek" })).toContainText("已配置");
});
