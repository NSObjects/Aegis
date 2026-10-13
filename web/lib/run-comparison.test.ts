import { describe, expect, it } from "vitest";
import { compareResearchRuns } from "./run-comparison";
import type { LiveArtifact, LiveRun } from "./live-contract";

const makeRun = (id: string, symbol="600519.SH", mode: LiveRun["mode"]="PORTFOLIO_REVIEW", state: LiveRun["state"]="COMPLETED"): LiveRun => ({
  run_id: id.repeat(32), symbol, mode, state,
  requested_as_of: "2026-10-11T07:00:00Z", created_at: "2026-10-11T07:00:00Z",
});
const a = makeRun("a"), b = makeRun("b");

function artifact(input: {
  conclusion?: string; regime?: string; advance?: number | null;
  close?: number | null; ids?: Array<string | null> | null; status?: string;
  account?: string | null;
} = {}): LiveArtifact {
  return {
    research_artifact: {
      symbol: "600519.SH",
      synthesis: { summary: input.conclusion ?? "缺乏充分证据" },
      market: { regime: input.regime ?? "MIXED", advance_count: input.advance === undefined ? 0 : input.advance, as_of: "20261009" },
      technical: {
        symbol: "600519.SH", requested_as_of: "20261011", effective_as_of: "20261009",
        last_trade_date: "20261009", status: input.status ?? "NORMAL",
        snapshot: input.close === null ? null : {
          open: 100, high: 110, low: 90, close: input.close ?? 100,
        },
        moving_averages: { ma5: 104.5 }, momentum: { rsi14: 0, macd: { hist: 0 } },
        returns: {}, volume: {},
      },
      event: {
        symbol: "600519.SH", status: "NO_EVENT",
        ...(input.ids === null ? {} : { events: (input.ids ?? []).map(id => ({ id, headline: "fixture" })) }),
      },
    },
    portfolio_review: {
      research_plan: { symbol: "600519.SH", conclusion: "INSUFFICIENT_EVIDENCE" },
      risk_context: { assessment: "LIMITED", summary: "insufficient" },
      portfolio_decision: { portfolio_state: input.account === undefined ? "NOT_PROVIDED" : input.account },
    },
    final_risk_gate: { disposition: "REVIEW_REQUIRED" },
  };
}
const find = (value: ReturnType<typeof compareResearchRuns>, label: string) =>
  value?.rows.find(r => r.label === label);

describe("safe historical Run comparison", () => {
  it("compares two completed and same-symbol Runs with a typed zero, missing data and actual differences", () => {
    const result = compareResearchRuns(a, artifact({ advance: 0, close: 100, ids: ["old"] }), b,
      artifact({ regime: "RISK_OFF", advance: null, close: 104, ids: ["new", "old"] }));
    expect(result).not.toBeNull();
    expect(find(result, "市场状态")).toMatchObject({ kind: "different", left: "MIXED", right: "RISK_OFF" });
    expect(find(result, "上涨家数")).toMatchObject({ kind: "only_left", left: "0", right: null });
    expect(find(result, "收盘价（RAW）")).toMatchObject({ kind: "different", left: "100", right: "104" });
    expect(find(result, "RSI14")).toMatchObject({ kind: "same", left: "0", right: "0" });
    expect(result?.evidence).toMatchObject({ added: ["new"], removed: [] });
    expect(result?.changed).toBeGreaterThanOrEqual(2);
    expect(result?.missing).toBeGreaterThan(0);
  });

  it("does not invent disappearances when either event list is absent, malformed or ambiguous", () => {
    const noIds = compareResearchRuns(a, artifact({ids:null}), b, artifact({ids:["e"]}));
    expect(noIds?.evidence).toMatchObject({ left:null, right:["e"], added:null, removed:null });
    const duplicates = compareResearchRuns(a, artifact({ids:["e","e"]}), b, artifact({ids:["e"]}));
    expect(duplicates?.evidence).toMatchObject({ left:null, added:null, removed:null });
    const missing = compareResearchRuns(a, artifact({ids:[null]}), b, artifact({ids:["e"]}));
    expect(missing?.evidence.added).toBeNull();
  });

  it("treats two explicitly empty event lists as observed empty rather than unknown", () => {
    const r = compareResearchRuns(a, artifact({ids:[]}), b, artifact({ids:[]}));
    expect(r?.evidence).toMatchObject({ left:[], right:[], added:[], removed:[] });
  });

  it("rejects same ID, mixed security, mixed mode and every nonterminal run", () => {
    expect(compareResearchRuns(a, artifact(), a, artifact())).toBeNull();
    expect(compareResearchRuns(a, artifact(), makeRun("c","000001.SZ"), artifact())).toBeNull();
    expect(compareResearchRuns(a, artifact(), makeRun("c","600519.SH","DECISION"), artifact())).toBeNull();
    for (const state of ["PENDING","RUNNING","FAILED","CANCELLED"] as const) {
      expect(compareResearchRuns(a, artifact(), makeRun("c","600519.SH","PORTFOLIO_REVIEW",state), artifact())).toBeNull();
    }
  });

  it("rejects cross-symbol artifacts, missing research root and mismatched technical symbol", () => {
    const other = artifact();
    ((other.research_artifact as Record<string,unknown>).technical as Record<string,unknown>).symbol="000001.SZ";
    expect(compareResearchRuns(a, artifact(), b, other)).toBeNull();
    const bad = artifact();
    (bad.research_artifact as Record<string,unknown>).symbol="000001.SZ";
    expect(compareResearchRuns(a, artifact(), b, bad)).toBeNull();
    expect(compareResearchRuns(a, {}, b, artifact())).toBeNull();
  });

  it("hides unverified numerical technical values for NO_DATA and invalid OHLC", () => {
    const noData = compareResearchRuns(a, artifact({status:"NO_DATA"}), b, artifact());
    expect(find(noData,"收盘价（RAW）")).toMatchObject({kind:"only_right",left:null,right:"100"});
    expect(find(noData,"RSI14")).toMatchObject({kind:"only_right",left:null,right:"0"});
    const invalid = compareResearchRuns(a, artifact({close:200}), b, artifact());
    expect(find(invalid,"收盘价（RAW）")?.left).toBeNull();
  });

  it("compares plain Decision mode without pretending a portfolio exists", () => {
    const x: LiveArtifact = {symbol:"600519.SH",action:"WATCH",market_view:"RISK_OFF",evidence:["fixture"]};
    const y: LiveArtifact = {symbol:"600519.SH",action:"AVOID",market_view:"RISK_OFF"};
    const result = compareResearchRuns(makeRun("c","600519.SH","DECISION"),x,
      makeRun("d","600519.SH","DECISION"),y);
    expect(find(result,"研究动作（非委托）")).toMatchObject({kind:"different",left:"WATCH",right:"AVOID"});
    expect(result?.rows.some(r=>r.section==="风险门禁")).toBe(false);
    expect(result?.evidence.added).toBeNull();
  });
  it("compares recorded model configuration fingerprints, without inventing missing provenance", () => {
    const left = { ...a, model_provenance: { schema_version: 1, config_fingerprint: "a".repeat(64), bindings: [] } };
    const right = { ...b, model_provenance: { schema_version: 1, config_fingerprint: "b".repeat(64), bindings: [] } };
    const result = compareResearchRuns(left, artifact(), right, artifact());
    expect(find(result, "模型配置指纹")).toMatchObject({kind: "different", left: "a".repeat(64), right: "b".repeat(64)});
    const missing = compareResearchRuns(a, artifact(), right, artifact());
    expect(find(missing, "模型配置指纹")).toMatchObject({kind: "only_right", left: null, right: "b".repeat(64)});
  });

});
