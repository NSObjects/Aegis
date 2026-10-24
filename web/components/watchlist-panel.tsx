"use client";

import { useCallback, useEffect, useState } from "react";
import { Eye, Plus, RefreshCcw, Search, Trash2, TriangleAlert } from "lucide-react";
import { isWatchItem, type LiveState, type WatchItem } from "@/lib/live-contract";
import { cn } from "@/lib/utils";

const base = "/api/workbench";
const stateLabels: Record<LiveState, string> = {
  PENDING: "等待执行", RUNNING: "研究中", COMPLETED: "已完成", FAILED: "失败", CANCELLED: "已取消",
};
const stateColors: Record<LiveState, string> = {
  PENDING: "blue", RUNNING: "blue", COMPLETED: "green", FAILED: "red", CANCELLED: "gray",
};
const formatDate = (value: string | undefined) => value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";

async function watchRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, { cache: "no-store", credentials: "same-origin", ...init });
  if (r.status === 204) return undefined as T;
  if (!r.ok) {
    if (r.status === 401) throw Error("登录已过期，请重新登录");
    if (r.status === 409) {
      const body = await r.json().catch(() => ({}));
      throw Error(body.error?.includes("capacity") ? "观察池已满（上限 50 个标的）" : "该标的已在观察池中");
    }
    if (r.status === 503) throw Error("观察池服务不可用");
    throw Error(`请求失败（HTTP ${r.status}）`);
  }
  return r.json() as Promise<T>;
}

export default function WatchlistPanel({
  onOpenRun,
  onStartResearch,
  researching,
}: {
  onOpenRun: (runId: string) => void;
  onStartResearch: (symbol: string) => void;
  researching: boolean;
}) {
  const [items, setItems] = useState<WatchItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [symbol, setSymbol] = useState("");
  const [reason, setReason] = useState("");
  const [conditions, setConditions] = useState("");

  const refresh = useCallback(async () => {
    try {
      const data = await watchRequest<{ items: unknown[] }>(`${base}/watchlist`);
      setItems(Array.isArray(data.items) ? data.items.filter(isWatchItem) : []);
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);

  // Initial load mirrors the history view: settle asynchronously with a
  // cancelled flag instead of driving state synchronously inside the effect.
  useEffect(() => {
    let cancelled = false;
    watchRequest<{ items: unknown[] }>(`${base}/watchlist`)
      .then(data => { if (!cancelled) { setItems(Array.isArray(data.items) ? data.items.filter(isWatchItem) : []); setError(null); } })
      .catch(e => { if (!cancelled) { setItems([]); setError((e as Error).message); } });
    return () => { cancelled = true; };
  }, []);

  async function add(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = symbol.trim().toUpperCase();
    if (!/^[0-9]{6}\.(SH|SZ)$/.test(name)) { setError("股票代码必须为 6 位代码加 .SH 或 .SZ，例如 600519.SH"); return; }
    if (!reason.trim()) { setError("请填写加入观察的理由"); return; }
    setBusy(true); setError(null);
    try {
      await watchRequest(`${base}/watchlist`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ symbol: name, reason: reason.trim(), conditions: conditions.trim() }),
      });
      setSymbol(""); setReason(""); setConditions("");
      await refresh();
    } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  }

  async function remove(target: string) {
    setBusy(true); setError(null);
    try {
      await watchRequest(`${base}/watchlist/${target}`, { method: "DELETE" });
      await refresh();
    } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  }

  return <section className="surface live-history" aria-label="观察池">
    <div className="panel-head">
      <h2>观察池</h2>
      <div className="live-history-actions">
        <span className="badge blue">{items ? `${items.length} / 50` : "…"}</span>
        <button type="button" className="btn" onClick={() => void refresh()}><RefreshCcw size={14} />刷新</button>
      </div>
    </div>
    {error && <div role="alert" className="live-alert"><TriangleAlert size={15} />{error}</div>}
    <details className="live-create" open={!items || items.length === 0}>
      <summary>加入观察 <span>标的、理由与关注条件保存在服务端</span></summary>
      <div className="live-create-body">
        <form onSubmit={add}>
          <label>股票代码<input value={symbol} onChange={e => setSymbol(e.target.value.toUpperCase())} placeholder="600519.SH" required /></label>
          <label>观察理由（必填）<input maxLength={1024} value={reason} onChange={e => setReason(e.target.value)} placeholder="例如：回调至前低附近，等待企稳" required /></label>
          <label>关注条件（可选）<input maxLength={1024} value={conditions} onChange={e => setConditions(e.target.value)} placeholder="例如：跌破 MA20 或放量突破前高时复查" /></label>
          <div className="live-create-foot">
            <button className="btn primary" type="submit" disabled={busy}><Plus size={15} />{busy ? "处理中…" : "加入观察池"}</button>
          </div>
        </form>
      </div>
    </details>
    {items === null && !error ? <p className="live-empty">正在读取服务端观察池…</p>
      : items !== null && items.length === 0 ? <p className="live-empty">观察池为空。加入标的后可从这里直接启动研究。</p>
      : items !== null ? <div className="live-history-table">{items.map(item => (
        <div key={item.symbol} className="watch-row">
          <div className="watch-main">
            <strong>{item.symbol}</strong>
            <span className="watch-reason">{item.reason}</span>
            {item.conditions && <span className="watch-conditions">条件：{item.conditions}</span>}
            <small>加入于 {formatDate(item.created_at)}</small>
          </div>
          <div className="watch-side">
            {item.latest_run ? (
              <button type="button" className="watch-run-link" onClick={() => onOpenRun(item.latest_run!.run_id)}>
                <span>最近研究</span>
                <span className={cn("badge", stateColors[item.latest_run.state])}>{stateLabels[item.latest_run.state]}</span>
                <small>{formatDate(item.latest_run.created_at)}</small>
              </button>
            ) : <span className="watch-run-link none"><span>最近研究</span><small>尚无研究记录</small></span>}
            <button type="button" className="btn" disabled={researching} onClick={() => onStartResearch(item.symbol)}><Search size={14} />启动研究</button>
            <button type="button" className="btn danger" disabled={busy} onClick={() => void remove(item.symbol)} aria-label={`移除 ${item.symbol}`}><Trash2 size={14} />移除</button>
          </div>
        </div>))}</div> : null}
    <p className="watch-footnote"><Eye size={13} />观察池仅保存操作者自填的理由与条件，重启后保留；不代表任何研究结论。</p>
  </section>;
}
