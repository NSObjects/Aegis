/** Exact metadata/event field names from the Go ResearchRun API.
 * Server-owned research_artifact stays opaque until an actual COMPLETED run is loaded.
 */
export type LiveState = "PENDING" | "RUNNING" | "COMPLETED" | "FAILED" | "CANCELLED";
export type LiveMode = "DECISION" | "PORTFOLIO_REVIEW";
// Optional for pre-R1-03 runs. This describes submitted inputs only;
// it is NOT proof of provider/model versions or actual retrieved sources.
export interface RunRequestSnapshot {
  schema_version: 1;
  market_query_provided: boolean;
  event_window_start?: string;
  event_window_end?: string;
  event_pit_safe: boolean;
  preferred_source_types?: string[];
  min_documents?: number;
  min_independent_sources?: number;
  portfolio_state?: string;
}
/** Captured on successful Go graph construction, not a verified provider revision. */
export interface RunModelProvenance {
  schema_version: 1;
  config_fingerprint: string;
  bindings: Array<{ role: string; profile: string; provider: string; model: string }>;
}
/**
 * Copied from the exact completed Portfolio Review EventContext.
 * References are canonical identities only, not original URLs or audited sources.
 */
export interface RunEvidenceIndex {
  schema_version: 1;
  status: "NORMAL" | "NO_EVENT" | "INSUFFICIENT_EVIDENCE";
  requested_as_of: string;
  evidence_as_of?: string;
  events: Array<{
    event_id: string;
    occurred_at?: string;
    source_count: number;
    document_ids: string[];
  }>;
  document_ids: string[];
}
export interface LiveRun {
  run_id: string;
  mode: LiveMode;
  symbol: string;
  requested_as_of: string;
  state: LiveState;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  failure_code?: string;
  /** Server-validated terminal Run this new study was deliberately started from. */
  rerun_of?: string;
  request_snapshot?: RunRequestSnapshot;
  model_provenance?: RunModelProvenance;
  evidence_index?: RunEvidenceIndex;
}
export interface LiveStageEvent {
  seq: number;
  stage: string;
  status: string;
  at: string;
  duration_ms?: number;
}
export interface LiveSession { authenticated: boolean; configured: boolean; mode: LiveMode; local_dev?: boolean; }
export type LiveArtifact = Record<string, unknown>;

const states = new Set<string>(["PENDING", "RUNNING", "COMPLETED", "FAILED", "CANCELLED"]);
export const isTerminal = (s: LiveState) => s === "COMPLETED" || s === "FAILED" || s === "CANCELLED";
export const isRunID = (s: string) => /^[a-f0-9]{32}$/.test(s);
export const isAshareSymbol = (s: string) => /^[0-9]{6}\.(SH|SZ)$/.test(s);
export function isLiveRun(x: unknown): x is LiveRun {
  if (typeof x !== "object" || x === null) return false;
  const r = x as Record<string, unknown>;
  return typeof r.run_id === "string" && isRunID(r.run_id) &&
    (r.mode === "DECISION" || r.mode === "PORTFOLIO_REVIEW") &&
    typeof r.symbol === "string" && isAshareSymbol(r.symbol) &&
    typeof r.state === "string" && states.has(r.state) &&
    typeof r.created_at === "string" && Number.isFinite(Date.parse(r.created_at)) &&
    typeof r.requested_as_of === "string" && Number.isFinite(Date.parse(r.requested_as_of)) &&
    (r.rerun_of === undefined || (typeof r.rerun_of === "string" && isRunID(r.rerun_of)));
}
export function isLiveEvent(x: unknown): x is LiveStageEvent {
  if (typeof x !== "object" || x === null) return false;
  const e = x as Record<string, unknown>;
  return Number.isSafeInteger(e.seq) && (e.seq as number) > 0 &&
    typeof e.stage === "string" && e.stage.length > 0 && e.stage.length <= 100 &&
    typeof e.status === "string" && e.status.length > 0 &&
    typeof e.at === "string" && Number.isFinite(Date.parse(e.at));
}
export function mergeStageEvents(before: LiveStageEvent[], incoming: LiveStageEvent[]): LiveStageEvent[] {
  const bySeq = new Map(before.map(ev => [ev.seq, ev]));
  for (const event of incoming) if (isLiveEvent(event)) bySeq.set(event.seq, event);
  return [...bySeq.values()].sort((a, b) => a.seq - b.seq);
}
export function readReportSummary(value: unknown): { headline: string | null; sections: Array<{ label: string; value: string }> } {
  if (!value || typeof value !== "object" || Array.isArray(value)) return { headline: null, sections: [] };
  const result = value as Record<string, unknown>;
  const obj = (v: unknown) => v && typeof v === "object" && !Array.isArray(v) ? v as Record<string, unknown> : {};
  const artifact = obj(result.research_artifact);
  const synthesis = obj(artifact.synthesis);
  const gate = obj(result.final_risk_gate);
  const intent = obj(obj(result.portfolio_review).trade_intent);
  const sections: Array<{ label: string; value: string }> = [];
  if (typeof result.market_view === "string") sections.push({ label: "Market view · Decision", value: result.market_view });
  if (typeof result.action === "string") sections.push({ label: "Research action · Decision", value: result.action });
  if (typeof synthesis.summary === "string") sections.push({ label: "Research synthesis", value: synthesis.summary });
  if (typeof gate.disposition === "string") sections.push({ label: "Final risk gate", value: gate.disposition });
  if (typeof gate.reason === "string") sections.push({ label: "Risk gate reason", value: gate.reason });
  if (typeof intent.status === "string") sections.push({ label: "TradeIntent · Research only", value: intent.status });
  return { headline: typeof synthesis.summary === "string" ? synthesis.summary : typeof result.market_view === "string" ? result.market_view : null, sections };
}
