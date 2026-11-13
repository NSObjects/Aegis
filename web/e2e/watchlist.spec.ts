import { expect, test, type Page } from "@playwright/test";

const password = "ci-browser-access-password";

async function signIn(page: Page) {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await page.getByLabel("工作台访问口令").fill(password);
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "工作台", exact: true })).toBeVisible();
}

async function openWatchlist(page: Page) {
  await page.getByRole("button", { name: "观察池" }).click();
  await expect(page.getByRole("heading", { name: "观察池", level: 1 })).toBeVisible();
}

// The add form collapses once any entry exists; open it explicitly like the
// existing workbench specs do instead of changing production defaults.
async function openAddForm(page: Page) {
  await page.locator("details.live-create").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
}

// Mutations through the Next proxy require an explicit same-origin header.
async function clearWatch(page: Page, symbol: string) {
  await page.request.delete(`/api/workbench/watchlist/${symbol}`, { headers: { Origin: new URL(page.url()).origin } });
}

async function revealRunDiagnostics(page: Page) {
  await page.locator("details.live-secondary-panel").first().evaluate((el: HTMLDetailsElement) => { el.open = true; });
}

test("watchlist add, latest-research link, one-click research, and remove", async ({ page }) => {
  await signIn(page);
  await openWatchlist(page);
  // A previous spec file may have watched this symbol against the shared
  // fixture; clear it first so this test owns the entry.
  await clearWatch(page, "601398.SH");
  await openAddForm(page);

  await page.getByLabel("股票代码", { exact: true }).fill("601398.SH");
  await page.getByLabel("观察理由（必填）").fill("E2E 回调观察理由");
  await page.getByLabel("关注条件（可选）").fill("跌破 MA20 时复查");
  await page.getByRole("button", { name: "加入观察池" }).click();
  const row = page.locator(".watch-row", { hasText: "601398.SH" });
  await expect(row).toBeVisible();
  await expect(row).toContainText("E2E 回调观察理由");
  await expect(row).toContainText("条件：跌破 MA20 时复查");
  await expect(row).toContainText("尚无研究记录");

  // Duplicate adds surface the backend conflict instead of a second entry.
  await openAddForm(page);
  await page.getByLabel("股票代码", { exact: true }).fill("601398.SH");
  await page.getByLabel("观察理由（必填）").fill("重复理由");
  await page.getByRole("button", { name: "加入观察池" }).click();
  await expect(page.locator(".live-alert")).toContainText("已在观察池中");

  // One-click research goes through the existing run API and navigates away.
  await row.getByRole("button", { name: "启动研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  // The workspace is the default view: its URL carries the run id, not a view key.
  await expect(page).toHaveURL(/run=/);
  await revealRunDiagnostics(page);
  const runId = (await page.locator(".live-run-fields code").first().innerText()).trim();
  expect(runId).toMatch(/^[a-f0-9]{32}$/);

  // Back on the watchlist the entry links to that exact run, whatever its state.
  await openWatchlist(page);
  const linked = page.locator(".watch-row", { hasText: "601398.SH" });
  await expect(linked.locator(".watch-run-link").first()).not.toContainText("尚无研究记录");
  await linked.locator(".watch-run-link").first().click();
  await expect(page).toHaveURL(new RegExp(`run=${runId}`));
  await revealRunDiagnostics(page);
  await expect(page.locator(".live-run-fields code").first()).toHaveText(runId);

  await openWatchlist(page);
  await page.locator(".watch-row", { hasText: "601398.SH" }).getByRole("button", { name: "移除 601398.SH" }).click();
  await expect(page.locator(".watch-row", { hasText: "601398.SH" })).toHaveCount(0);
});

test("watchlist deep link, empty state, and invalid symbol rejected", async ({ page }) => {
  await signIn(page);
  await page.goto("/?view=watchlist");
  await expect(page.getByRole("heading", { name: "观察池", level: 1 })).toBeVisible();

  // Invalid symbols never reach the backend as entries.
  await openAddForm(page);
  await page.getByLabel("股票代码", { exact: true }).fill("600519");
  await page.getByLabel("观察理由（必填）").fill("无效代码理由");
  await page.getByRole("button", { name: "加入观察池" }).click();
  await expect(page.locator(".live-alert")).toContainText("股票代码必须为 6 位代码加");
  await expect(page.locator(".watch-row")).toHaveCount(0);

  await clearWatch(page, "300750.SZ");
  await openAddForm(page);
  await page.getByLabel("股票代码", { exact: true }).fill("300750.SZ");
  await page.getByLabel("观察理由（必填）").fill("空态验证");
  await page.getByRole("button", { name: "加入观察池" }).click();
  await expect(page.locator(".watch-row", { hasText: "300750.SZ" })).toBeVisible();
  await page.locator(".watch-row", { hasText: "300750.SZ" }).getByRole("button", { name: "移除 300750.SZ" }).click();
  await expect(page.locator(".watch-row")).toHaveCount(0);
  await expect(page.getByText("观察池为空。加入标的后可从这里直接启动研究。")).toBeVisible();
});
