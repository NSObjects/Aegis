import { test, expect, type Page } from "@playwright/test";

const fakeArtifact = {
  research_artifact: {
    symbol: "600519.SH",
    synthesis: { summary: "PROTOCOL_FIXTURE_ONLY evidence navigation sample", agreements: [], conflicts: [], risks: [] },
    event: { symbol: "600519.SH", status: "NORMAL", requested_as_of: "2026-10-11T00:00:00Z",
      events: [
        { id: "evt-fixture-1", headline: "示例公告", fact_summary: "仅供浏览器验证的模拟公告内容",
          type: "DISCLOSURE", occurred_at: "2026-10-10T11:00:00Z",
          source_count: 1, evidence_document_ids: ["doc-fixture-1"] },
        { id: "evt-fixture-2", headline: "无文档事件", fact_summary: "文档标识缺失", source_count: 0, evidence_document_ids: [] },
      ],
    },
  },
  portfolio_review: {
    trade_intent: { status: "MONITOR", evidence: ["doc-not-a-structured-link"] },
    research_plan: { summary: "PROTOCOL_FIXTURE_ONLY", conclusion: "BALANCED" },
    evidence_ledger: {
      conclusion: "BALANCED",
      arguments: [{
        id: "bull-001", side: "BULL", round: 1, claim: "后端声明的多头示例",
        disposition: "UNRESOLVED",
        references: [{ domain: "EVENT", event_id: "evt-fixture-1" }, { domain: "MARKET" }, { domain: "EVENT", event_id: "not-found" }],
        challenges: [{ side: "BEAR", round: 2, claim: "针对 bull-001 的反驳",
          references: [{ domain: "EVENT", event_id: "evt-fixture-2" }] }],
      }],
      insights: [{ kind: "RISK", claim: "风险需要复核", argument_ids: ["bull-001"],
        references: [{ domain: "EVENT", event_id: "evt-fixture-2" }] }],
    },
    portfolio_decision: { portfolio_state: "NOT_PROVIDED", disposition: "REVIEW_REQUIRED" },
  },
  final_risk_gate: { disposition: "REVIEW_REQUIRED", reason: "FIXTURE_NO_REAL_ACCOUNT" },
};

async function signIn(page: Page) {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "AEGIS · 研究工作台" })).toBeVisible();
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "实时研究工作台" })).toBeVisible();
}

async function submit(page: Page) {
  await page.getByLabel("股票代码").fill("600519.SH");
  await page.getByRole("button", { name: "启动真实研究" }).click();
  await expect(page.getByText("研究任务已提交至 Go 服务", { exact: false })).toBeVisible();
  return page.locator(".live-run-fields code").innerText();
}

test("EvidenceLedger links only explicit event IDs and lets a reader inspect document IDs", async ({ page }) => {
  // Controlled synthetic artifact: exercises Browser->BFF report reader only,
  // not a market provider, model, or an external document service.
  await page.route("**/api/workbench/runs/*/artifact", route => route.fulfill({
    status: 200, contentType: "application/json", body: JSON.stringify(fakeArtifact),
  }));
  await signIn(page);
  const first = await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("已完成", { timeout: 25_000 });
  const nav = page.getByRole("region", { name: "交互式证据与论点导航" });
  await expect(nav).toBeVisible();
  await expect(nav.getByText("后端声明的多头示例").first()).toBeVisible();
  await expect(nav.getByText("UNRESOLVED").first()).toBeVisible();
  await expect(nav.getByText("事件 ID 未在本次工件中找到")).toBeVisible();
  await expect(nav.getByText("仅有领域引用，没有具体事件 ID")).toBeVisible();
  await expect(nav.getByText("doc-not-a-structured-link")).toHaveCount(0);
  await nav.getByRole("button", { name: "查看事件 evt-fixture-1" }).click();
  const details = nav.getByRole("region", { name: "已匹配的事件详情" });
  await expect(details).toContainText("示例公告");
  await expect(details).toContainText("doc-fixture-1");
  await expect(details).toContainText("匹配到 ID 不代表已经验证原文内容");

  await nav.getByRole("button", { name: /风险需要复核/ }).click();
  await expect(details).toHaveCount(0);
  await expect(nav.getByRole("button", { name: "打开 bull-001" })).toBeVisible();
  await nav.getByRole("button", { name: "查看事件 evt-fixture-2" }).click();
  await expect(nav.getByRole("region", { name: "已匹配的事件详情" })).toContainText("本次 Artifact 未携带文档 ID");

  await nav.getByRole("button", { name: "打开 bull-001" }).click();
  await expect(nav.getByText("针对 bull-001 的反驳")).toBeVisible();
  await page.reload();
  await expect(page.locator(".live-run-fields")).toContainText(first);
  const restored = page.getByRole("region", { name: "交互式证据与论点导航" });
  await expect(restored.getByText("后端声明的多头示例").first()).toBeVisible();
  await expect(restored.getByRole("region", { name: "已匹配的事件详情" })).toHaveCount(0);
});

test("failed work never shows stale linked evidence", async ({ page }) => {
  await page.route("**/api/workbench/runs/*/artifact", route => route.fulfill({
    status: 200, contentType: "application/json", body: JSON.stringify(fakeArtifact),
  }));
  await signIn(page);
  await page.getByLabel("研究关注点（可选）").fill("fail provider fixture");
  await submit(page);
  await expect(page.locator(".live-run-fields")).toContainText("RESEARCH_FAILED", { timeout: 25_000 });
  await expect(page.getByRole("region", { name: "交互式证据与论点导航" })).toHaveCount(0);
});
