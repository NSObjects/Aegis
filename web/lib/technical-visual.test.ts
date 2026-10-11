import { describe, expect, it } from "vitest";
import { readTechnicalVisual } from "./technical-visual";

const run = { symbol: "600519.SH", requested_as_of: "2026-10-11T08:00:00Z" };
const source = {
  symbol: run.symbol,
  requested_as_of: "20261011",
  effective_as_of: "20261009",
  last_trade_date: "20261009",
  status: "NORMAL",
  snapshot: { open: 1500, high: 1530, low: 1480, close: 1500, pre_close: 1490,
    change_percent: 0, volume: 0, amount: 98000000, turnover_rate: 0 },
  returns: { return_1d: 0, return_5d: -2.2, return_20d: null },
  moving_averages: { ma5: 1494.3, ma10: 1504.1, ma20: null, ma60: 1450.2 },
  momentum: { rsi14: 45, macd: { dif: -1.12, dea: -0.3, hist: -1.64 } },
  volume: { volume_ma5: 105, volume_vs_ma5: 0, volume_vs_ma20: null },
};
const wrap = (technical: unknown, researchSymbol = run.symbol) => ({
  research_artifact: { symbol: researchSymbol, technical },
});

describe("typed Go technical visualization (one trading-day only)", () => {
  it("displays original OHLC exactly, preserving observed zeros and all cutoffs", () => {
    const result = readTechnicalVisual(wrap(source), run);
    expect(result).not.toBeNull();
    expect(result?.status).toBe("NORMAL");
    expect(result?.requested).toBe("20261011");
    expect(result?.effective).toBe("20261009");
    expect(result?.candle).toEqual({ open: 1500, high: 1530, low: 1480, close: 1500,
      preClose: 1490, changePercent: 0 });
    expect(result?.volumes).toContainEqual({ key: "volume", label: "当日成交量", value: 0 });
    expect(result?.volumes).toContainEqual({ key: "volume_vs_ma5", label: "量比 MA5", value: 0, unit: "×" });
    expect(result?.momentum).toContainEqual({ key: "hist", label: "MACD HIST", value: -1.64 });
    expect(result?.returns).toContainEqual({ key: "return_1d", label: "1 日", value: 0 });
  });

  it("omits missing values rather than converting absent MA/returns to 0", () => {
    const result = readTechnicalVisual(wrap(source), run)!;
    expect(result.movingAverages.map(v => v.key)).toEqual(["ma5", "ma10", "ma60"]);
    expect(result.returns.map(v => v.key)).toEqual(["return_1d", "return_5d"]);
    expect(result.volumes.some(v => v.key === "volume_vs_ma20")).toBe(false);
  });

  it("refuses to draw if reported dates are later than study cutoff or self-inconsistent", () => {
    for (const broken of [
      { ...source, effective_as_of: "20261012" },
      { ...source, requested_as_of: "20261012" },
      { ...source, effective_as_of: "20261008" }, // no longer matches last_trade_date
      { ...source, requested_as_of: "20261301" },
    ]) {
      const result = readTechnicalVisual(wrap(broken), run);
      expect(result?.candle).toBeNull();
      expect(result?.unavailable).toContain("不一致");
    }
  });

  it("rejects bad OHLC geometry and missing high/low, never fabricates a candle", () => {
    for (const snapshot of [
      { ...source.snapshot, high: 1490 },
      { ...source.snapshot, low: 1510 },
      { ...source.snapshot, close: 0 },
      { ...source.snapshot, open: null },
      { ...source.snapshot, high: Infinity },
    ]) {
      const result = readTechnicalVisual(wrap({ ...source, snapshot }), run);
      expect(result?.candle).toBeNull();
      expect(result?.unavailable).toContain("OHLC");
      expect(result?.momentum).toEqual([]);
    }
  });

  it("never mixes another security's completed artifact with the currently selected Run", () => {
    expect(readTechnicalVisual(wrap(source, "000001.SZ"), run)).toBeNull();
    expect(readTechnicalVisual(wrap({ ...source, symbol: "000001.SZ" }), run)).toBeNull();
    expect(readTechnicalVisual({}, run)).toBeNull();
    expect(readTechnicalVisual(null, run)).toBeNull();
  });

  it("honors NO_DATA, SUSPENDED and nullable technical context without plotting zero prices", () => {
    for (const status of ["NO_DATA", "SUSPENDED", "DELISTED"]) {
      const result = readTechnicalVisual(wrap({ ...source, status, snapshot: null }), run);
      expect(result?.status).toBe(status);
      expect(result?.candle).toBeNull();
      expect(result?.unavailable).toContain(status);
      expect(result?.volumes).toEqual([]);
    }
    expect(readTechnicalVisual(wrap(null), run)).toBeNull();
  });

  it("handles flat valid bars and ignores nonsensical source indicator values", () => {
    const result = readTechnicalVisual(wrap({
      ...source, snapshot: { ...source.snapshot, open: 1500, high: 1500, low: 1500, close: 1500 },
      momentum: { rsi14: 110, macd: { hist: 0 } },
      moving_averages: { ma5: -2, ma20: 1400 },
    }), run);
    expect(result?.candle).not.toBeNull();
    expect(result?.momentum).toEqual([{ key: "hist", label: "MACD HIST", value: 0 }]);
    expect(result?.movingAverages).toEqual([{ key: "ma20", label: "MA20", value: 1400 }]);
  });
});
