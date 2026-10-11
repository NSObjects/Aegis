import type { LiveRun, LiveStageEvent } from "./live-contract";

export type StagePhase = "RUNNING" | "DONE" | "FAIL" | "INCOMPLETE" | "UNKNOWN";

export interface StageTrace {
  stage: string;
  phase: StagePhase;
  firstSeq: number;
  lastSeq: number;
  startedAt: string | null;
  finishedAt: string | null;
  /** Only populated from a trusted Go duration_ms, never a browser-clock estimate. */
  durationMS: number | null;
  rawStatus: string;
}

export interface RunTrace {
  stages: StageTrace[];
  lifecycle: LiveStageEvent[];
  explicitFailure: string | null;
  unlocatedFailure: boolean;
}

/** Only stages actually reported by Go may appear; absence never means SKIPPED. */
export function buildRunTrace(run: LiveRun, events: LiveStageEvent[]): RunTrace {
  const ordered = [...events]
    .filter(ev => Number.isSafeInteger(ev.seq) && ev.seq > 0 &&
      typeof ev.stage === "string" && ev.stage.length > 0 &&
      typeof ev.status === "string" && Number.isFinite(Date.parse(ev.at)))
    .sort((a, b) => a.seq - b.seq);
  const byName = new Map<string, StageTrace>();
  const lifecycle: LiveStageEvent[] = [];
  for (const ev of ordered) {
    if (ev.stage === "Run") {
      lifecycle.push(ev);
      continue;
    }
    const status = ev.status.toUpperCase();
    const phase: StagePhase = status === "START" ? "RUNNING" :
      status === "DONE" ? "DONE" : status === "FAIL" ? "FAIL" : "UNKNOWN";
    const existing = byName.get(ev.stage);
    if (!existing) {
      byName.set(ev.stage, {
        stage: ev.stage, phase, firstSeq: ev.seq, lastSeq: ev.seq,
        startedAt: status === "START" ? ev.at : null,
        finishedAt: status === "DONE" || status === "FAIL" ? ev.at : null,
        durationMS: typeof ev.duration_ms === "number" && Number.isFinite(ev.duration_ms) && ev.duration_ms > 0
          && (status === "DONE" || status === "FAIL") ? ev.duration_ms : null,
        rawStatus: ev.status,
      });
      continue;
    }
    // Preserve the most recent START for repeated stages, but never infer a
    // duration from local timestamps or guess a skipped/failing Agent.
    existing.lastSeq = ev.seq;
    existing.phase = phase;
    existing.rawStatus = ev.status;
    if (status === "START") {
      existing.startedAt = ev.at;
      existing.finishedAt = null;
      existing.durationMS = null;
    } else if (status === "DONE" || status === "FAIL") {
      existing.finishedAt = ev.at;
      existing.durationMS = typeof ev.duration_ms === "number" && Number.isFinite(ev.duration_ms)
        && ev.duration_ms > 0 ? ev.duration_ms : null;
    } else {
      existing.durationMS = null;
    }
  }
  const terminal = run.state === "COMPLETED" || run.state === "FAILED" || run.state === "CANCELLED";
  const stages = [...byName.values()].map(s => ({
    ...s, phase: terminal && s.phase === "RUNNING" ? "INCOMPLETE" as StagePhase : s.phase,
  }));
  const failedStage = stages.find(s => s.phase === "FAIL");
  return {
    stages, lifecycle,
    explicitFailure: failedStage?.stage ?? null,
    unlocatedFailure: run.state === "FAILED" && !failedStage,
  };
}

export const stageLabels: Record<string, string> = {
  Market: "市场分析", Technical: "技术分析", Event: "事件分析",
  Fundamental: "基本面分析", Synthesis: "研究综合",
  Debate: "多空辩论", ResearchManager: "研究经理",
  Trading: "研究意向", Risk: "风险分析",
  Portfolio: "组合审查", FinalRiskGate: "最终风险门禁",
};

export const stageReportId: Record<string, string> = {
  Market: "market", Technical: "technical", Event: "event",
  Fundamental: "fundamental", Synthesis: "synthesis",
  Debate: "debate", ResearchManager: "manager", Trading: "intent",
  Risk: "risk", Portfolio: "portfolio", FinalRiskGate: "portfolio",
};
