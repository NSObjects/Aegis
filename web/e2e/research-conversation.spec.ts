import { expect, test } from "@playwright/test";

test("authenticated Web conversation displays fixture response without exposing backend credentials", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await expect(panel).toBeVisible();
  await panel.getByLabel("询问已有研究").fill("列出已保存的研究");
  const reply = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  expect((await reply).status()).toBe(200);
  await expect(panel).toContainText("run_fixture_001");
  await expect(panel).toContainText("列出已保存的研究");
  await expect(panel.getByLabel("询问已有研究")).toHaveValue("");
  await expect(panel).not.toContainText("ci-go-api-fixture-only-secret");
});

test("conversation Next.js proxy rejects unauthenticated browser requests", async ({ request }) => {
  const response = await request.post("/api/workbench/conversation", {
    data: { message: "查看研究" },
    headers: { origin: "http://localhost:3100" },
  });
  expect(response.status()).toBe(401);
  expect(await response.text()).not.toContain("ci-go-api-fixture-only-secret");
});

test("research start needs explicit ticker confirmation and forwards scoped consent", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await expect(panel).toBeVisible();
  await panel.getByLabel("询问已有研究").fill("请研究 600519.SH");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("button", { name: "发送问题" }).click();
  await expect(panel.getByRole("alert")).toContainText("明确确认");
  const check = panel.getByRole("checkbox", { name: /我确认允许启动/ });
  await check.check();
  const responsePromise = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(200);
  expect(response.request().postDataJSON()).toEqual({ message: "请研究 600519.SH", start_symbol: "600519.SH", confirm_start: true });
  await expect(panel).toContainText("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
  await expect(check).toHaveCount(0);
});

test("conversation proxy rejects unconfirmed research action after authentication", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const response = await page.evaluate(async () => {
    const r = await fetch("/api/workbench/conversation", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: "研究一下", start_symbol: "600519.SH", confirm_start: false }) });
    return r.status;
  });
  expect(response).toBe(400);
});

test("confirmed research session is recovered after a browser reload", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("启动600519.SH研究");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  const post = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  expect((await post).status()).toBe(200);
  await expect(panel).toContainText("当前研究 · Run ID: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
  await page.reload();
  await page.locator("#research-follow-up > summary").click();
  const restored = page.getByRole("region", { name: "研究对话" });
  await expect(restored).toContainText("已恢复研究 600519.SH · Run ID: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
});

test("follow-up sends restored session reference without authorizing a new research", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("研究600519.SH");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  const first = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  expect((await first).status()).toBe(200);
  await page.reload();
  await page.locator("#research-follow-up > summary").click();
  await expect(page.getByRole("region", { name: "研究对话" })).toContainText("已恢复研究 600519.SH");
  const followup = page.getByRole("region", { name: "研究对话" });
  await followup.getByLabel("询问已有研究").fill("刚才那次研究有什么空头依据？");
  const second = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
  await followup.getByRole("button", { name: "发送问题" }).click();
  const result = await second;
  expect(result.status()).toBe(200);
  expect(result.request().postDataJSON()).toEqual({ message: "刚才那次研究有什么空头依据？", session_id: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" });
});

test("second confirmed research reuses the restored conversation session", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  for (let i=0;i<2;i++) {
    await panel.getByLabel("询问已有研究").fill(i===0 ? "第一次研究" : "第二次研究");
    await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
    await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
    const pending = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
    await panel.getByRole("button", { name: "发送问题" }).click();
    const response = await pending;
    expect(response.status()).toBe(200);
    const sent = response.request().postDataJSON();
    expect(sent.confirm_start).toBe(true);
    if (i===0) expect(sent.session_id).toBeUndefined();
    else expect(sent.session_id).toBe("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
  }
});

test("binding failure preserves created Run ID and warns against duplicate research", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  // Establish a real fixture-backed session first: binding failure must
  // not leave a stale session attached to the newly created Run.
  const existing = page.getByRole("region", { name: "研究对话" });
  await existing.getByLabel("询问已有研究").fill("第一次研究");
  await existing.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await existing.getByLabel("A 股代码").fill("600519.SH");
  await existing.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  await existing.getByRole("button", { name: "发送问题" }).click();
  await expect(existing).toContainText("当前研究 · Run ID");
  await page.route("**/api/workbench/conversation", async route => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      answer: "研究任务已创建，但会话关联失败。请保存 Run ID；不要重复启动研究。",
      run_id: "c".repeat(32), symbol: "600519.SH", state: "PENDING", session_binding: "FAILED",
    }) });
  });
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("请研究 600519.SH");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  await panel.getByRole("button", { name: "发送问题" }).click();
  await expect(panel).toContainText("c".repeat(32));
  await expect(panel.getByRole("alert")).toContainText("勿重复启动");
  await expect(panel).not.toContainText("当前研究 · Run ID");
  expect(await page.evaluate(() => sessionStorage.getItem("aegis-conversation-session"))).toBeNull();
  await panel.getByLabel("询问已有研究").fill("刚才的研究依据是什么？");
  const followup = page.waitForRequest(r => r.url().endsWith("/api/workbench/conversation") && r.method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  expect((await followup).postDataJSON()).toEqual({ message: "刚才的研究依据是什么？" });
});

test("conversation Run opens the corresponding workspace deep link", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("研究 600519.SH");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  await panel.getByRole("button", { name: "发送问题" }).click();
  await expect(panel.getByRole("button", { name: "在研究工作台打开此 Run" })).toBeVisible();
  await panel.getByRole("button", { name: "在研究工作台打开此 Run" }).click();
  await expect(page).toHaveURL(/\\?run=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/);
});

test("selected report follow-up is explicitly read-only and references exactly that Run", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("研究 600519.SH");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  await panel.getByRole("button", { name: "发送问题" }).click();
  await panel.getByRole("button", { name: "在研究工作台打开此 Run" }).click();
  const selected = panel.getByRole("checkbox", { name: /针对当前研究 Run/ });
  await expect(selected).toBeVisible();
  await selected.check();
  await panel.getByLabel("询问已有研究").fill("有哪些反对依据？");
  const pending = page.waitForRequest(r => r.url().endsWith("/api/workbench/conversation") && r.method() === "POST" && r.postDataJSON()?.message?.includes("反对依据"));
  await panel.getByRole("button", { name: "发送问题" }).click();
  const data = (await pending).postDataJSON();
  expect(data.session_id).toBeUndefined(); // Explicit Run must not inherit an unrelated session.
  expect(data.confirm_start).toBeUndefined();
  expect(data.start_symbol).toBeUndefined();
  expect(data.message).toContain("Run ID aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
  expect(data.message).toContain("不要启动新的研究");
});

test("successful research without a session ID clears previous binding but retains Run navigation", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("第一次研究");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  await panel.getByRole("button", { name: "发送问题" }).click();
  await expect(panel).toContainText("当前研究 · Run ID");
  await page.route("**/api/workbench/conversation", async route => {
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      answer: "研究任务已创建", run_id: "c".repeat(32), symbol: "600519.SH", state: "PENDING"
    }) });
  });
  await panel.getByLabel("询问已有研究").fill("第二次研究");
  await panel.locator("details.conversation-start-research").evaluate((el: HTMLDetailsElement) => { el.open = true; });
  await panel.getByLabel("A 股代码").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  await panel.getByRole("button", { name: "发送问题" }).click();
  await expect(panel.getByRole("alert")).toContainText("未返回有效会话关联");
  await expect(panel.getByRole("button", { name: "在研究工作台打开此 Run" })).toBeVisible();
  expect(await page.evaluate(() => sessionStorage.getItem("aegis-conversation-session"))).toBeNull();
  await panel.getByLabel("询问已有研究").fill("这次研究的依据是什么？");
  const followup = page.waitForRequest(r => r.url().endsWith("/api/workbench/conversation") && r.method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  expect((await followup).postDataJSON()).toEqual({ message: "这次研究的依据是什么？" });
});

test("expired authentication rejects read-only follow-up without sending research consent", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await page.route("**/api/workbench/conversation", async route => {
    await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "unauthorized" }) });
  });
  await panel.getByLabel("询问已有研究").fill("请回看已有研究的依据");
  const request = page.waitForRequest(r => r.url().endsWith("/api/workbench/conversation") && r.method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  const body = (await request).postDataJSON();
  expect(body.message).toContain("请只读查询已有 Run ID ");
  expect(body.message).toContain("请回看已有研究的依据");
  expect(body.message).toContain("不要启动新的研究。");
  expect(body.start_symbol).toBeUndefined();
  expect(body.confirm_start).toBeUndefined();
  await expect(panel.getByRole("alert")).toContainText("登录已过期");
  await expect(panel).not.toContainText("Aegis：");
});

test("conversation service failure never invents a reply or grants research consent", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await page.locator("#research-follow-up > summary").click();
  const panel = page.getByRole("region", { name: "研究对话" });
  await page.route("**/api/workbench/conversation", async route => {
    await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "service unavailable" }) });
  });
  await panel.getByLabel("询问已有研究").fill("对比前两次已保存的研究");
  const pending = page.waitForRequest(r => r.url().endsWith("/api/workbench/conversation") && r.method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  const body = (await pending).postDataJSON();
  expect(body.message).toContain("请只读查询已有 Run ID ");
  expect(body.message).toContain("对比前两次已保存的研究");
  expect(body.message).toContain("不要启动新的研究。");
  expect(body.start_symbol).toBeUndefined();
  expect(body.confirm_start).toBeUndefined();
  await expect(panel.getByRole("alert")).toContainText("对话服务暂不可用");
  await expect(panel.getByLabel("询问已有研究")).toHaveValue("对比前两次已保存的研究");
  await expect(panel).not.toContainText("Aegis：");
  await expect(panel.getByRole("button", { name: "发送问题" })).toBeEnabled();
});
