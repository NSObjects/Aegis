import { describe, expect, it } from "vitest";
import { demoTraceForRun, claimsForEvidence, claimsForStage, resolveClaimEvidence } from "./research-trace";
import { initialDemoRuns } from "./demo-run-store";
import { demoStocks } from "./mock-research";

describe("demo research trace: provenance and fail-closed rules", () => {
  const stock = demoStocks[0];
  const finished = initialDemoRuns.find(r => r.status === "COMPLETED" && r.symbol === stock.symbol)!;

  it("never produces a report for no run, non-terminal or failed run", () => {
    expect(demoTraceForRun(null, stock)).toBeNull();
    for (const status of ["RUNNING", "FAILED", "CANCELLED", "PENDING"] as const) {
      expect(demoTraceForRun({ ...finished, status }, stock)).toBeNull();
    }
  });

  it("refuses to link a completed run to another symbol", () => {
    expect(demoTraceForRun(finished, demoStocks[1])).toBeNull();
  });

  it("links every claim to an existing mock reference and stage", () => {
    for (const sample of demoStocks) {
      const run = { ...finished, symbol: sample.symbol };
      const trace = demoTraceForRun(run, sample)!;
      expect(trace).not.toBeNull();
      for (const claim of trace.claims) {
        expect(resolveClaimEvidence(trace, claim.id, sample)).toEqual(claim.evidenceIds);
        expect(claim.evidenceIds.length).toBeGreaterThan(0);
        for (const evidence of claim.evidenceIds) {
          expect(claimsForEvidence(trace, evidence)).toContainEqual(claim);
        }
      }
      expect(claimsForStage(trace, "debate").map(c => c.side)).toEqual(["BULL", "BEAR"]);
      expect(trace.rounds.flatMap(r => r.claimIds)).toEqual(trace.claims.map(c => c.id));
    }
  });

  it("does not resolve unknown claim references", () => {
    const trace = demoTraceForRun(finished, stock)!;
    expect(resolveClaimEvidence(trace, "unknown", stock)).toEqual([]);
    expect(claimsForEvidence(trace, "unknown")).toEqual([]);
  });
});
