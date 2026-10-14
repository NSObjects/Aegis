import { expect, test } from "@playwright/test";

// PROTOCOL_FIXTURE_ONLY: validates Run SSE/poll UI, never claims a real provider invocation.
test("observed TOOL events belong to the same Run and survive refresh", async ({ page }) => {
 await page.goto("/");
 await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
 await page.getByRole("button",{name:"解锁研究工作台"}).click();
 await expect(page.getByRole("heading",{name:"实时研究工作台"})).toBeVisible();
 await page.getByLabel("股票代码").fill("600519.SH");
 await page.getByLabel("研究关注点（可选）").fill("observed tool trace fixture");
 await page.getByRole("button",{name:"启动真实研究"}).click();
 await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
 await page.locator("details.live-trace-panel").evaluate((el: HTMLDetailsElement) => { el.open = true; });
 await expect(page.locator(".live-run-fields")).toContainText("已完成",{timeout:25000});
 const tools=page.locator(".live-trace-tools");
 await expect(tools).toContainText("snapshot_official");
 await expect(tools).toContainText("完成");
 await expect(page.locator(".live-trace-stage").filter({hasText:"Event"})).toContainText("成功");
 await page.reload();
 await page.locator("details.live-trace-panel").evaluate((el: HTMLDetailsElement) => { el.open = true; });
 await expect(tools).toContainText("snapshot_official");
 await expect(tools).toContainText("345ms");
});
