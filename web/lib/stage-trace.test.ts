import { describe, expect, it } from "vitest";
import { buildRunTrace, stageReportId } from "./stage-trace";
import { mergeStageEvents, type LiveRun, type LiveStageEvent } from "./live-contract";

const time = "2026-10-11T07:00:00Z";
const run = (state: LiveRun["state"], failure_code?: string): LiveRun => ({
  run_id: "a".repeat(32), symbol: "600519.SH", mode: "PORTFOLIO_REVIEW",
  requested_as_of: time, created_at: time, state, ...(failure_code ? { failure_code } : {}),
});
const event = (seq: number, stage: string, status: string, duration_ms?: number): LiveStageEvent => ({
  seq, stage, status, at: time, ...(duration_ms === undefined ? {} : { duration_ms }),
});

describe("evidence-grounded Go stage timeline", () => {
  it("shows only observed stages and separates Run lifecycle from agent work", () => {
    const trace = buildRunTrace(run("COMPLETED"), [
      event(4, "Event", "DONE", 0), event(1, "Run", "PENDING"), event(2, "Market", "START"),
      event(3, "Event", "START"), event(5, "Market", "DONE", 1210), event(6, "Run", "COMPLETED"),
    ]);
    expect(trace.stages.map(s => s.stage)).toEqual(["Market", "Event"]);
    expect(trace.lifecycle.map(ev => ev.status)).toEqual(["PENDING", "COMPLETED"]);
    expect(trace.stages.map(s => s.phase)).toEqual(["DONE", "DONE"]);
    expect(trace.stages[0].durationMS).toBe(1210);
    expect(trace.stages[1].durationMS).toBeNull();
    expect(trace.explicitFailure).toBeNull();
    expect(trace.stages.some(s => s.stage === "Fundamental")).toBe(false);
  });

  it("does not invent a completed stage after a cancelled or failed run", () => {
    const incomplete = buildRunTrace(run("CANCELLED"), [event(1, "Run", "RUNNING"), event(2, "Market", "START"), event(3, "Run", "CANCELLED")]);
    expect(incomplete.stages[0].phase).toBe("INCOMPLETE");
    expect(incomplete.stages[0].finishedAt).toBeNull();
    expect(incomplete.unlocatedFailure).toBe(false);
    const failed = buildRunTrace(run("FAILED", "RESEARCH_FAILED"), [event(1, "Market", "START"), event(2, "Run", "FAILED")]);
    expect(failed.unlocatedFailure).toBe(true);
    expect(failed.explicitFailure).toBeNull();
    expect(failed.stages[0].phase).toBe("INCOMPLETE");
  });

  it("locates only explicit stage FAIL, without treating every unfinished neighbor as failed", () => {
    const trace = buildRunTrace(run("FAILED", "RESEARCH_FAILED"), [
      event(1, "Event", "START"), event(2, "Market", "START"),
      event(3, "Event", "FAIL", 237), event(4, "Run", "FAILED"),
    ]);
    expect(trace.explicitFailure).toBe("Event");
    expect(trace.unlocatedFailure).toBe(false);
    expect(trace.stages.find(s => s.stage === "Market")?.phase).toBe("INCOMPLETE");
    expect(trace.stages.find(s => s.stage === "Event")?.durationMS).toBe(237);
  });

  it("replays merged SSE/poll events deterministically and uses last stage attempt", () => {
    const a = [event(1, "Market", "START"), event(2, "Market", "FAIL", 300)];
    const merged = mergeStageEvents(a, [event(2, "Market", "FAIL", 300), event(3, "Market", "START"), event(4, "Market", "DONE", 210)]);
    const trace = buildRunTrace(run("COMPLETED"), merged);
    expect(trace.stages).toHaveLength(1);
    expect(trace.stages[0]).toMatchObject({ phase: "DONE", firstSeq: 1, lastSeq: 4, durationMS: 210 });
  });

  it("never promotes unrecognized source statuses to success", () => {
    const trace = buildRunTrace(run("RUNNING"), [event(1, "ToolMystery", "SKIPPED"), event(2, "Market", "START")]);
    expect(trace.stages[0].phase).toBe("UNKNOWN");
    expect(trace.stages[0].rawStatus).toBe("SKIPPED");
    expect(trace.stages[1].phase).toBe("RUNNING");
  });

  it("keeps observed tool events out of agent phase transitions", () => {
    const trace = buildRunTrace(run("COMPLETED"), [
      event(1, "Event", "START"),
      { ...event(2, "Event", "FAIL"), kind: "TOOL", tool: "news_search" },
      { ...event(3, "Event", "DONE"), kind: "TOOL", tool: "news_search" },
      event(4, "Event", "DONE", 120),
    ]);
    expect(trace.stages).toHaveLength(1);
    expect(trace.stages[0]).toMatchObject({ stage: "Event", phase: "DONE", lastSeq: 4, durationMS: 120 });
    expect(trace.explicitFailure).toBeNull();
  });

  it("links only known report cards and not arbitrary stage names", () => {
    expect(stageReportId.Market).toBe("market");
    expect(stageReportId.FinalRiskGate).toBe("portfolio");
    expect(stageReportId.Unexpected).toBeUndefined();
  });
});
