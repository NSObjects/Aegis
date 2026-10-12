import { describe, expect, it } from "vitest";
import { readLiveReport } from "./live-report";
import { readResearchBrief, readableResearchCode } from "./research-brief";

describe("research brief is source-backed and safe", () => {
 it("glosses only known research enums and keeps exact backend codes",()=>{
   expect(readableResearchCode("BALANCED")).toBe("多空观点相对均衡（BALANCED）");
   expect(readableResearchCode("REVIEW_REQUIRED")).toBe("需要人工复核（REVIEW_REQUIRED）");
   expect(readableResearchCode("BULL_LEAN")).toBe("研究偏多（BULL_LEAN）");
   expect(readableResearchCode("HIGH_RISK")).toBe("高风险（HIGH_RISK）");
   expect(readableResearchCode("CANDIDATE")).toBe("进入候选观察名单（CANDIDATE）");
   expect(readableResearchCode("unknown")).toBe("unknown");
   expect(readableResearchCode(null)).toBe("未提供");
 });
 it("organizes the exact portfolio text and distinguishes risk from missing data", () => {
   const result=readResearchBrief(readLiveReport({
     research_artifact:{symbol:"600519.SH",synthesis:{summary:"多空观点仍有分歧",agreements:[{claim:"市场偏弱"}],risks:["外部事件未验证"]},
       market:{regime:"MIXED",advance_count:0,decline_count:4100},
       technical:{status:"NO_DATA"}},
     portfolio_review:{research_plan:{conclusion:"BALANCED",summary:"建议继续观察",opportunities:[{claim:"量能改善"}],risks:[{claim:"事件影响未知"}]},
       risk_context:{unresolved:[{claim:"尚无账户信息"}]},portfolio_decision:{portfolio_state:"NOT_PROVIDED"}},
     final_risk_gate:{disposition:"REVIEW_REQUIRED"},
   }));
   expect(result.conclusion).toBe("BALANCED");
   expect(result.summary).toBe("建议继续观察");
   expect(result.posture).toBe("REVIEW_REQUIRED");
   expect(result.evidence.flatMap(x=>x.lines)).toContain("量能改善");
   expect(result.risks.flatMap(x=>x.lines)).toContain("事件影响未知");
   expect(result.unknowns.join(" ")).toMatch(/账户状态未提供/);
   expect(result.unknowns.join(" ")).toMatch(/没有可用的技术行情/);
   expect(result.market).toContainEqual({label:"上涨家数",value:"0"});
 });
 it("does not replace missing evidence with invented thesis, valuation or prices",()=>{
   const result=readResearchBrief(readLiveReport({research_artifact:{symbol:"600519.SH",event:{status:"NO_EVENT"}}}));
   expect(result.summary).toBeNull();
   expect(result.conclusion).toBeNull();
   expect(result.evidence).toEqual([]);
   expect(result.technical).toEqual([]);
   expect(result.unknowns.join(" ")).toMatch(/没有可用的技术行情/);
 });
 it("preserves Decision-mode research evidence and risk without claiming a trade",()=>{
   const result=readResearchBrief(readLiveReport({symbol:"600519.SH",action:"WATCH",market_view:"风险偏大",evidence:["波动走高"],risk:["流动性不足"]}));
   expect(result.conclusion).toBe("WATCH");
   expect(result.posture).toBeNull();
   expect(result.evidence.flatMap(x=>x.lines)).toContain("波动走高");
   expect(result.risks.flatMap(x=>x.lines)).toContain("流动性不足");
 });
});
