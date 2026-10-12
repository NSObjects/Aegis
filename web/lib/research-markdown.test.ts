import { describe, expect, it } from "vitest";
import { renderResearchMarkdown } from "./research-markdown";

const run = { run_id: "a".repeat(32), symbol: "600519.SH", requested_as_of: "2026-10-12T00:00:00Z", mode: "PORTFOLIO_REVIEW" };
describe("Markdown report export", () => {
  it("exports only typed fields and preserves evidence IDs and unknown account semantics", () => {
    const result = renderResearchMarkdown({
      research_artifact: { symbol: "600519.SH", synthesis: { summary: "研究观点", agreements: [{ claim: "事件支持", references: [{ domain: "EVENT", event_id: "E-42" }] }] } },
      portfolio_review: { research_plan: { conclusion: "BALANCED", summary: "经理判断" }, portfolio_decision: { portfolio_state: "NOT_PROVIDED" } },
      request_snapshot: { market_query: "secret-input-not-for-export" },
    }, run);
    expect(result).toContain("600519.SH");
    expect(result).toContain("2026-10-12T00:00:00Z");
    expect(result).toContain("E-42");
    expect(result).toContain(String.raw`NOT\_PROVIDED`);
    expect(result).not.toContain("secret-input-not-for-export");
    expect(result).toContain("不构成交易指令");
  });
  it("does not invent missing values and preserves actual zero", () => {
    const result = renderResearchMarkdown({ research_artifact: { market: { advance_count: 0 } } }, run);
    expect(result).toContain("| 上涨家数 | 0 |");
    expect(result).toContain("未提供");
  });
  it("exports decision mode without portfolio hallucinations", () => {
    const result = renderResearchMarkdown({ symbol: "600519.SH", action: "WATCH", market_view: "谨慎观察" }, { ...run, mode: "DECISION" });
    expect(result).toContain("谨慎观察");
    expect(result).not.toContain("组合决策");
  });
  it("keeps model-provided Markdown syntax as plain report content", () => {
    const result = renderResearchMarkdown({
      symbol: "600519.SH",
      market_view: "## 伪造的报告章节 [外部链接](https://example.invalid) **保证收益**",
      action: "WATCH",
      evidence: ["ordinary evidence"],
    }, { ...run, mode: "DECISION" });
    expect(result).toContain("# Aegis 研究报告");
    expect(result).toContain("## 研究结论");
    expect(result).not.toContain("\n## 伪造的报告章节");
    expect(result).not.toContain("[外部链接](https://example.invalid)");
    expect(result).not.toContain("**保证收益**");
    expect(result).toContain("\\#\\# 伪造的报告章节");
  });

});
