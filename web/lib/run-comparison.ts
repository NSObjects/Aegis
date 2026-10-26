import type { LiveArtifact, LiveRun } from "./live-contract";
import { readTechnicalVisual } from "./technical-visual";

type Obj = Record<string, unknown>;
const obj = (v: unknown): Obj | null => v !== null && typeof v === "object" && !Array.isArray(v) ? v as Obj : null;
const path = (v: unknown, keys: string[]): unknown => keys.reduce<unknown>((x, key) => obj(x)?.[key], v);
const scalar = (v: unknown): string | null =>
  typeof v === "string" && v.trim() !== "" ? v :
    typeof v === "number" && Number.isFinite(v) ? String(v) : null;

export type CompareKind = "same" | "different" | "only_left" | "only_right" | "unavailable";
export type CompareRow = { section: string; label: string; left: string | null; right: string | null; kind: CompareKind; key: string };
export type CompareEvidence = { left: string[] | null; right: string[] | null; added: string[] | null; removed: string[] | null };
export type ResearchComparison = { symbol: string; mode: LiveRun["mode"]; rows: CompareRow[]; evidence: CompareEvidence; changed: number; missing: number };

/** Find the nearest earlier completed Run that can be compared with the selected Run. */
export function findPreviousComparableRun(runs: LiveRun[], currentRunId: string): LiveRun | null {
  const current = runs.find(run => run.run_id === currentRunId && run.state === "COMPLETED");
  if (!current) return null;
  const currentTime = Date.parse(current.created_at);
  if (!Number.isFinite(currentTime)) return null;
  return runs
    .filter(run => run.run_id !== current.run_id && run.state === "COMPLETED" &&
      run.symbol === current.symbol && run.mode === current.mode)
    .map(run => ({ run, createdAt: Date.parse(run.created_at) }))
    .filter(item => Number.isFinite(item.createdAt) && item.createdAt < currentTime)
    .sort((a, b) => b.createdAt - a.createdAt)[0]?.run ?? null;
}

function validIdentity(run: LiveRun, artifact: unknown): boolean {
  const root = obj(artifact);
  if (!root || run.state !== "COMPLETED") return false;
  if (run.mode === "DECISION") return scalar(root.symbol) === run.symbol && typeof root.action === "string";
  const research = obj(root.research_artifact);
  if (!research || scalar(research.symbol) !== run.symbol) return false;
  // Absence is explicitly permitted, but a present context must not be for
  // another symbol. Run IDs prove storage identity, not individual field truth.
  for (const key of ["technical", "event", "fundamental"]) {
    const context = obj(research[key]);
    if (context && context.symbol !== undefined && scalar(context.symbol) !== run.symbol) return false;
  }
  const review = obj(root.portfolio_review);
  const plan = review && obj(review.research_plan);
  if (plan && plan.symbol !== undefined && scalar(plan.symbol) !== run.symbol) return false;
  return true;
}

function uniqueEventIds(artifact: unknown): string[] | null {
  const event = path(artifact, ["research_artifact", "event"]);
  if (!obj(event) || !Array.isArray(obj(event)?.events)) return null;
  const records = obj(event)?.events as unknown[];
  const ids: string[] = [];
  const found = new Set<string>();
  for (const raw of records) {
    const id = scalar(obj(raw)?.id);
    if (!id || id.length > 128 || found.has(id)) return null;
    found.add(id);
    ids.push(id);
  }
  return ids.sort();
}

/** No causal explanation or numeric delta is inferred from two independent Runs. */
export function compareResearchRuns(
  left: LiveRun, leftArtifact: LiveArtifact,
  right: LiveRun, rightArtifact: LiveArtifact,
): ResearchComparison | null {
  if (left.run_id === right.run_id || left.symbol !== right.symbol || left.mode !== right.mode ||
    !validIdentity(left, leftArtifact) || !validIdentity(right, rightArtifact)) return null;

  const rows: CompareRow[] = [];
  const add = (section: string, label: string, lv: unknown, rv: unknown) => {
    const a = scalar(lv), b = scalar(rv);
    const kind: CompareKind = a === null && b === null ? "unavailable" :
      a === null ? "only_right" : b === null ? "only_left" : a === b ? "same" : "different";
    rows.push({ section, label, left: a, right: b, kind, key: section + "/" + label });
  };
  add("研究记录", "研究截止（PIT）", left.requested_as_of, right.requested_as_of);
  add("研究记录", "模型配置指纹", left.model_provenance?.config_fingerprint, right.model_provenance?.config_fingerprint);
  add("研究记录", "任务状态", left.state, right.state);
  if (left.mode === "DECISION") {
    add("研究结论", "研究动作（非委托）", path(leftArtifact, ["action"]), path(rightArtifact, ["action"]));
    add("研究结论", "市场观点", path(leftArtifact, ["market_view"]), path(rightArtifact, ["market_view"]));
  } else {
    const p = (v: LiveArtifact, keys: string[]) => path(v, keys);
    const f = (section: string, label: string, keys: string[]) => add(section, label, p(leftArtifact, keys), p(rightArtifact, keys));
    f("研究结论", "研究综合摘要", ["research_artifact", "synthesis", "summary"]);
    f("研究结论", "研究经理结论", ["portfolio_review", "research_plan", "conclusion"]);
    f("研究结论", "研究经理摘要", ["portfolio_review", "research_plan", "summary"]);
    f("研究结论", "研究意向（非委托）", ["portfolio_review", "trade_intent", "status"]);
    f("市场环境", "市场状态", ["research_artifact", "market", "regime"]);
    f("市场环境", "市场数据日", ["research_artifact", "market", "as_of"]);
    f("市场环境", "上涨家数", ["research_artifact", "market", "advance_count"]);
    f("市场环境", "下跌家数", ["research_artifact", "market", "decline_count"]);
    f("市场环境", "市场成交额（源单位）", ["research_artifact", "market", "turnover"]);
    const lt = readTechnicalVisual(leftArtifact, left), rt = readTechnicalVisual(rightArtifact, right);
    add("技术分析", "技术数据状态", lt?.status, rt?.status);
    add("技术分析", "有效交易日", lt?.effective, rt?.effective);
    add("技术分析", "收盘价（RAW）", lt?.candle?.close, rt?.candle?.close);
    const techMetric = (section: "movingAverages" | "momentum" | "returns" | "volumes", key: string, visual: typeof lt) =>
      visual?.candle ? visual[section].find(item => item.key === key)?.value : null;
    for (const [section, key, label] of [
      ["movingAverages", "ma5", "MA5（技术计算口径）"],
      ["movingAverages", "ma10", "MA10（技术计算口径）"],
      ["movingAverages", "ma20", "MA20（技术计算口径）"],
      ["momentum", "rsi14", "RSI14"],
      ["momentum", "hist", "MACD HIST"],
      ["volumes", "volume_vs_ma5", "成交量 / MA5"],
    ] as const) add("技术分析", label, techMetric(section, key, lt), techMetric(section, key, rt));
    f("事件证据", "事件数据状态", ["research_artifact", "event", "status"]);
    f("事件证据", "事件证据观察截止", ["research_artifact", "event", "evidence_as_of"]);
    f("风险门禁", "风险评估", ["portfolio_review", "risk_context", "assessment"]);
    f("风险门禁", "风险分析摘要", ["portfolio_review", "risk_context", "summary"]);
    f("风险门禁", "账户状态", ["portfolio_review", "portfolio_decision", "portfolio_state"]);
    f("风险门禁", "组合审查结论", ["portfolio_review", "portfolio_decision", "disposition"]);
    f("风险门禁", "组合审查原因代码", ["portfolio_review", "portfolio_decision", "reason_code"]);
    f("风险门禁", "最终风险门禁", ["final_risk_gate", "disposition"]);
    f("风险门禁", "最终门禁原因", ["final_risk_gate", "reason"]);
  }
  const evLeft = left.mode === "PORTFOLIO_REVIEW" ? uniqueEventIds(leftArtifact) : null;
  const evRight = right.mode === "PORTFOLIO_REVIEW" ? uniqueEventIds(rightArtifact) : null;
  const both = evLeft !== null && evRight !== null;
  const evidence: CompareEvidence = {
    left: evLeft, right: evRight,
    added: both ? evRight.filter(id => !evLeft.includes(id)) : null,
    removed: both ? evLeft.filter(id => !evRight.includes(id)) : null,
  };
  return {
    symbol: left.symbol, mode: left.mode, rows, evidence,
    changed: rows.filter(r => r.kind === "different").length,
    missing: rows.filter(r => r.kind === "only_left" || r.kind === "only_right" || r.kind === "unavailable").length,
  };
}
