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
