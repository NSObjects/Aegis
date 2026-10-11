/**
 * Go source: internal/agent/technical/context.go. This is a read-only
 * projection of ONE research-time Snapshot, not an OHLCV time series.
 *
 * Reject mismatched identity, invalid date cutoffs and malformed OHLC bars
 * instead of plotting visually plausible but invented financial data.
 */
export type Scalar = { label: string; key: string; value: number; unit?: string };
export type Candle = { open: number; high: number; low: number; close: number; preClose: number | null; changePercent: number | null };
export type TechnicalVisual = {
  status: string;
  symbol: string;
  requested: string | null;
  effective: string | null;
  candle: Candle | null;
  unavailable: string | null;
  movingAverages: Scalar[];
  momentum: Scalar[];
  returns: Scalar[];
  volumes: Scalar[];
};

type Obj = Record<string, unknown>;
const obj = (x: unknown): Obj | null =>
  x !== null && typeof x === "object" && !Array.isArray(x) ? x as Obj : null;
const str = (x: unknown): string | null => typeof x === "string" && x.trim() !== "" ? x : null;
const finite = (x: unknown): number | null => typeof x === "number" && Number.isFinite(x) ? x : null;
const tradingDate = (x: unknown): string | null => {
  if (typeof x !== "string" || !/^\d{8}$/.test(x)) return null;
  const yy = +x.slice(0, 4), mm = +x.slice(4, 6), dd = +x.slice(6, 8);
  const d = new Date(Date.UTC(yy, mm - 1, dd));
  return d.getUTCFullYear() === yy && d.getUTCMonth() + 1 === mm && d.getUTCDate() === dd ? x : null;
};

function metrics(raw: unknown, specs: Array<[string, string, string?]>, min: number | null = null, max: number | null = null): Scalar[] {
  const source = obj(raw);
  if (!source) return [];
  return specs.flatMap(([key, label, unit]) => {
    const n = finite(source[key]);
    return n === null || (min !== null && n < min) || (max !== null && n > max)
      ? [] : [{ key, label, value: n, ...(unit ? { unit } : {}) }];
  });
}

export function readTechnicalVisual(artifact: unknown, run: { symbol: string; requested_as_of: string }): TechnicalVisual | null {
  const root = obj(artifact);
  const research = root && obj(root.research_artifact);
  const technical = research && obj(research.technical);
  if (!technical) return null;
  // Reject wrong-Run data even if a valid-looking candle is present.
  if (str(research?.symbol) !== run.symbol || str(technical.symbol) !== run.symbol) return null;
  const status = str(technical.status) ?? "UNKNOWN";
  const requested = tradingDate(technical.requested_as_of);
  const effective = tradingDate(technical.effective_as_of);
  const requestTime = new Date(run.requested_as_of);
  const runDate = !Number.isNaN(requestTime.getTime())
    ? requestTime.toISOString().slice(0, 10).replaceAll("-", "") : null;
  const chronologyValid = !!requested && !!effective && !!runDate &&
    effective <= requested && requested <= runDate &&
    (!str(technical.last_trade_date) || tradingDate(technical.last_trade_date) === effective);
  const result: TechnicalVisual = {
    status, symbol: run.symbol, requested, effective, candle: null, unavailable: null,
    movingAverages: [], momentum: [], returns: [], volumes: [],
  };
  if (status !== "NORMAL") {
    result.unavailable = "后端技术数据状态：" + status + "。未返回可绘制的有效单日行情。";
    return result;
  }
  if (!chronologyValid) {
    result.unavailable = "研究时间、有效交易日或 Run 截止时间不一致，拒绝绘制价格。";
    return result;
  }
  const raw = obj(technical.snapshot);
  const o = finite(raw?.open), h = finite(raw?.high), l = finite(raw?.low), c = finite(raw?.close);
  if (o === null || h === null || l === null || c === null ||
    o <= 0 || h <= 0 || l <= 0 || c <= 0 || h < l ||
    h < Math.max(o, c) || l > Math.min(o, c)) {
    result.unavailable = "OHLC 缺失或开高低收关系无效，拒绝生成 K 线。";
    return result;
  }
  result.candle = {
    open: o, high: h, low: l, close: c,
    preClose: finite(raw?.pre_close), changePercent: finite(raw?.change_percent),
  };
  // Go technical researcher currently calculates PIT_QFQ technical metrics from
  // adjusted close series but stores only RAW latest OHLC. Do NOT overlay these
  // MAs onto the RAW candle or assert identical source/adjustment basis.
  result.movingAverages = metrics(technical.moving_averages, [
    ["ma5", "MA5"], ["ma10", "MA10"], ["ma20", "MA20"], ["ma60", "MA60"],
  ], 0).filter(x => x.value > 0);
  const momentum = obj(technical.momentum);
  result.momentum = [
    ...metrics(momentum, [["rsi14", "RSI14"]], 0, 100),
    ...metrics(momentum?.macd, [["dif", "MACD DIF"], ["dea", "MACD DEA"], ["hist", "MACD HIST"]]),
  ];
  result.returns = metrics(technical.returns, [
    ["return_1d", "1 日"], ["return_5d", "5 日"], ["return_10d", "10 日"],
    ["return_20d", "20 日"], ["return_60d", "60 日"],
  ]);
  result.volumes = [
    ...metrics(raw, [["volume", "当日成交量"], ["amount", "当日成交额"], ["turnover_rate", "换手率", "%"]], 0),
    ...metrics(technical.volume, [
      ["volume_ma5", "成交量 MA5"], ["volume_ma20", "成交量 MA20"],
      ["volume_vs_ma5", "量比 MA5", "×"], ["volume_vs_ma20", "量比 MA20", "×"],
      ["amount_vs_ma5", "成交额比 MA5", "×"], ["amount_vs_ma20", "成交额比 MA20", "×"],
    ], 0),
  ];
  return result;
}
