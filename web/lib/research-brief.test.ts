import { describe, expect, it } from "vitest";
import { readLiveReport } from "./live-report";
import { readResearchBrief, readableResearchCode, summarizeBriefBlocks } from "./research-brief";

describe("research brief is source-backed and safe", () => {
 it("limits summary previews to three source-diverse lines and counts omitted details",()=>{
   const summary=summarizeBriefBlocks([
     {title:"经理观点",source:"研究经理",lines:["依据 A","依据 B","依据 C"]},
     {title:"综合观点",source:"综合分析",lines:["依据 D","依据 E"]},
     {title:"事件依据",source:"事件分析",lines:["依据 F"]},
     {title:"决策依据",source:"决策分析",lines:["依据 G"]},
   ]);
   expect(summary.blocks.flatMap(block=>block.lines)).toEqual(["依据 A","依据 F","依据 G"]);
   expect(summary.blocks.map(block=>block.source)).toEqual(["研究经理","事件分析","决策分析"]);
   expect(summary.omittedLines).toBe(4);
   expect(summarizeBriefBlocks([{title:"单一来源",source:"来源",lines:["A","B","C","D"]}]).blocks[0].lines).toEqual(["A","B","C"]);
   const twoSources=summarizeBriefBlocks([
     {title:"来源一",source:"来源一",lines:["A","B"]},
     {title:"来源二",source:"来源二",lines:["C","D"]},
   ]);
   expect(twoSources.blocks.flatMap(block=>block.lines)).toEqual(["A","B","C"]);
   expect(twoSources.blocks.map(block=>block.lines.length)).toEqual([2,1]);
 });
 it("counts all source entries omitted from the brief preview while preserving the full chapter",()=>{
   const opportunities=Array.from({length:7},(_,i)=>({claim:`机会 ${i+1}`}));
   const risks=Array.from({length:6},(_,i)=>({claim:`风险 ${i+1}`}));
   const report=readLiveReport({
     research_artifact:{symbol:"600519.SH"},
     portfolio_review:{research_plan:{conclusion:"BALANCED",opportunities,risks}},
   });
   const brief=readResearchBrief(report);
   expect(brief.evidence[0].lines).toHaveLength(7);
   expect(brief.risks[0].lines).toHaveLength(6);
   const evidence=summarizeBriefBlocks(brief.evidence);
   const risk=summarizeBriefBlocks(brief.risks);
   expect(evidence.blocks.flatMap(block=>block.lines)).toHaveLength(3);
   expect(evidence.omittedLines).toBe(4);
   expect(risk.blocks.flatMap(block=>block.lines)).toHaveLength(3);
   expect(risk.omittedLines).toBe(3);
 });
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
       trade_intent:{status:"MONITOR"},risk_context:{unresolved:[{claim:"尚无账户信息"}]},portfolio_decision:{portfolio_state:"NOT_PROVIDED"}},
     final_risk_gate:{disposition:"REVIEW_REQUIRED"},
   }));
   expect(result.conclusion).toBe("BALANCED");
   expect(result.marketView).toBe("MIXED");
   expect(result.action).toBe("MONITOR");
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
   expect(result.marketView).toBe("风险偏大");
   expect(result.action).toBe("WATCH");
   expect(result.posture).toBeNull();
   expect(result.evidence.flatMap(x=>x.lines)).toContain("波动走高");
   expect(result.risks.flatMap(x=>x.lines)).toContain("流动性不足");
 });
});
