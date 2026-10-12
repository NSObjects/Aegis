import { expect, test } from "@playwright/test";

// HOSTED_REAL_PROVIDER_READ_ONLY: the Go CLI check already paid for exactly
// one research Run. Do not click Start or Rerun here.
test("completed hosted Go/Eino research remains readable in the actual Web workbench", async ({ page }) => {
  const session = await page.request.get("/api/workbench/session");
  expect(session.status()).toBe(200);
  expect(await session.json()).toMatchObject({ configured: true, authenticated: true, local_dev: true });

  const list = await page.request.get("/api/workbench/runs");
  expect(list.status()).toBe(200);
  const data = await list.json() as { runs: Array<{run_id:string; symbol:string; state:string}> };
  const matching = data.runs.filter(r => r.symbol === "600519.SH" && r.state === "COMPLETED");
  expect(matching).toHaveLength(1);
  const runId = matching[0].run_id;
  expect(runId).toMatch(/^[a-f0-9]{32}$/);

  const artifactResponse = await page.request.get("/api/workbench/runs/" + runId + "/artifact");
  expect(artifactResponse.status()).toBe(200);
  const artifact = await artifactResponse.json();
  expect(artifact).toHaveProperty("research_artifact");
  expect(artifact).toHaveProperty("portfolio_review");

  await page.goto("/?run=" + runId);
  const details = page.locator(".live-run-fields");
  await expect(details).toContainText(runId, { timeout: 30000 });
  await expect(details).toContainText("600519.SH");
  await expect(details).toContainText("已完成");
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeEnabled();
  await expect(page.getByRole("button", { name: "导出真实 JSON" })).toBeEnabled();
  await page.reload();
  await expect(details).toContainText(runId);
  await expect(page.getByRole("button", { name: "导出 Markdown" })).toBeEnabled();

  // Reuse this exact completed and already-paid Run for narrow-screen reading checks.
  await page.setViewportSize({ width: 375, height: 812 });
  await expect(page.getByRole("heading", { name: "结构化研究报告" })).toBeVisible();
  await expect(page.locator(".live-brief-lead")).toBeVisible();
  await expect(page.locator(".live-brief-appendix")).toBeVisible();
  await expect(page.getByRole("navigation", { name: "研究报告章节目录" })).toBeVisible();
  const horizontalOverflow = await page.evaluate(() =>
    document.documentElement.scrollWidth > document.documentElement.clientWidth + 1);
  expect(horizontalOverflow, "375px hosted research page must not scroll horizontally").toBe(false);
});
