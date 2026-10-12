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

test("persisted request snapshot is readable across navigation without echoing the research query", async ({ page }) => {
  await signIn(page);
  const secretQuery = "PROVENANCE_QUERY_NOT_A_PUBLIC_HISTORY_FIELD";
  const id = await submit(page, secretQuery);
  const response = await page.request.get(`/api/workbench/runs/${id}`);
  expect(response.status()).toBe(200);
  const raw = await response.text();
  expect(raw).not.toContain(secretQuery);
  const record = JSON.parse(raw);
  expect(record.request_snapshot).toMatchObject({
    schema_version: 1,
    market_query_provided: true,
    event_pit_safe: true,
    min_documents: 1,
    min_independent_sources: 1,
    portfolio_state: "NOT_PROVIDED",
  });
  expect(Date.parse(record.request_snapshot.event_window_start)).toBeLessThan(Date.parse(record.request_snapshot.event_window_end));
  await page.locator(".live-request-snapshot summary").click();
  await expect(page.locator(".live-request-snapshot")).toContainText("已提交（正文不保存）");
  await expect(page.locator(".live-request-snapshot")).toContainText("PIT-safe");
  await expect(page.locator(".live-request-snapshot")).toContainText("NOT_PROVIDED · 未知");
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator(".live-request-snapshot summary")).toContainText("已记录");
  await page.locator(".live-request-snapshot summary").click();
  await expect(page.locator(".live-request-snapshot")).toContainText("已提交（正文不保存）");
  expect(await page.locator("body").innerText()).not.toContain(secretQuery);
});

test("run's configured model identity stays attached to the same run after browser reload", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const res = await page.request.get(`/api/workbench/runs/${id}`);
  expect(res.status()).toBe(200);
  const metadata = await res.json();
  expect(metadata.model_provenance).toMatchObject({
    schema_version: 1,
    config_fingerprint: "a".repeat(64),
    bindings: [{ role: "market", profile: "fixture-profile", provider: "fixture-only", model: "fixture-model" }],
  });
  await page.locator(".live-model-provenance summary").click();
  await expect(page.locator(".live-model-provenance")).toContainText("fixture-model");
  await expect(page.locator(".live-model-provenance")).toContainText("模型 ID 不是第三方 Provider 保证的固定版本号");
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator(".live-model-provenance summary")).toContainText("已记录");
  await page.locator(".live-model-provenance summary").click();
  await expect(page.locator(".live-model-provenance")).toContainText("fixture-profile");
  await expect(page.locator(".live-model-provenance")).not.toContainText("API_KEY");
});

test("completed portfolio source IDs stay attached to the original historical Run", async ({ page }) => {
  await signIn(page);
  const id = await submit(page, "historical comparison fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const response = await page.request.get(`/api/workbench/runs/${id}`);
  expect(response.status()).toBe(200);
  const meta = await response.json();
  expect(meta.evidence_index).toMatchObject({
    schema_version: 1,
    status: "NORMAL",
    events: [{ event_id: "fixture-comparison-event-1", document_ids: ["fixture-doc-1"], source_count: 1 }],
    document_ids: ["fixture-doc-1"],
  });
  const panel = page.locator(".live-evidence-source-index");
  await expect(panel.locator("summary")).toContainText("已记录");
  await panel.locator("summary").click();
  await expect(panel).toContainText("fixture-comparison-event-1");
  await expect(panel).toContainText("fixture-doc-1");
  await expect(panel).toContainText("FIXTURE_PUBLISHER_ONLY");
  await expect(panel).toContainText("2026-10-11T00:00:00Z");
  await expect(panel).toContainText("fixture-version-1");
  await expect(panel.locator(".live-evidence-source-records a")).toHaveAttribute("href", "https://example.org/fixture-document");
  await expect(panel.locator(".live-evidence-source-records a")).toHaveAttribute("rel", "noopener noreferrer");
  await expect(panel).toContainText("文档 ID");
  await expect(panel.locator("a")).toHaveCount(1);
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(panel.locator("summary")).toContainText("已记录");
  await panel.locator("summary").click();
  await expect(panel).toContainText("fixture-doc-1");
  await expect(panel).toContainText("FIXTURE_PUBLISHER_ONLY");
});

test("source index differentiates explicit NO_EVENT from failed research", async ({ page }) => {
  await signIn(page);
  const noEventId = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const noEvent = await (await page.request.get(`/api/workbench/runs/${noEventId}`)).json();
  expect(noEvent.evidence_index).toMatchObject({ schema_version: 1, status: "NO_EVENT", events: [], document_ids: [] });
  const panel = page.locator(".live-evidence-source-index");
  await panel.locator("summary").click();
  await expect(panel).toContainText("不是自动证明市场没有相关新闻");

  await page.getByLabel("研究关注点（可选）").fill("fail synthetic source");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  const failingId = await page.locator(".live-run-fields code").innerText();
  expect(failingId).not.toBe(noEventId);
  await expect(page.locator(".live-run-fields")).toContainText("失败", { timeout: 25_000 });
  const failed = await (await page.request.get(`/api/workbench/runs/${failingId}`)).json();
  expect(failed.evidence_index).toBeUndefined();
  await expect(page.locator(".live-evidence-source-index")).toHaveCount(0);
});

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
  await expect(page.locator(".live-brief-lead")).toContainText("FIXTURE_ONLY_MANAGER_NO_RESEARCH", { timeout: 25_000 });
  await page.locator("#chapter-synthesis summary").click();
  await expect(page.locator("#report-synthesis")).toContainText("PROTOCOL_FIXTURE_ONLY");
  await page.locator("#chapter-portfolio summary").click();
  await expect(page.locator("#report-portfolio")).toContainText("FIXTURE_NO_REAL_ACCOUNT");
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
  await expect(page.locator(".live-brief-lead")).toContainText("FIXTURE_ONLY_MANAGER_NO_RESEARCH");
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
  const diagnostic = page.getByRole("status", { name: "任务失败诊断" });
  await expect(diagnostic).toContainText("RESEARCH_FAILED");
  await expect(diagnostic).toContainText(id);
  await expect(diagnostic).toContainText("Event");

  const artifact = await page.request.get(`/api/workbench/runs/${id}/artifact`);
  expect(artifact.status()).toBe(409);
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();
});

test("settings lets a signed-in operator save provider credentials without echoing keys", async ({ page }) => {
  await signIn(page);
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.getByRole("heading", { name: "模型与数据源凭证" })).toBeVisible();
  const groups = page.locator(".provider-settings-group");
  await expect(groups).toHaveCount(3);
  await expect(groups.nth(0).getByRole("heading", { name: "AI 模型" })).toBeVisible();
  await expect(groups.nth(1).getByRole("heading", { name: "金融数据" })).toBeVisible();
  await expect(groups.nth(2).getByRole("heading", { name: "资讯搜索" })).toBeVisible();
  await expect(groups.nth(0).getByLabel("DeepSeek API Key")).toBeVisible();
  await expect(groups.nth(1).getByLabel("TuShare Token")).toBeVisible();
  await expect(groups.nth(2).getByLabel("Tavily API Key")).toBeVisible();

  const modelKey = "synthetic-browser-model-key-no-provider-calls";
  const marketKey = "synthetic-browser-market-token-no-provider-calls";
  await page.getByLabel("DeepSeek API Key").fill(modelKey);
  await page.getByLabel("TuShare Token").fill(marketKey);
  const response = page.waitForResponse(r => r.url().endsWith("/api/workbench/credentials") && r.request().method() === "PUT");
  await page.getByRole("button", { name: "保存凭证" }).click();
  expect((await response).status()).toBe(200);
  await expect(page.locator(".provider-settings-form .live-info")).toContainText("已保存至 Go 服务端");
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  await expect(page.getByLabel("TuShare Token")).toHaveValue("");
  expect(await page.locator("body").innerText()).not.toContain(modelKey);
  expect(await page.locator("body").innerText()).not.toContain(marketKey);

  await page.reload();
  await page.getByRole("button", { name: "凭证设置" }).click();
  await expect(page.getByLabel("DeepSeek API Key")).toHaveValue("");
  await expect(page.getByLabel("TuShare Token")).toHaveValue("");
  await expect(page.locator(".provider-settings-row").filter({ hasText: "DeepSeek" })).toContainText("已配置");
  await expect(page.locator(".provider-settings-row").filter({ hasText: "TuShare" })).toContainText("已配置");
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


test("observed Go stages show duration, lifecycle, links and restore after refresh", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const trace = page.getByRole("region", { name: "Agent 执行轨迹" });
  await expect(trace.locator(".live-trace-stage").filter({ hasText: "Market" })).toContainText("成功");
  await expect(trace.locator(".live-trace-stage").filter({ hasText: "Market" })).toContainText("Go 记录耗时");
  await expect(trace.locator(".live-trace-stage").filter({ hasText: "Event" })).toContainText("成功");
  await expect(trace.locator(".live-trace-lifecycle")).toContainText("COMPLETED");
  await expect(trace.locator(".live-trace-stage")).toHaveCount(2);
  await expect(trace.locator(".live-trace-stage").filter({ hasText: "Market" }).locator("a")).toHaveAttribute("href", "#report-market");
  await expect(trace.getByText("未提供脱敏工具调用", { exact: false })).toBeVisible();
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator(".live-trace-stage").filter({ hasText: "Market" })).toContainText("成功");
  await expect(page.locator(".live-event-item").filter({ hasText: "Market" }).first()).toBeVisible();
});

test("failed and cancelled Go Runs show only observed stage failures, never inferred successes", async ({ page }) => {
  await signIn(page);
  const failed = await submit(page, "fail provider fixture");
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  const trace = page.getByRole("region", { name: "Agent 执行轨迹" });
  await expect(trace.locator(".live-trace-error")).toContainText("Event");
  await expect(trace.locator(".live-trace-stage").filter({ hasText: "Event" })).toContainText("失败");
  await expect(trace.locator(".live-trace-stage a.live-trace-report-link")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeDisabled();

  const held = await submit(page, "hold fixture");
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  await expect(trace.getByText("未收到 DONE 的阶段不代表成功", { exact: false })).toBeVisible();
  await expect(trace.locator(".live-trace-stage a.live-trace-report-link")).toHaveCount(0);
  await page.goto("/?run=" + failed);
  await expect(trace.locator(".live-trace-error")).toContainText("Event");
  await expect(page.locator(".live-run-fields")).toContainText(failed);
  await expect(page.locator(".live-run-fields")).not.toContainText(held);
});

test("persisted-event polling restores trace when EventSource is unavailable", async ({ page }) => {
  await signIn(page);
  await page.route("**/api/workbench/runs/*/stream", route => route.abort());
  const id = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await expect(page.getByRole("region", { name: "Agent 执行轨迹" }).locator(".live-trace-stage").filter({ hasText: "Market" })).toContainText("成功", { timeout: 20_000 });
  await expect(page.locator(".live-trace-lifecycle")).toContainText("COMPLETED");
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.locator(".live-trace-stage").filter({ hasText: "Event" })).toContainText("成功");
});


test("completed Run presents one source-owned OHLC candle and technical values without a fabricated price series", async ({ page }) => {
  await signIn(page);
  const id = await submit(page, "technical snapshot fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await page.locator("#chapter-technical summary").click();
  const chart = page.getByRole("region", { name: "技术行情快照可视化" });
  await expect(chart).toBeVisible();
  await expect(chart.getByText("2026-10-09")).toBeVisible();
  await expect(chart.getByText("原始单日 OHLC · 仅一个交易日")).toBeVisible();
  await expect(chart.getByRole("img", { name: "单个有效交易日的开盘价、最高价、最低价和收盘价（非历史走势图）" })).toBeVisible();
  await expect(chart.locator("rect.technical-candle-body")).toHaveCount(1);
  await expect(chart.locator(".technical-candle-prices")).toContainText("1,500.12");
  await expect(chart.locator(".technical-candle-prices")).toContainText("1,531.4");
  await expect(chart.locator(".technical-candle-prices")).toContainText("1,487.8");
  await expect(chart.locator(".technical-candle-prices")).toContainText("1,512.65");
  await expect(chart).toContainText("MA5");
  await expect(chart).toContainText("1,507.2");
  await expect(chart).toContainText("RSI14");
  await expect(chart).toContainText("56.4");
  await expect(chart).toContainText("MACD HIST");
  await expect(chart).toContainText("量比 MA5");
  await expect(chart).toContainText("1.1053");
  await expect(chart).not.toContainText("MA20");
  const raw = await page.request.get("/api/workbench/runs/" + id + "/artifact");
  expect(raw.status()).toBe(200);
  const technical = (await raw.json()).research_artifact.technical;
  expect(technical.snapshot).toMatchObject({ open: 1500.12, high: 1531.4, low: 1487.8, close: 1512.65 });
  expect(technical.moving_averages.ma5).toBe(1507.2);
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await page.locator("#chapter-technical summary").click();
  await expect(page.getByRole("region", { name: "技术行情快照可视化" }).locator("rect.technical-candle-body")).toHaveCount(1);
});

test("NO_DATA never draws a synthetic OHLC candle", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await page.locator("#chapter-technical summary").click();
  const chart = page.getByRole("region", { name: "技术行情快照可视化" });
  await expect(chart).toContainText("不可绘制单日行情");
  await expect(chart).toContainText("NO_DATA");
  await expect(chart.locator("rect.technical-candle-body")).toHaveCount(0);
});


test("two completed research runs compare source-owned conclusion, market, technical, event and risk facts", async ({ page }) => {
  await signIn(page);
  const baseline = await submit(page, "technical snapshot fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const followup = await submit(page, "historical comparison fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const methods: string[] = [];
  page.on("request", req => {
    if (req.url().includes("/api/workbench/runs") && req.method() === "POST") methods.push("POST");
  });
  await page.getByRole("button", { name: "研究对比", exact: true }).click();
  await expect(page.getByRole("heading", { name: "两次研究记录对比" })).toBeVisible();
  await page.getByLabel("基准研究").selectOption(baseline);
  await page.getByLabel("对比研究").selectOption(followup);
  await expect(page).toHaveURL("/?view=compare&left=" + baseline + "&right=" + followup);
  const chart = page.getByRole("region", { name: "历史研究对比" });
  await expect(chart.getByRole("table", { name: "市场环境对比" })).toContainText("MIXED");
  await expect(chart.getByRole("table", { name: "市场环境对比" })).toContainText("RISK_OFF");
  await expect(chart.getByRole("table", { name: "技术分析对比" })).toContainText("1,512.65".replaceAll(",", ""));
  await expect(chart.getByRole("table", { name: "技术分析对比" })).toContainText("1528.2");
  await expect(chart.getByRole("table", { name: "风险门禁对比" })).toContainText("FIXTURE_NEW_RISK_REVIEW");
  await expect(chart.getByText("fixture-comparison-event-1")).toBeVisible();
  await expect(chart.getByText("仅基准有数据").first()).toBeVisible();
  await expect(chart.locator(".live-compare-heads code")).toContainText([baseline,followup]);
  expect(methods).toEqual([]);
  const reference = await page.request.get("/api/workbench/runs/" + followup + "/artifact");
  expect(reference.status()).toBe(200);
  const source = await reference.json();
  expect(source.research_artifact.technical.snapshot.close).toBe(1528.2);
  expect(source.research_artifact.market.regime).toBe("RISK_OFF");
  await page.reload();
  await expect(chart.getByRole("table", { name: "市场环境对比" })).toContainText("RISK_OFF");
  await expect(page.getByLabel("基准研究")).toHaveValue(baseline);
  await expect(page.getByLabel("对比研究")).toHaveValue(followup);
  await chart.getByRole("link", { name: "查看原任务与报告" }).first().click();
  await expect(page).toHaveURL("/?run=" + baseline);
  await expect(page.locator(".live-run-fields")).toContainText(baseline);
  await page.goBack();
  await expect(chart.getByText("fixture-comparison-event-1")).toBeVisible();
});

test("comparison refuses identical, failed and mismatched-symbol Runs, without mixing artifacts", async ({ page }) => {
  await signIn(page);
  const original = await submit(page, "technical snapshot fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await page.goto("/?view=compare&left=" + original + "&right=" + original);
  await expect(page.getByText("不能将同一 Run 与自己比较")).toBeVisible();
  await expect(page.locator(".live-compare-table")).toHaveCount(0);
  await page.goto("/?run=" + original);
  const failed = await submit(page, "fail provider fixture");
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  await page.goto("/?view=compare&left=" + original + "&right=" + failed);
  await expect(page.getByText("对比只支持已完成 Run", { exact: false })).toBeVisible();
  await expect(page.locator(".live-compare-table")).toHaveCount(0);
  await page.goto("/?view=compare&left=" + original + "&right=" + "f".repeat(32));
  await expect(page.getByText("读取服务端历史记录失败", { exact: false })).toBeVisible();
  await expect(page.locator(".live-compare-table")).toHaveCount(0);
});

test("anonymous comparison deep link never leaks run data before login", async ({ page }) => {
  await page.goto("/?view=compare&left=" + "a".repeat(32) + "&right=" + "b".repeat(32));
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "两次研究记录对比" })).toHaveCount(0);
});


test("overview is authenticated, URL-addressable and opens the same completed Run report", async ({ page }) => {
  await page.goto("/?view=overview");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "研究任务总览" })).toHaveCount(0);
  await signIn(page);
  const id = await submit(page, "technical snapshot fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await page.getByRole("button", { name: "研究概览", exact: true }).click();
  await expect(page).toHaveURL("/?view=overview");
  const overview = page.getByRole("region", { name: "研究任务概览" });
  await expect(overview.getByRole("heading", { name: "研究任务总览" })).toBeVisible();
  await expect(overview.getByText("样本最多 100 条", { exact: false })).toBeVisible();
  await expect(overview.getByRole("img", { name: "最近七个 UTC 日", exact: false })).toBeVisible();
  const list = overview.getByRole("region", { name: "近期研究任务列表" });
  await list.getByRole("textbox", { name: "筛选股票或任务 ID" }).fill(id);
  await expect(list.locator(".run-overview-row")).toHaveCount(1);
  await expect(list.locator(".run-overview-row")).toContainText("查看报告");
  await list.locator(".run-overview-row").click();
  await expect(page).toHaveURL("/?run=" + id);
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL("/?view=overview");
  await expect(overview.getByRole("heading", { name: "研究任务总览" })).toBeVisible();
  await page.reload();
  await expect(overview.getByRole("heading", { name: "研究任务总览" })).toBeVisible();
  await overview.getByRole("button", { name: "对比研究" }).click();
  await expect(page).toHaveURL("/?view=compare");
});

test("overview active and failed filters open live progress or exact failure, with no synthetic reports", async ({ page }) => {
  await signIn(page);
  const failed = await submit(page, "fail provider fixture");
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  const held = await submit(page, "hold fixture");
  await expect(page.locator(".live-run-fields")).toContainText(held);
  await page.getByRole("button", { name: "研究概览", exact: true }).click();
  const overview = page.getByRole("region", { name: "研究任务概览" });
  const list = overview.getByRole("region", { name: "近期研究任务列表" });
  const filter = overview.getByRole("group", { name: "研究任务状态筛选" });
  await filter.getByRole("button", { name: "执行中" }).click();
  await list.getByRole("textbox", { name: "筛选股票或任务 ID" }).fill(held);
  await expect(list.locator(".run-overview-row")).toHaveCount(1);
  await expect(list.locator(".run-overview-row")).toContainText("查看进度");
  await list.locator(".run-overview-row").click();
  await expect(page).toHaveURL("/?run=" + held);
  await expect(page.getByRole("region", { name: "Agent 执行轨迹" })).toBeVisible();
  await page.getByRole("button", { name: "请求取消" }).click();
  await expect(page.locator(".live-run-fields")).toContainText("已取消", { timeout: 20_000 });
  await page.getByRole("button", { name: "研究概览", exact: true }).click();
  await filter.getByRole("button", { name: "失败" }).click();
  await list.getByRole("textbox", { name: "筛选股票或任务 ID" }).fill(failed);
  await expect(list.locator(".run-overview-row")).toHaveCount(1);
  await expect(list.locator(".run-overview-row")).toContainText("排查失败");
  await list.locator(".run-overview-row").click();
  await expect(page).toHaveURL("/?run=" + failed);
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED");
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "再次研究该标的" })).toBeVisible();
});

test("overview with a list of no records shows an empty state, not fabricated KPIs or report rows", async ({ page }) => {
  await page.route("**/api/workbench/runs", async route => {
    if (route.request().method() === "GET") await route.fulfill({ status:200, contentType:"application/json", body:JSON.stringify({runs:[]}) });
    else await route.continue();
  });
  await page.goto("/?view=overview");
  await page.getByLabel("工作台访问口令").fill(password);
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  const overview = page.getByRole("region", { name: "研究任务概览" });
  await expect(overview.getByRole("heading", { name: "研究任务总览" })).toBeVisible();
  await expect(overview.getByText("暂无已保存的研究任务")).toBeVisible();
  await expect(overview.locator(".run-overview-row")).toHaveCount(0);
  await overview.getByRole("button", { name: "发起第一项研究" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
  await expect(page.getByLabel("股票代码")).toBeVisible();
});


test("completed research defaults to a readable brief with expandable source chapters", async ({ page }) => {
  await signIn(page);
  const id = await submit(page, "technical snapshot fixture");
  await expect(page.locator(".live-run-fields")).toContainText("已完成", {timeout:25000});
  const overview = page.getByRole("region",{name:"研究报告概览"});
  await expect(overview.getByText("研究结论 · 源自 Go 工件")).toBeVisible();
  await expect(overview.getByRole("region",{name:"研究关键依据"})).toBeVisible();
  await expect(overview.getByRole("region",{name:"研究风险与不确定性"})).toBeVisible();
  await expect(page.getByRole("heading",{name:"完整研究章节"})).toBeVisible();
  const manager = page.locator("#chapter-manager");
  await expect(manager).not.toHaveAttribute("open");
  await manager.locator("summary").click();
  await expect(manager).toHaveAttribute("open");
  await expect(page.locator("#report-manager")).toBeVisible();
  await expect(page.locator(".live-report-source-details")).not.toHaveAttribute("open");
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(id);
  await expect(overview.getByText("研究结论 · 源自 Go 工件")).toBeVisible();
});

test("research history filters loaded runs without hiding source data or inventing pagination", async ({ page }) => {
  await signIn(page);
  const id = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  await page.getByRole("button", { name: "研究历史" }).click();
  const rows = page.locator(".live-history-row");
  await expect(rows.filter({ hasText: id.slice(0, 12) })).toBeVisible();
  await page.getByLabel("按股票代码筛选").fill("000000");
  await expect(rows).toHaveCount(0);
  await expect(page.getByText("已加载记录中没有符合筛选条件的任务", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选" }).click();
  await expect(rows.filter({ hasText: id.slice(0, 12) })).toBeVisible();
  await page.getByLabel("按任务状态筛选").selectOption("FAILED");
  await expect(rows.filter({ hasText: id.slice(0, 12) })).toHaveCount(0);
  await page.getByLabel("按任务状态筛选").selectOption("COMPLETED");
  await expect(rows.filter({ hasText: id.slice(0, 12) })).toBeVisible();
  await expect(page.locator(".live-history")).toContainText("已加载");
});

test("research history displays the persisted failure code for the correct run", async ({ page }) => {
  await signIn(page);
  const failedId = await submit(page, "fail provider fixture");
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  await page.getByRole("button", { name: "研究历史" }).click();
  const failedRow = page.locator(".live-history-row").filter({ hasText: failedId.slice(0, 12) });
  await expect(failedRow).toContainText("失败");
  await expect(failedRow.locator(".live-history-failure")).toContainText("RESEARCH_FAILED");
  await failedRow.click();
  await expect(page.locator(".live-run-fields")).toContainText(failedId);
});
