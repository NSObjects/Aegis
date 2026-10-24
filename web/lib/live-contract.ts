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
  documents?: Array<{
    document_id: string;
    version_id?: string;
    content_sha256?: string;
    publisher?: string;
    url?: string;
    source_type?: string;
    authority?: string;
    is_primary: boolean;
    published_at?: string;
    available_at?: string;
    first_seen_at?: string;
  }>;
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
  /** Explicit Go stage FAIL only; absent means location unknown. */
  failure_stage?: string;
  /** Fixed safe diagnostic category persisted by Go; no raw model/provider errors. */
  failure_category?: string;
  /** Server-validated terminal Run this new study was deliberately started from. */
  rerun_of?: string;
  request_snapshot?: RunRequestSnapshot;
  model_provenance?: RunModelProvenance;
  evidence_index?: RunEvidenceIndex;
  /** Go-owned, persisted raw daily series; validate before charting. */
  historical_ohlcv?: unknown;
}
export interface LiveStageEvent {
  kind?: "TOOL";
  tool?: string;
  outcome?: "SUCCEEDED" | "PARTIAL" | "TIMEOUT" | "FAILED";
  seq: number;
  stage: string;
  status: string;
  at: string;
  duration_ms?: number;
}
export interface LiveSession { authenticated: boolean; configured: boolean; mode: LiveMode; local_dev?: boolean; }
export type LiveArtifact = Record<string, unknown>;

/** Operator-curated watchlist entry; the reason/conditions are the operator's
 * own words, never a research verdict or a market snapshot. */
export interface WatchItem {
  symbol: string;
  reason: string;
  conditions: string;
  created_at: string;
  updated_at: string;
  /** Most recent persisted run for this symbol, resolved by Go; null when none exists. */
  latest_run: { run_id: string; state: LiveState; mode: LiveMode; created_at: string } | null;
}
export function isWatchItem(x: unknown): x is WatchItem {
  if (typeof x !== "object" || x === null) return false;
  const w = x as Record<string, unknown>;
  if (typeof w.symbol !== "string" || !isAshareSymbol(w.symbol)) return false;
  if (typeof w.reason !== "string" || w.reason.length === 0 || w.reason.length > 1024) return false;
  if (typeof w.conditions !== "string" || w.conditions.length > 1024) return false;
  if (typeof w.created_at !== "string" || !Number.isFinite(Date.parse(w.created_at))) return false;
  if (typeof w.updated_at !== "string" || !Number.isFinite(Date.parse(w.updated_at))) return false;
  if (w.latest_run !== null && typeof w.latest_run !== "object") return false;
  if (w.latest_run === null) return true;
  const ref = w.latest_run as Record<string, unknown>;
  return typeof ref.run_id === "string" && isRunID(ref.run_id) &&
    typeof ref.state === "string" && states.has(ref.state) &&
    (ref.mode === "DECISION" || ref.mode === "PORTFOLIO_REVIEW") &&
    typeof ref.created_at === "string" && Number.isFinite(Date.parse(ref.created_at));
}

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
  // Strictly validate observed tool metadata; old events without kind remain valid.
  if (e.kind !== undefined && e.kind !== "TOOL") return false;
  if (e.kind === "TOOL" &&
      (typeof e.tool !== "string" || !/^[A-Za-z0-9_-]{1,64}$/.test(e.tool) ||
        !["START", "DONE", "FAIL"].includes(String(e.status)))) return false;
  if (e.kind === "TOOL" && e.outcome !== undefined && !["SUCCEEDED", "PARTIAL", "TIMEOUT", "FAILED"].includes(String(e.outcome))) return false;
  if (e.kind !== "TOOL" && (e.tool !== undefined || e.outcome !== undefined)) return false;
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
