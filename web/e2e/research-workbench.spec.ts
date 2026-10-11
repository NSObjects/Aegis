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
  await expect(page.locator(".provider-settings-form .live-info")).toContainText("已保存至 Go 服务端");
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  expect(await page.locator("body").innerText()).not.toContain(key);
  await page.reload();
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  await expect(page.locator(".provider-settings-row").filter({ hasText: "DeepSeek" })).toContainText("已配置");
});


test("Live Run permalink, refresh and browser back/forward restore the correct workspace", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page).toHaveURL(new RegExp(`\\?run=${id}$`));
  await expect(page.locator(".live-run-fields")).toContainText(id);

  await page.getByRole("button", { name: "研究历史" }).click();
  await expect(page).toHaveURL(/\?view=history$/);
  await expect(page.getByRole("heading", { name: "研究历史" })).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(new RegExp(`\\?run=${id}$`));
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await page.goForward();
  await expect(page.getByRole("heading", { name: "研究历史" })).toBeVisible();

  await page.locator(".live-history-row").filter({ hasText: id.slice(0, 12) }).click();
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator(".live-permalink")).toHaveAttribute("href", `/?run=${id}`);

  await page.goto("/?view=settings");
  await expect(page.getByRole("heading", { name: "数据源与模型凭证" })).toBeVisible();
  await page.goto(`/?run=${id}`);
  await expect(page.locator(".live-run-fields")).toContainText(id);
});

test("unknown Live Run permalink never substitutes the latest report", async ({ page }) => {
  await signIn(page);
  await submit(page);
  const unknown = "ffffffffffffffffffffffffffffffff";
  await page.goto(`/?run=${unknown}`);
  await expect(page.getByText(`找不到任务 ${unknown}`, { exact: false })).toBeVisible();
  await expect(page.locator(".live-run-fields")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
});


test("Live structured report reads Go-shaped fixture fields without fabricated financial values", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toBeVisible({ timeout: 25_000 });
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator("#report-synthesis")).toContainText("PROTOCOL_FIXTURE_ONLY");
  await expect(page.locator("#report-market")).toContainText("20261009");
  await expect(page.locator("#report-technical")).toContainText("NO_DATA");
  await expect(page.locator("#report-event")).toContainText("NO_EVENT");
  await expect(page.locator("#report-fundamental")).toHaveCount(0);
  await expect(page.locator("#report-manager")).toContainText("INSUFFICIENT_EVIDENCE");
  await expect(page.locator("#report-debate")).toContainText("FIXTURE_BULL_ONLY");
  await expect(page.locator("#report-ledger")).toContainText("领域引用（无具体事件 ID）");
  await expect(page.locator("#report-portfolio")).toContainText("NOT_PROVIDED · 未知（不能解释为零持仓）");
  await expect(page.locator("#report-portfolio")).toContainText("FIXTURE_NO_REAL_ACCOUNT");
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeEnabled();
  await page.getByText("高级：查看后端原始 JSON 工件").click();
  await expect(page.getByText("完整 Typed Artifact")).toBeVisible();
});


test("completed Run creates a new study with durable origin metadata and no copied query", async ({ page }) => {
  await signIn(page);
  const first = await submit(page, "original research focus");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 20_000 });
  await page.getByRole("button", { name: "再次研究该标的" }).click();
  await expect(page.getByText("仅预填标的，不会复制旧的关注点", { exact: false })).toBeVisible();
  await expect(page.getByLabel("股票代码")).toHaveValue("600519.SH");
  await expect(page.getByLabel("研究关注点（可选）")).toHaveValue("");
  await page.getByLabel("研究关注点（可选）").fill("independent follow-up analysis");
  const nextRequest = page.waitForRequest(r => r.url().endsWith("/api/workbench/runs") && r.method() === "POST");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  const payload = JSON.parse((await nextRequest).postData() || "{}");
  expect(payload).toMatchObject({
    symbol: "600519.SH", mode: "PORTFOLIO_REVIEW", rerun_of: first,
    market_query: "independent follow-up analysis", portfolio: { state: "NOT_PROVIDED" },
  });
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  const second = await page.locator(".live-run-fields code").innerText();
  expect(second).toMatch(/^[a-f0-9]{32}$/);
  expect(second).not.toBe(first);
  await expect(page.locator(".live-run-fields")).toContainText("来源研究任务");
  await expect(page.locator(".live-run-fields a.live-permalink").last()).toHaveAttribute("href", "/?run=" + first);
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(second);
  await expect(page.locator(".live-run-fields")).toContainText(first);
  await page.goto("/?run=" + first);
  await expect(page.locator(".live-run-fields")).toContainText(first);
  await expect(page.locator(".live-run-fields")).not.toContainText("来源研究任务");
});

test("failed and cancelled studies can be researched again without replaying old failure conditions", async ({ page }) => {
  await signIn(page);
  const failed = await submit(page, "fail provider fixture");
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
  await page.getByRole("button", { name: "再次研究该标的" }).click();
  await expect(page.getByLabel("研究关注点（可选）")).toHaveValue("");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  const retried = await page.locator(".live-run-fields code").innerText();
  expect(retried).not.toBe(failed);
  await expect(page.locator(".live-run-fields")).toContainText(failed);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await page.goto("/?run=" + failed);
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED");
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();

  const held = await submit(page, "hold fixture");
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  await page.getByRole("button", { name: "再次研究该标的" }).click();
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  const fresh = await page.locator(".live-run-fields code").innerText();
  expect(fresh).not.toBe(held);
  await expect(page.locator(".live-run-fields")).toContainText(held);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
});

test("protocol refuses nonterminal, unknown and cross-symbol rerun provenance", async ({ page }) => {
  await signIn(page);
  const run = await submit(page, "hold fixture");
  const post = async (symbol: string, rerun_of: string) => page.evaluate(async ({ symbol, rerun_of }) => {
    const response = await fetch("/api/workbench/runs", {
      method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json",
        "Idempotency-Key": crypto.randomUUID() }, body: JSON.stringify({
        symbol, mode: "PORTFOLIO_REVIEW", portfolio: { state: "NOT_PROVIDED" }, rerun_of,
      }),
    });
    return response.status;
  }, { symbol, rerun_of });
  expect(await post("600519.SH", run)).toBe(400);
  expect(await post("600519.SH", "f".repeat(32))).toBe(400);
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  expect(await post("000001.SZ", run)).toBe(400);
  expect(await post("600519.SH", run)).toBe(202);
});
