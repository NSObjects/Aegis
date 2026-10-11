import { describe, expect, it } from "vitest";
import { readLiveReport } from "./live-report";

function card(report: ReturnType<typeof readLiveReport>, id: string) {
  const found = report.cards.find(c => c.id === id);
  expect(found, "missing report card: " + id).toBeDefined();
  return found!;
}

describe("Live Go typed artifact report projection", () => {
  const full = {
    research_artifact: {
      symbol: "600519.SH",
      synthesis: {
        summary: "市场与标的证据综合（后端文本）",
        agreements: [{ claim: "市场偏谨慎", references: [{ domain: "MARKET" }] }],
        conflicts: [{ description: "事件驱动与技术趋势矛盾", references: [{ domain: "EVENT", event_id: "E-1" }] }],
        risks: ["有效性需复核"],
      },
      market: {
        as_of: "20261009", regime: "MIXED", advance_count: 0, decline_count: 4221,
        turnover: 10000000, evidence: ["市场研究输入"], risk: ["波动较高"],
      },
      technical: {
        symbol: "600519.SH", requested_as_of: "20261011", effective_as_of: "20261009",
        last_trade_date: "20261009", status: "NORMAL",
        snapshot: { open: 1400, close: 1432.5, change_percent: 0, volume: 10000 },
        moving_averages: { ma5: 1420, ma10: null, ma20: 1411 },
        momentum: { rsi14: 54, macd: { dif: 2, dea: 1, hist: 1 } },
      },
      event: {
        requested_as_of: "2026-10-11T02:00:00Z", evidence_as_of: "2026-10-11T02:05:00Z",
        status: "NORMAL", events: [{
          id: "E-1", type: "DISCLOSURE", headline: "已归档公告", fact_summary: "示例公告事实",
          source_count: 2, evidence_document_ids: ["DOC-1"],
        }],
      },
      fundamental: {
        symbol: "600519.SH", requested_as_of: "20261011", interpretation: "根据已公告报表的模型解释",
        income: { period_end: "20260630", announcement_date: "20260831", source: "TUSHARE_INCOME",
          revenue: 0, net_income: null },
      },
    },
    portfolio_review: {
      research_plan: {
        conclusion: "BALANCED", summary: "后端经理的结论",
        opportunities: [{ claim: "潜在机会", references: [{ domain: "EVENT", event_id: "E-1" }] }],
        risks: [{ claim: "潜在风险", references: [{ domain: "MARKET" }] }],
        unresolved_argument_ids: ["a1"],
      },
      debate_state: { turns: [
        { round: 1, side: "BULL", thesis: "多头论点", arguments: [{
          id: "a1", claim: "信息支撑看多", references: [{ domain: "EVENT", event_id: "E-1" }],
        }] },
        { round: 2, side: "BEAR", thesis: "空头论点", rebuttals: [{
          target_argument_id: "a1", claim: "质疑信息充分性", references: [{ domain: "MARKET" }],
        }] },
      ] },
      evidence_ledger: { conclusion: "BALANCED", arguments: [{
        id: "a1", side: "BULL", round: 1, claim: "信息支撑看多",
        disposition: "UNRESOLVED", references: [{ domain: "EVENT", event_id: "E-1" }],
        challenges: [{ claim: "质疑信息充分性", references: [{ domain: "MARKET" }] }],
      }], insights: [{ kind: "RISK", claim: "需更多证据", argument_ids: ["a1"], references: [{ domain: "EVENT" }] }] },
      trade_intent: { status: "MONITOR", thesis: "只观察，不执行", evidence: ["证据"], risk: ["风险"] },
      risk_context: { assessment: "INSUFFICIENT_CONTEXT", summary: "账户数据不足",
        unresolved: [{ claim: "无法计算持仓风险" }] },
      portfolio_decision: { disposition: "REVIEW_REQUIRED", reason_code: "ACCOUNT_DATA_ABSENT",
        portfolio_state: "NOT_PROVIDED", risk_assessment: "INSUFFICIENT_CONTEXT", summary: "尚不能评估" },
    },
    final_risk_gate: { disposition: "REVIEW_REQUIRED", reason: "PORTFOLIO_DECISION" },
  };

  it("shows exact nested Go contract fields and explicit observation cutoffs", () => {
    const report = readLiveReport(full);
    expect(report.mode).toBe("PORTFOLIO_REVIEW");
    expect(report.headline).toBe("后端经理的结论");
    expect(report.overview).toContainEqual({ label: "账户状态", value: "NOT_PROVIDED · 未知" });
    expect(report.overview).toContainEqual({ label: "最终风险门禁", value: "REVIEW_REQUIRED" });
    expect(card(report, "market").fields).toContainEqual({ label: "上涨家数", value: "0" });
    expect(card(report, "technical").fields).toContainEqual({ label: "涨跌幅（%）", value: "0" });
    expect(card(report, "technical").fields).toContainEqual({ label: "有效交易日", value: "20261009" });
    expect(card(report, "event").fields).toContainEqual({ label: "证据观察截止", value: "2026-10-11T02:05:00Z" });
    expect(card(report, "event").entries[0].details).toContain("文档 ID · DOC-1");
    expect(card(report, "fundamental").fields).toContainEqual({ label: "营业收入（人民币元）", value: "0" });
    expect(card(report, "fundamental").fields.find(x => x.label.startsWith("净利润"))).toBeUndefined();
  });

  it("reads debate and ledger references literally, preserving unresolved research arguments", () => {
    const report = readLiveReport(full);
    const debate = card(report, "debate");
    expect(debate.entries.some(x => x.label.includes("反驳 a1") && x.body === "质疑信息充分性")).toBe(true);
    const ledger = card(report, "ledger");
    expect(ledger.entries[0].details).toContain("经理分类 · UNRESOLVED");
    expect(ledger.entries[0].details).toContain("EVENT · 事件 E-1");
    expect(ledger.entries[1].details).toContain("MARKET · 领域引用（无具体事件 ID）");
    const plan = card(report, "manager");
    expect(plan.entries.some(x => x.label === "尚未解决的论点 ID" && x.body === "a1")).toBe(true);
  });

  it("never fabricates missing amounts, agent panels, market quotes, or an empty account", () => {
    const report = readLiveReport({
      research_artifact: { symbol: "000001.SZ", synthesis: { summary: "仅综合结果" },
        technical: { status: "NO_DATA", moving_averages: { ma5: null } },
        event: { status: "NO_EVENT" } },
      portfolio_review: { portfolio_decision: { portfolio_state: "NOT_PROVIDED", disposition: "NO_ACTION" } },
    });
    expect(report.cards.map(x => x.id)).not.toContain("market");
    expect(report.cards.map(x => x.id)).not.toContain("fundamental");
    expect(card(report, "technical").fields).toEqual([{ label: "数据状态", value: "NO_DATA" }]);
    expect(card(report, "event").fields).toEqual([{ label: "事件状态", value: "NO_EVENT" }]);
    expect(card(report, "portfolio").fields).toContainEqual({
      label: "账户状态", value: "NOT_PROVIDED · 未知（不能解释为零持仓）",
    });
    expect(card(report, "portfolio").fields.some(x => /现金|仓位/.test(x.label))).toBe(false);
  });

  it("renders Decision mode only with the original decision fields", () => {
    const report = readLiveReport({
      symbol: "600519.SH", action: "WATCH", market_view: "尚无充分信号",
      evidence: ["市场弱势"], risk: ["数据不充分"], confidence: 0,
    });
    expect(report.mode).toBe("DECISION");
    expect(report.headline).toBe("尚无充分信号");
    expect(report.cards.map(x => x.id)).toEqual(["decision"]);
    expect(card(report, "decision").entries.some(x => x.body === "数据不充分")).toBe(true);
    expect(report.overview).toContainEqual({ label: "研究动作（非委托）", value: "WATCH" });
  });

  it("fails closed for unknown artifacts rather than displaying stale report values", () => {
    expect(readLiveReport(null)).toEqual({ mode: "UNKNOWN", headline: null, overview: [], cards: [] });
    expect(readLiveReport({})).toEqual({ mode: "UNKNOWN", headline: null, overview: [], cards: [] });
  });
});
