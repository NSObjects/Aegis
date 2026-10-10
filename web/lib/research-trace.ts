/**
 * UI-only research trace fixture, not a decoded Go EvidenceLedger or real LLM debate.
 * Links are stable within a stock fixture and may only be shown for a completed
 * mock run of that stock. Real API output must replace this entire adapter.
 */
import type { DemoRun } from "./demo-run-store";
import type { StockFixture } from "./mock-research";

export type ClaimSide = "BULL" | "BEAR" | "SYNTHESIS" | "RISK";
export type ClaimAssessment = "SUPPORTS" | "CONTESTS" | "UNVERIFIED";

export interface DemoClaim {
  id: string;
  side: ClaimSide;
  title: string;
  description: string;
  assessment: ClaimAssessment;
  stageId: string;
  evidenceIds: string[];
  respondsTo?: string;
}

export interface DemoDebateRound {
  id: string;
  label: string;
  summary: string;
  claimIds: string[];
}

export interface DemoResearchTrace {
  runId: string;
  symbol: string;
  asOf: string;
  claims: DemoClaim[];
  rounds: DemoDebateRound[];
}

/** Returns no report for cancelled, running, failed or mismatched-symbol runs. */
export function demoTraceForRun(run: DemoRun | null, stock: StockFixture): DemoResearchTrace | null {
  if (!run || run.status !== "COMPLETED" || run.symbol !== stock.symbol) return null;
  const [support, concern, missing] = stock.evidence;
  // The fixtures have one supportive, one cautionary and one not-yet-verified
  // reference. Preserve that provenance: references are NOT verified market facts.
  if (!support || !concern || !missing) return null;
  const prefix = `SAMPLE-${stock.symbol.replace(".", "-")}`;
  const bull = `${prefix}-BULL`;
  const bear = `${prefix}-BEAR`;
  return {
    runId: run.id,
    symbol: stock.symbol,
    asOf: "2026-09-24",
    claims: [
      {
        id: bull, side: "BULL", title: "多头：趋势结构假设", description: stock.bull,
        assessment: "SUPPORTS", evidenceIds: [support.id], stageId: "debate",
      },
      {
        id: bear, side: "BEAR", title: "空头：量能与失效风险", description: stock.bear,
        assessment: "CONTESTS", evidenceIds: [concern.id], stageId: "debate", respondsTo: bull,
      },
      {
        id: `${prefix}-MANAGER`, side: "SYNTHESIS", title: "研究经理：保持观察", description: stock.researchSummary,
        assessment: "UNVERIFIED", evidenceIds: [support.id, concern.id, missing.id], stageId: "manager", respondsTo: bear,
      },
      {
        id: `${prefix}-RISK`, side: "RISK", title: "风险审查：数据限制", description: stock.risk,
        assessment: "UNVERIFIED", evidenceIds: [concern.id, missing.id], stageId: "risk",
      },
    ],
    rounds: [
      { id: "opening", label: "01 · 初始观点", summary: "两侧分别提出支持与反对假设；引用均为静态演示样本。", claimIds: [bull] },
      { id: "rebuttal", label: "02 · 反驳与分歧", summary: "空头质疑持续性，并回应多头的趋势论点；不代表真实模型运行记录。", claimIds: [bear] },
      { id: "handoff", label: "03 · 研究结论交接", summary: "Research Manager 汇总分歧，风险阶段保留未知账户状态。", claimIds: [`${prefix}-MANAGER`, `${prefix}-RISK`] },
    ],
  };
}

export function claimsForEvidence(trace: DemoResearchTrace, evidenceId: string): DemoClaim[] {
  return trace.claims.filter(claim => claim.evidenceIds.includes(evidenceId));
}

export function claimsForStage(trace: DemoResearchTrace, stageId: string): DemoClaim[] {
  return trace.claims.filter(claim => claim.stageId === stageId);
}

export function resolveClaimEvidence(trace: DemoResearchTrace, claimId: string, stock: StockFixture): string[] {
  const claim = trace.claims.find(item => item.id === claimId);
  return claim ? claim.evidenceIds.filter(id => stock.evidence.some(e => e.id === id)) : [];
}
