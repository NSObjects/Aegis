import type { LiveRun, LiveStageEvent } from "./live-contract";

const stages = new Set(["Run", "Market", "Technical", "Event", "Fundamental", "Synthesis", "Debate", "ResearchManager", "Trading", "Risk", "Portfolio", "FinalRiskGate"]);
const statuses = new Set(["PENDING", "RUNNING", "COMPLETED", "FAILED", "CANCELLED", "START", "DONE", "FAIL"]);
const categories = new Set(["EVENT_JSON_DECODE", "EVENT_MODEL_GENERATE", "EVENT_MODEL_EMPTY", "EVENT_CONTEXT_VALIDATION", "EVENT_SYNTHESIS_VALIDATION", "PERSISTENCE", "INVALID_RESULT", "PROCESS_INTERRUPTED", "LANGUAGE_VALIDATION", "SYNTHESIS_JSON_DECODE", "SYNTHESIS_SCHEMA_VALIDATION", "SYNTHESIS_MODEL_GENERATE", "SYNTHESIS_EMPTY_OUTPUT", "UNKNOWN"]);
const codes = new Set(["RESEARCH_FAILED", "PROCESS_INTERRUPTED", "INVALID_RESULT", "PERSISTENCE_FAILED"]);
const safe = (value: string | undefined, allowed: Set<string>) => value && allowed.has(value) ? value : "未提供";
const date = (value: string | undefined) => value && Number.isFinite(Date.parse(value)) ? new Date(value).toISOString() : "未提供";

/** Copy only persisted, bounded execution metadata; never stringify arbitrary Run or tool payloads. */
export function formatFailureDiagnostic(run: LiveRun, events: LiveStageEvent[]): string {
  const ordered = events.filter(ev =>
    Number.isSafeInteger(ev.seq) && ev.seq > 0 &&
    stages.has(ev.stage) && statuses.has(ev.status) &&
    Number.isFinite(Date.parse(ev.at)) &&
    (ev.kind !== "TOOL" || (ev.stage !== "Run" && typeof ev.tool === "string" && /^[A-Za-z0-9_-]{1,64}$/.test(ev.tool)))
  ).sort((a, b) => a.seq - b.seq).slice(0, 20000);
  const explicit = [...ordered].reverse().find(ev => ev.kind !== "TOOL" && ev.stage !== "Run" && ev.status === "FAIL");
  const stage = run.failure_stage && stages.has(run.failure_stage) && run.failure_stage !== "Run" ? run.failure_stage : explicit?.stage ?? "未定位";
  const lines = [
    "Aegis 研究失败诊断（仅包含允许公开的执行元数据）",
    `Run ID: ${/^[a-f0-9]{32}$/.test(run.run_id) ? run.run_id : "未提供"}`,
    `标的: ${/^[0-9]{6}\.(SH|SZ)$/.test(run.symbol) ? run.symbol : "未提供"}`,
    `状态: ${safe(run.state, statuses)}`,
    `失败阶段: ${stage}`,
    `错误代码: ${safe(run.failure_code, codes)}`,
    `错误类别: ${safe(run.failure_category, categories)}`,
    `开始时间: ${date(run.started_at)}`,
    `结束时间: ${date(run.finished_at)}`,
    "已观测事件（TOOL DONE 不代表阶段成功）：",
  ];
  for (const ev of ordered) {
    const tool = ev.kind === "TOOL" ? ` TOOL=${ev.tool}` : "";
    const ms = Number.isSafeInteger(ev.duration_ms) && ev.duration_ms! >= 0 ? ` ${ev.duration_ms}ms` : "";
    lines.push(`#${ev.seq} ${date(ev.at)} ${ev.stage}${tool} ${ev.status}${ms}`);
  }
  return lines.join("\n");
}
