import { describe, expect, it } from "vitest";
import { readLiveReport } from "./live-report";
import { buildReadingDigest, chapterCaption, groups, readStatus } from "./report-reader";

describe("human-readable Live report projection (no regenerated claims)", () => {
  const raw = {
    research_artifact: {
      symbol:"600519.SH",
      synthesis: { summary:"模型综合的原始总结", agreements:[{ claim:"有稳定的领域线索", references:[{domain:"MARKET"}]}],
        conflicts:[{description:"趋势与新闻发生分歧",references:[]}], risks:["信息来源仍有限"] },
      market:{as_of:"20261009",regime:"MIXED",advance_count:0,decline_count:4012},
      event:{status:"NO_EVENT",events:[]},
    },
    portfolio_review: {
      research_plan:{conclusion:"INSUFFICIENT_EVIDENCE",summary:"经理原文：建议继续收集信息",
        opportunities:[{claim:"关注供需基本面",references:[]}],
        risks:[{claim:"市场流动性不足",references:[]}],
        unresolved_argument_ids:["arg-19"],trading_constraints:["限制假设"]},
      trade_intent:{status:"MONITOR",thesis:"尚在观察"},
      risk_context:{assessment:"INSUFFICIENT_CONTEXT", summary:"风控提示：账户事实缺失",
        unresolved:[{claim:"账户持仓未知"}]},
      portfolio_decision:{portfolio_state:"NOT_PROVIDED",disposition:"REVIEW_REQUIRED"},
    },
    final_risk_gate:{disposition:"REVIEW_REQUIRED",reason:"原始门禁未通过"},
  };
  it("places original manager conclusion before synthesis, without rewriting or inventing recommendations", () => {
    const d = buildReadingDigest(readLiveReport(raw));
    expect(d.headline).toBe("经理原文：建议继续收集信息");
    expect(d.supporting.map(p=>p.text)).toEqual(["关注供需基本面","有稳定的领域线索"]);
    expect(d.conflicting.map(p=>p.text)).toEqual(["趋势与新闻发生分歧","arg-19"]);
    expect(d.risks.map(p=>p.text)).toEqual(["市场流动性不足","限制假设","风控提示：账户事实缺失"]);
    expect(d.omittedRisks).toBeGreaterThan(0);
    expect(d.supporting[0]).toMatchObject({sectionId:"manager",label:"机会 1"});
    expect(d.status).toContainEqual({label:"研究结论",raw:"INSUFFICIENT_EVIDENCE",display:"证据不足 · INSUFFICIENT_EVIDENCE"});
    expect(d.status).toContainEqual({label:"账户状态",raw:"NOT_PROVIDED · 未知",display:"NOT_PROVIDED · 未知"});
  });

  it("preserves the exact server code when no verified translation exists", () => {
    expect(readStatus("MONITOR")).toBe("MONITOR");
    expect(readStatus("INSUFFICIENT_CONTEXT")).toBe("INSUFFICIENT_CONTEXT");
    expect(readStatus("NO_DATA")).toBe("未取得数据 · NO_DATA");
    expect(readStatus("MIXED")).toBe("信号分化 · MIXED");
  });

  it("distinguishes numeric zero from a missing field, not fabricating any risk items", () => {
    const report=readLiveReport({research_artifact:{symbol:"000001.SZ",
      synthesis:{summary:"唯一源文字"},market:{advance_count:0},technical:{status:"NO_DATA"}},
      portfolio_review:{portfolio_decision:{portfolio_state:"NOT_PROVIDED"}}});
    expect(report.cards.find(c=>c.id==="market")?.fields).toContainEqual({label:"上涨家数",value:"0"});
    const d=buildReadingDigest(report);
    expect(d.headline).toBe("唯一源文字");
    expect(d.risks).toEqual([]);
    expect(d.supporting).toEqual([]);
    expect(d.conflicting).toEqual([]);
    expect(d.status).toContainEqual({label:"账户状态",raw:"NOT_PROVIDED · 未知",display:"NOT_PROVIDED · 未知"});
    expect(d.eventCount).toBeNull();
  });

  it("shows absent summary as absent and never rewrites a Decision run as portfolio advice", () => {
    const empty=buildReadingDigest(readLiveReport({}));
    expect(empty).toMatchObject({headline:null,status:[],supporting:[],risks:[],conflicting:[]});
    const decision=buildReadingDigest(readLiveReport({symbol:"600519.SH",action:"WATCH",market_view:"等待验证",
      evidence:["价格未形成信号"],risk:["没有已完成的数据"]}));
    expect(decision.headline).toBe("等待验证");
    expect(decision.status).toEqual([{label:"研究动作（非委托）",raw:"WATCH",display:"持续观察 · WATCH"}]);
    expect(decision.supporting[0].text).toBe("价格未形成信号");
    expect(decision.risks[0].text).toBe("没有已完成的数据");
    expect(decision.eventCount).toBeNull();
  });

  it("reports an explicit event card independently of provider completeness and keeps readable original chapters", () => {
    const d=buildReadingDigest(readLiveReport(raw));
    expect(d.eventCount).toBe(0);
    expect(groups.flatMap(g=>g.cardIds)).toEqual(expect.arrayContaining([
      "synthesis","market","technical","event","fundamental","debate","manager","risk","intent","portfolio","ledger","decision"
    ]));
    const event = readLiveReport(raw).cards.find(c=>c.id==="event")!;
    expect(chapterCaption(event)).toBe("1 项已记录字段");
  });
});
