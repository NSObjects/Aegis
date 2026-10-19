import { expect, test } from "@playwright/test";

test("authenticated Web conversation displays fixture response without exposing backend credentials", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
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
  const panel = page.getByRole("region", { name: "研究对话" });
  await expect(panel).toBeVisible();
  await panel.getByLabel("询问已有研究").fill("请研究 600519.SH");
  await panel.getByLabel("启动新研究（可选，A 股代码）").fill("600519.SH");
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
  const panel = page.getByRole("region", { name: "研究对话" });
  await panel.getByLabel("询问已有研究").fill("启动600519.SH研究");
  await panel.getByLabel("启动新研究（可选，A 股代码）").fill("600519.SH");
  await panel.getByRole("checkbox", { name: /我确认允许启动/ }).check();
  const post = page.waitForResponse(r => r.url().endsWith("/api/workbench/conversation") && r.request().method() === "POST");
  await panel.getByRole("button", { name: "发送问题" }).click();
  expect((await post).status()).toBe(200);
  await expect(panel).toContainText("当前研究 · Run ID: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
  await page.reload();
  const restored = page.getByRole("region", { name: "研究对话" });
  await expect(restored).toContainText("已恢复研究 600519.SH · Run ID: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
});
