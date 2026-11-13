"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowRight, Database, Eye, Plus, RefreshCcw, Search, TriangleAlert } from "lucide-react";
import { isAshareSymbol, isRunID } from "@/lib/live-contract";

const base = "/api/workbench";
type ScanState = "PENDING" | "RUNNING" | "COMPLETED" | "FAILED";
type Verdict = "SELECTED" | "REJECTED" | "INSUFFICIENT_DATA";
type ScanResult = { symbol: string; verdict: Verdict; reason: string };
type ScanRecord = {
  scan_id: string; as_of: string; strategy_version: string; state: ScanState;
  universe_source: string; symbols: string[]; created_at: string;
  updated_at: string; failure_code?: string; results?: ScanResult[];
};
type Source = { sha256: string; bars_count: number; effective_as_of: string };
type Ranked = {
  rank: number; symbol: string; source: Source;
  metrics: { close: number; ma20: number; ma60: number; ma20_over_ma60: number; close_over_ma20: number; ma60_slope_5d: number };
};
type Incomparable = { symbol: string; verdict: Verdict; reason: string; note: string; source?: Source };
type Comparison = {
  scan_id: string; as_of: string; comparison_version: string;
  ranking_rule: string; ranked: Ranked[]; incomparable: Incomparable[];
};

const verdicts = new Set(["SELECTED", "REJECTED", "INSUFFICIENT_DATA"]);
const states = new Set(["PENDING", "RUNNING", "COMPLETED", "FAILED"]);
const reasonLabels: Record<string, string> = {
  TREND_PULLBACK_V1: "满足趋势回踩条件", TREND_NOT_UP: "上升趋势条件不满足",
  NOT_NEAR_MA20: "距离 MA20 不在策略区间", MISSING_BARS: "K 线数据不足",
  METRICS_INSUFFICIENT_BARS: "不足 65 条有效日线", MISSING_PIT_ADJUSTMENT: "缺少时点复权因子",
  INVALID_PIT_ADJUSTMENT: "时点复权因子无效", HALTED_OR_NO_VOLUME: "停牌或无成交量",
  STALE_OR_MISSING_CLOSE: "收盘价缺失或过期", OUTSIDE_STOCK_UNIVERSE: "不在策略支持的股票范围",
  UNSUPPORTED_PRICE_BASIS: "价格口径不支持", NO_COMMITTED_SNAPSHOT: "无可核验快照",
  NO_VALID_HISTORICAL_OBSERVATION: "未取得有效历史观察", NOT_SELECTED: "未通过选股条件",
  INSUFFICIENT_DATA: "数据不足", PROCESS_INTERRUPTED: "服务重启导致扫描中断",
};
const textReason = (reason: string) => reasonLabels[reason] ?? reason;
const statusLabel = (state: ScanState) =>
  ({ PENDING: "等待执行", RUNNING: "扫描中", COMPLETED: "已完成", FAILED: "失败" })[state];
const dateLabel = (value: string) => /^\d{8}$/.test(value) ? `${value.slice(0,4)}-${value.slice(4,6)}-${value.slice(6,8)}` : value;
const chinaToday = () => new Date().toLocaleDateString("sv-SE", { timeZone: "Asia/Shanghai" }).replaceAll("-", "");
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const isResult = (v: unknown): v is ScanResult =>
  isObj(v) && typeof v.symbol === "string" && isAshareSymbol(v.symbol) &&
  typeof v.verdict === "string" && verdicts.has(v.verdict) && typeof v.reason === "string";
function isScan(v: unknown): v is ScanRecord {
  return isObj(v) && typeof v.scan_id === "string" && isRunID(v.scan_id) &&
    typeof v.as_of === "string" && /^\d{8}$/.test(v.as_of) &&
    typeof v.strategy_version === "string" && typeof v.state === "string" && states.has(v.state) &&
    v.universe_source === "USER_SUPPLIED" && Array.isArray(v.symbols) &&
    v.symbols.length >= 1 && v.symbols.length <= 20 && v.symbols.every(s => typeof s === "string" && isAshareSymbol(s)) &&
    typeof v.created_at === "string" && Number.isFinite(Date.parse(v.created_at)) &&
    (v.results === undefined || (Array.isArray(v.results) && v.results.every(isResult)));
}
const isSource = (v: unknown): v is Source =>
  isObj(v) && typeof v.sha256 === "string" && /^[a-f0-9]{64}$/.test(v.sha256) &&
  typeof v.bars_count === "number" && Number.isSafeInteger(v.bars_count) &&
  typeof v.effective_as_of === "string";
function isComparison(v: unknown): v is Comparison {
  return isObj(v) && typeof v.scan_id === "string" && isRunID(v.scan_id) &&
    typeof v.as_of === "string" && typeof v.comparison_version === "string" &&
    typeof v.ranking_rule === "string" && Array.isArray(v.ranked) &&
    v.ranked.every((r: unknown) => isObj(r) && typeof r.rank === "number" &&
      typeof r.symbol === "string" && isAshareSymbol(r.symbol) && isSource(r.source) &&
      isObj(r.metrics) && ["close","ma20","ma60","ma20_over_ma60","close_over_ma20","ma60_slope_5d"].every(k =>
        typeof (r.metrics as Record<string, unknown>)[k] === "number" && Number.isFinite((r.metrics as Record<string, number>)[k]))) &&
    Array.isArray(v.incomparable) && v.incomparable.every((r: unknown) =>
      isObj(r) && typeof r.symbol === "string" && isAshareSymbol(r.symbol) &&
      typeof r.reason === "string" && typeof r.note === "string" &&
      typeof r.verdict === "string" && verdicts.has(r.verdict) &&
      (r.source === undefined || isSource(r.source)));
}
async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin", cache: "no-store", ...init });
  if (!res.ok) {
    if (res.status === 401) throw Error("登录已失效，请重新登录");
    if (res.status === 503) throw Error("扫描服务不可用：请确认 Go API 已配置 TUSHARE_TOKEN 并重启服务");
    if (res.status === 409) throw Error("已有扫描正在执行，请完成后重试");
    if (res.status === 404) throw Error("扫描记录不存在或已被删除");
    throw Error(`扫描接口返回 HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}
function validAsOf(value: string): boolean {
  if (!/^\d{8}$/.test(value) || value > chinaToday()) return false;
  const year = Number(value.slice(0, 4)), month = Number(value.slice(4, 6)), day = Number(value.slice(6, 8));
  return new Date(Date.UTC(year, month - 1, day)).toISOString().slice(0, 10).replaceAll("-", "") === value;
}
const num = (value: number, digits = 2) => Number.isFinite(value) ? value.toFixed(digits) : "—";

export default function StrategyScanPanel({
  onStartResearch, onOpenWatchlist, researching,
}: {
  onStartResearch: (symbol: string) => void;
  onOpenWatchlist: () => void;
  researching: boolean;
}) {
  const [history, setHistory] = useState<ScanRecord[] | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<{ id: string; scan: ScanRecord } | null>(null);
  const [comp, setComp] = useState<{ id: string; value: Comparison } | null>(null);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [historyBusy, setHistoryBusy] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [watchSaving, setWatchSaving] = useState<string | null>(null);
  const [saved, setSaved] = useState<string[]>([]);
  const [symbolsText, setSymbolsText] = useState("");
  const [asOf, setAsOf] = useState(chinaToday);
  const [watchReason, setWatchReason] = useState("");

  const refresh = useCallback(async () => {
    setHistoryBusy(true);
    try {
      const result = await request<{ scans: unknown[] }>(`${base}/scans?limit=20`);
      if (!Array.isArray(result.scans) || !result.scans.every(isScan)) throw Error("扫描历史返回格式无效");
      setHistory(result.scans);
      setSelectedId(current => current ?? result.scans[0]?.scan_id ?? null);
      setError("");
    } catch (e) {
      setError((e as Error).message);
    } finally { setHistoryBusy(false); }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  const selectedFromHistory = useMemo(() => history?.find(s => s.scan_id === selectedId) ?? null, [history, selectedId]);
  const selected = detail?.id === selectedId ? detail.scan : selectedFromHistory;
  const comparison = comp?.id === selectedId ? comp.value : null;

  useEffect(() => {
    if (!selectedId) return;
    let disposed = false;
    const load = async () => {
      try {
        const record: unknown = await request(`${base}/scans/${selectedId}`);
        if (!isScan(record) || record.scan_id !== selectedId) throw Error("扫描详情数据不完整");
        if (!disposed) setDetail({ id: selectedId, scan: record });
        if (record.state !== "COMPLETED") return;
        const result: unknown = await request(`${base}/scans/${selectedId}/comparison`);
        if (!isComparison(result) || result.scan_id !== selectedId || result.as_of !== record.as_of)
          throw Error("扫描比较结果校验失败，暂不显示候选排名");
        if (!disposed) setComp({ id: selectedId, value: result });
      } catch (e) { if (!disposed) setError((e as Error).message); }
    };
    void load();
    // Active Go scans survive navigation; poll persisted state until terminal.
    const timer = selected?.state === "PENDING" || selected?.state === "RUNNING"
      ? window.setInterval(() => { void load(); }, 3000) : null;
    return () => { disposed = true; if (timer !== null) window.clearInterval(timer); };
  }, [selectedId, selected?.state]);

  async function start(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const symbols = [...new Set(symbolsText.trim().toUpperCase().split(/[\s,，;；]+/).filter(Boolean))];
    if (symbols.length < 1 || symbols.length > 20 || symbols.some(s => !isAshareSymbol(s))) {
      setError("请输入 1–20 个完整 A 股代码（如 600519.SH），以逗号或空格分隔");
      return;
    }
    if (!validAsOf(asOf)) { setError("请填写有效且不晚于今天的 YYYYMMDD 日期"); return; }
    setError(""); setInfo(""); setSubmitting(true);
    try {
      const record: unknown = await request(`${base}/scans`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ as_of: asOf, universe_source: "USER_SUPPLIED", symbols }),
      });
      if (!isScan(record)) throw Error("后端已接收请求，但未返回有效扫描 ID；请刷新历史记录核对");
      setDetail({ id: record.scan_id, scan: record });
      setComp(null);
      setSelectedId(record.scan_id);
      setHistory(list => [record, ...(list ?? []).filter(x => x.scan_id !== record.scan_id)].slice(0, 20));
      setInfo(`已提交扫描 ${record.scan_id.slice(0, 12)}…；使用服务端真实行情数据，不调用 LLM。`);
    } catch (e) { setError((e as Error).message); }
    finally { setSubmitting(false); }
  }

  async function addWatch(symbol: string) {
    if (!selected || !comparison || !comparison.ranked.some(r => r.symbol === symbol)) return;
    if (watchReason.trim().length < 3) { setError("加入观察池前，请先填写至少 3 字的个人观察理由"); return; }
    const reason = `${watchReason.trim()}（策略扫描 ${selected.as_of}，ID ${selected.scan_id.slice(0, 12)}）`;
    setError(""); setInfo(""); setWatchSaving(symbol);
    try {
      await request(`${base}/watchlist`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ symbol, reason, conditions: "" }),
      });
      setSaved(items => [...items, symbol]);
      setInfo(`${symbol} 已加入服务端观察池，可继续在观察池中管理与研究。`);
    } catch (e) {
      if ((e as Error).message.includes("409")) setError(`${symbol} 已在观察池，请从观察池查看。`);
      else setError(`保存观察池失败：${(e as Error).message}`);
    } finally { setWatchSaving(null); }
  }

  return <div className="scan-workspace">
    <section className="surface scan-create" aria-label="指定股票范围策略扫描">
      <div className="panel-head"><div><h2>策略扫描 · 趋势回踩 V1</h2><p>对手动指定的股票范围进行确定性筛选，不是全市场自动选股。</p></div><span className="badge blue">不调用 LLM</span></div>
      <form onSubmit={start} className="scan-form">
        <label>股票代码（1–20 只）
          <textarea value={symbolsText} onChange={e => setSymbolsText(e.target.value)} rows={3}
            placeholder="600519.SH, 000001.SZ" maxLength={250} required />
        </label>
        <label>行情截止日（YYYYMMDD）
          <input value={asOf} onChange={e => setAsOf(e.target.value)} maxLength={8} inputMode="numeric" placeholder="20261113" required />
        </label>
        <button type="submit" className="btn primary" disabled={submitting}>
          <Search size={14} />{submitting ? "提交中…" : "开始扫描"}
        </button>
      </form>
      <p className="scan-disclaimer">仅支持 Go 后端当前 V1 的上升趋势 MA20/MA60 与回踩规则，行情源需要 TuShare。非交易日或数据/复权信息不足会明确列为不可比较；结果不构成买卖指令。</p>
    </section>

    {error && <div className="live-alert" role="alert"><TriangleAlert size={15} />{error}<button type="button" onClick={() => setError("")}>关闭</button></div>}
    {info && <div className="live-info" role="status">{info}</div>}

    <section className="surface scan-history">
      <div className="panel-head"><div><h2>扫描记录</h2><p>最多展示最近 20 条服务端任务；状态与结果可在重启后查询。</p></div>
        <button type="button" className="btn" disabled={historyBusy} onClick={() => void refresh()}><RefreshCcw size={14}/>{historyBusy ? "刷新中…" : "刷新"}</button>
      </div>
      {history === null ? <p className="live-empty">正在连接扫描服务…</p> :
        history.length === 0 ? <p className="live-empty">尚无扫描记录。填写股票代码后开始第一次扫描。</p> :
        <div className="scan-history-list">{history.map(scan =>
          <button key={scan.scan_id} type="button" className={scan.scan_id === selectedId ? "scan-history-row selected" : "scan-history-row"}
            onClick={() => { setSelectedId(scan.scan_id); setComp(null); setSaved([]); setError(""); }}>
            <span><strong>{dateLabel(scan.as_of)}</strong><small>{scan.symbols.length} 只 · {scan.strategy_version}</small></span>
            <span className={`badge ${scan.state === "FAILED" ? "red" : scan.state === "COMPLETED" ? "green" : "blue"}`}>{statusLabel(scan.state)}</span><ArrowRight size={14}/>
          </button>)}</div>}
    </section>

    {selected && <section className="surface scan-results" aria-label="扫描结果">
      <div className="panel-head"><div><h2>扫描结果 · {dateLabel(selected.as_of)}</h2>
        <p>策略版本 {selected.strategy_version} · 来源 USER_SUPPLIED · ID <code>{selected.scan_id.slice(0, 12)}…</code></p></div>
        <span className={`badge ${selected.state === "FAILED" ? "red" : selected.state === "COMPLETED" ? "green" : "blue"}`}>{statusLabel(selected.state)}</span>
      </div>
      {selected.state === "FAILED" ? <p className="scan-result-message" role="alert">扫描失败：{selected.failure_code ? textReason(selected.failure_code) : "后端未提供失败代码"}。没有可用候选名单。</p>
      : selected.state !== "COMPLETED" ? <p className="scan-result-message">扫描运行中；后端完成后自动读取持久化结果。刷新页面不会重新提交扫描。</p>
      : !comparison ? <p className="scan-result-message">正在核验候选排名和输入快照；未校验完成前不展示股票池。</p> : <>
        <div className="scan-stats"><span><strong>{comparison.ranked.length}</strong> 只通过</span><span><strong>{comparison.incomparable.length}</strong> 只未参与排名</span><span>统一截止日：{dateLabel(comparison.as_of)}</span></div>
        {comparison.ranked.length === 0 ? <p className="scan-result-message">当前扫描范围没有满足该策略的候选股票；请查看下方排除和数据不足原因。</p> : <>
          <label className="scan-watch-reason">加入观察池前填写理由（人工备注）
            <input value={watchReason} onChange={e => setWatchReason(e.target.value)} maxLength={800}
              placeholder="例如：观察回踩后是否放量企稳" />
          </label>
          <div className="scan-ranked-list">{comparison.ranked.map(item =>
            <div className="scan-rank-row" key={item.symbol}>
              <strong className="scan-rank">{item.rank}</strong>
              <div className="scan-rank-main"><strong>{item.symbol}</strong>
                <small>收盘 {num(item.metrics.close)} · MA20 {num(item.metrics.ma20)} · MA60 {num(item.metrics.ma60)}</small>
                <small>距 MA20 {(100 * (item.metrics.close_over_ma20 - 1)).toFixed(2)}% · MA20/MA60 {(100 * (item.metrics.ma20_over_ma60 - 1)).toFixed(2)}%</small>
                <a href={`${base}/scans/${selected.scan_id}/results/${item.symbol}/snapshot`} target="_blank" rel="noopener noreferrer" title={item.source.sha256}>
                  <Database size={12}/>查看行情快照（{item.source.bars_count} 条）
                </a>
              </div>
              <div className="scan-rank-actions">
                <button type="button" className="btn" disabled={watchSaving !== null || saved.includes(item.symbol)}
                  onClick={() => void addWatch(item.symbol)}><Plus size={13}/>{saved.includes(item.symbol) ? "已保存" : "加入观察"}</button>
                <button type="button" className="btn primary" disabled={researching} onClick={() => onStartResearch(item.symbol)}>
                  <Search size={13}/>研究该股</button>
              </div>
            </div>)}</div>
          <p className="scan-disclaimer">候选按接近 MA20 的程度、趋势强度排序；排名不是收益预测。点击“研究该股”将启动单独的 Go/Eino 研究任务，可能产生模型费用。</p>
          <button className="btn" type="button" onClick={onOpenWatchlist}><Eye size={14}/>查看观察池</button>
        </>}
        {comparison.incomparable.length > 0 && <details className="scan-excluded" open={comparison.ranked.length === 0}>
          <summary>排除与数据不足（{comparison.incomparable.length}）</summary>
          <div>{comparison.incomparable.map(item => <div className="scan-excluded-row" key={item.symbol}>
            <strong>{item.symbol}</strong><span>{item.verdict === "REJECTED" ? "规则未通过" : "无法比较"}</span>
            <span>{textReason(item.reason)} · {textReason(item.note)}</span></div>)}</div>
        </details>}
      </>}
    </section>}
  </div>;
}
