import { describe, expect, it } from "vitest";
import {
  advanceDemoRun,
  cancelDemoRun,
  createDemoRun,
  initialDemoRuns,
  keepRecentDemoRuns,
  restoreDemoRuns,
  type DemoRun,
} from "./demo-run-store";
import { stages } from "./mock-research";

function runningRun(overrides: Partial<DemoRun> = {}): DemoRun {
  return {
    id: "DEMO-test-run",
    symbol: "600519.SH",
    createdAt: "2026-09-24T10:00:00+08:00",
    status: "RUNNING",
    completedStages: 0,
    scenario: "success",
    ...overrides,
  };
}

describe("mock research run lifecycle", () => {
  it("creates a demo-only run with the selected stock and scenario", () => {
    const result = createDemoRun("000001.SZ", "provider_failure", new Date("2026-09-24T02:00:00Z"));
    expect(result.id).toMatch(/^DEMO-/);
    expect(result.symbol).toBe("000001.SZ");
    expect(result.scenario).toBe("provider_failure");
    expect(result.completedStages).toBe(0);
    expect(result.status).toBe("RUNNING");
  });

  it("completes only after all simulated agent stages", () => {
    let run = runningRun();
    for (let i = 0; i < stages.length - 1; i++) {
      run = advanceDemoRun(run);
      expect(run.status).toBe("RUNNING");
    }
    run = advanceDemoRun(run);
    expect(run.completedStages).toBe(stages.length);
    expect(run.status).toBe("COMPLETED");
    expect(advanceDemoRun(run)).toBe(run);
  });

  it("stops on a simulated Event provider failure without completing downstream roles", () => {
    let run = runningRun({ scenario: "provider_failure" });
    run = advanceDemoRun(advanceDemoRun(run));
    expect(run.completedStages).toBe(2);
    run = advanceDemoRun(run);
    expect(run.status).toBe("FAILED");
    expect(run.completedStages).toBe(2);
    expect(run.failureStage).toBe("event");
    expect(run.failureReason).toContain("DEMO_PROVIDER_UNAVAILABLE");
    expect(advanceDemoRun(run)).toBe(run);
  });

  it("cancellation is terminal and cannot resume on the next tick", () => {
    const cancelled = cancelDemoRun(runningRun({ completedStages: 4 }));
    expect(cancelled.status).toBe("CANCELLED");
    expect(cancelled.completedStages).toBe(4);
    expect(advanceDemoRun(cancelled)).toBe(cancelled);
    expect(cancelDemoRun(cancelled)).toBe(cancelled);
  });

  it("restores completed history and fails closed on in-progress browser refresh", () => {
    const records = [
      runningRun(),
      runningRun({ id: "DEMO-complete", status: "COMPLETED", completedStages: stages.length }),
    ];
    const restored = restoreDemoRuns(JSON.stringify(records));
    expect(restored[0].status).toBe("FAILED");
    expect(restored[0].failureReason).toContain("DEMO_SESSION_INTERRUPTED");
    expect(restored[1].status).toBe("COMPLETED");
    expect(restored[1].completedStages).toBe(stages.length);
  });

  it("does not trust corrupted or non-demo persistent records", () => {
    expect(restoreDemoRuns("not-json")).toEqual(initialDemoRuns);
    expect(restoreDemoRuns(JSON.stringify([{ id: "LIVE-123", symbol: "600519.SH" }]))).toEqual(initialDemoRuns);
    expect(restoreDemoRuns(JSON.stringify([{ ...runningRun(), symbol: "FAKE.STOCK" }]))).toEqual(initialDemoRuns);
  });

  it("keeps only the latest 50 demo records", () => {
    const all = Array.from({ length: 60 }, (_, i) => runningRun({ id: `DEMO-${i}` }));
    expect(keepRecentDemoRuns(all)).toHaveLength(50);
    expect(keepRecentDemoRuns(all)[0].id).toBe("DEMO-0");
  });
});
