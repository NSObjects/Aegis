import { describe, it, expect } from "vitest";
import { isLiveRun, isLiveEvent, mergeStageEvents, readReportSummary, isAshareSymbol, isWatchItem } from "./live-contract";
const run = { run_id: "a".repeat(32), mode: "PORTFOLIO_REVIEW", symbol: "600519.SH", requested_as_of: "2026-10-10T12:00:00Z", created_at: "2026-10-10T12:00:00Z", state: "RUNNING" };
const ev = (n: number) => ({ seq: n, stage: "Market", status: "START", at: "2026-10-10T12:00:00Z" });
describe("live Go contract (not demo)", () => {
  it("recognizes Go state, not proposed status", () => { expect(isLiveRun(run)).toBe(true); expect(isLiveRun({ ...run, state: undefined, status: "RUNNING" })).toBe(false); expect(isLiveRun({ ...run, run_id: "DEMO-1" })).toBe(false); });
  it("accepts only supported exchange-qualified symbols", () => { expect(isAshareSymbol("600519.SH")).toBe(true); expect(isAshareSymbol("../../etc/passwd")).toBe(false); });
  it("uses strictly increasing durable SSE seq and deduplicates replay", () => { expect(isLiveEvent(ev(1))).toBe(true); expect(mergeStageEvents([ev(1), ev(2)], [ev(2), ev(3)]).map(v => v.seq)).toEqual([1, 2, 3]); });
  it("never invents risk or account values from absent artifact", () => { expect(readReportSummary({}).sections).toEqual([]); expect(readReportSummary({ research_artifact: { synthesis: { summary: "真实研究结果" } }, final_risk_gate: { disposition: "REVIEW_REQUIRED" } }).sections.map(x => x.value)).toEqual(["真实研究结果", "REVIEW_REQUIRED"]); });
});

describe("rerun origin metadata", () => {
  it("accepts an optional validated parent Run ID but rejects untrusted values", () => {
    expect(isLiveRun({ ...run, rerun_of: "b".repeat(32) })).toBe(true);
    expect(isLiveRun({ ...run, rerun_of: "../bad" })).toBe(false);
    expect(isLiveRun({ ...run, rerun_of: "" })).toBe(false);
  });
});

describe("observed tool event validation", () => {
  const tool = { seq: 8, stage: "Event", status: "FAIL", kind: "TOOL" as const, tool: "snapshot_official", at: "2026-10-10T12:00:00Z" };
  it("accepts only sanitized observed tool metadata", () => {
    expect(isLiveEvent(tool)).toBe(true);
    expect(isLiveEvent({ ...tool, tool: "api_key=secret" })).toBe(false);
    expect(isLiveEvent({ ...tool, status: "SUCCESS" })).toBe(false);
    expect(isLiveEvent({ ...tool, kind: "TRACE" })).toBe(false);
    expect(isLiveEvent({ ...tool, tool: "a".repeat(65) })).toBe(false);
    expect(isLiveEvent({ ...tool, kind: undefined })).toBe(false);
  });
  it("deduplicates retriever events by durable sequence", () => {
    const merged = mergeStageEvents([], [tool, tool, { ...tool, seq: 9, status: "DONE" }]);
    expect(merged.map(x => x.seq)).toEqual([8, 9]);
  });
});

describe("watchlist entry contract", () => {
  const item = { symbol: "600519.SH", reason: "回调观察", conditions: "", created_at: "2026-10-24T08:00:00Z", updated_at: "2026-10-24T08:00:00Z", latest_run: null };
  it("accepts a Go watch item with no research link yet", () => {
    expect(isWatchItem(item)).toBe(true);
    expect(isWatchItem({ ...item, conditions: "跌破 MA20 复查" })).toBe(true);
  });
  it("validates the linked latest run like a run reference", () => {
    expect(isWatchItem({ ...item, latest_run: { run_id: "b".repeat(32), state: "FAILED", mode: "DECISION", created_at: "2026-10-24T09:00:00Z" } })).toBe(true);
    expect(isWatchItem({ ...item, latest_run: { run_id: "../bad", state: "FAILED", mode: "DECISION", created_at: "2026-10-24T09:00:00Z" } })).toBe(false);
    expect(isWatchItem({ ...item, latest_run: { run_id: "b".repeat(32), state: "DONE", mode: "DECISION", created_at: "2026-10-24T09:00:00Z" } })).toBe(false);
  });
  it("rejects forged operator fields and non-symbols", () => {
    expect(isWatchItem({ ...item, symbol: "600519" })).toBe(false);
    expect(isWatchItem({ ...item, reason: "" })).toBe(false);
    expect(isWatchItem({ ...item, reason: "x".repeat(1025) })).toBe(false);
    expect(isWatchItem({ ...item, conditions: "x".repeat(1025) })).toBe(false);
    expect(isWatchItem({ ...item, created_at: "not-a-date" })).toBe(false);
    expect(isWatchItem({ ...item, latest_run: undefined })).toBe(false);
  });
});
