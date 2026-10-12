import { describe, expect, it } from "vitest";
import { formatFailureDiagnostic } from "./failure-diagnostic";
import type { LiveRun, LiveStageEvent } from "./live-contract";

const run: LiveRun = {
  run_id: "a".repeat(32), mode: "DECISION", symbol: "600519.SH",
  requested_as_of: "2026-10-12T08:00:00Z", created_at: "2026-10-12T08:00:00Z",
  state: "FAILED", failure_code: "RESEARCH_FAILED", failure_category: "EVENT_JSON_DECODE",
};
const at = "2026-10-12T08:00:00Z";
describe("failure diagnostic export", () => {
  it("reports only observed failures, never infers them from tool completion", () => {
    const events: LiveStageEvent[] = [
      { seq: 2, kind: "TOOL", tool: "snapshot_breaking_news", stage: "Event", status: "DONE", at, duration_ms: 12 },
      { seq: 1, stage: "Event", status: "START", at },
    ];
    const output = formatFailureDiagnostic(run, events);
    expect(output).toContain("失败阶段: 未定位");
    expect(output).toContain("#2");
    expect(output.indexOf("#1")).toBeLessThan(output.indexOf("#2"));
    expect(formatFailureDiagnostic(run, [...events, { seq: 3, stage: "Event", status: "FAIL", at }])).toContain("失败阶段: Event");
  });
  it("exports recognized Event post-retrieval categories without raw errors", () => {
    for (const category of ["EVENT_EVIDENCE_LOOP", "EVENT_CAPTURE", "EVENT_ANALYSIS", "EVENT_SOURCE_VALIDATION"]) {
      const output = formatFailureDiagnostic({ ...run, failure_category: category }, [
        { seq: 1, kind: "TOOL", tool: "snapshot_breaking_news", stage: "Event", status: "DONE", at },
        { seq: 2, stage: "Event", status: "FAIL", at },
      ]);
      expect(output).toContain(`错误类别: ${category}`);
      expect(output).toContain("失败阶段: Event");
    }
  });
  it("omits untrusted text and unknown diagnostic categories", () => {
    const hostile = { ...run, failure_category: "SECRET_KEY=123", failure_stage: "bad\nsecret" };
    const events: LiveStageEvent[] = [
      { seq: 1, stage: "Event\nSECRET", status: "FAIL", at },
      { seq: 2, kind: "TOOL", tool: "api_key=secret", stage: "Event", status: "DONE", at },
    ];
    const output = formatFailureDiagnostic(hostile, events);
    expect(output).not.toContain("SECRET");
    expect(output).not.toContain("secret");
    expect(output).toContain("错误类别: 未提供");
  });
});
