import { describe, it, expect } from "vitest";
import { isLiveRun, isLiveEvent, mergeStageEvents, readReportSummary, isAshareSymbol } from "./live-contract";
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
