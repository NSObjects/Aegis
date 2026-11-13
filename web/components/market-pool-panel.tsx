"use client";

import { useCallback, useEffect, useState } from "react";
import { ArrowRight, Database, Pause, PlayCircle, Plus, RefreshCcw, Search, TriangleAlert } from "lucide-react";
import { isAshareSymbol, isRunID } from "@/lib/live-contract";

const base = "/api/workbench";
type PoolState = "RUNNING" | "PAUSED" | "FAILED" | "COMPLETED";
type PoolSummary = {
  pool_id: string; as_of: string; state: PoolState; strategy_version: string;
  universe_source: string; universe_sha256: string; universe_count: number;
  next_offset: number; cache_days_done?: number; cache_days_total?: number; failure_code?: string;
};
type PoolRecord = Omit<PoolSummary, "universe_count"> & { symbols: string[]; active_scan_id?: string };
type PoolCandidate = {
  rank: number; symbol: string; scan_id: string;
  source: { sha256: string; bars_count: number; effective_as_of: string };
  metrics: { close: number; ma20: number; ma60: number; close_over_ma20: number; ma20_over_ma60: number };
};
type PoolComparison = {
  pool_id: string; as_of: string; universe_sha256: string; universe_count: number;
  selected_count: number; incomparable_count: number; reason_counts: Record<string, number>;
  offset: number; limit: number; ranked: PoolCandidate[];
};
const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const poolStates = new Set(["RUNNING","PAUSED","FAILED","COMPLETED"]);
function isSummary(v: unknown): v is PoolSummary {
  return isObj(v) && typeof v.pool_id === "string" && isRunID(v.pool_id) &&
    typeof v.as_of === "string" && /^\d{8}$/.test(v.as_of) &&
    typeof v.state === "string" && poolStates.has(v.state) &&
    v.universe_source === "TUSHARE_STOCK_BASIC_L_D_P" &&
    typeof v.universe_sha256 === "string" && /^[a-f0-9]{64}$/.test(v.universe_sha256) &&
    typeof v.universe_count === "number" && v.universe_count > 0 &&
    typeof v.next_offset === "number" && v.next_offset >= 0 && v.next_offset <= v.universe_count;
}
function isRecord(v: unknown): v is PoolRecord {
  return isObj(v) && Array.isArray(v.symbols) && v.symbols.every(x => typeof x === "string" && isAshareSymbol(x)) &&
    isSummary({ ...v, universe_count: v.symbols.length });
}
function isComparison(v: unknown): v is PoolComparison {
  return isObj(v) && typeof v.pool_id === "string" && isRunID(v.pool_id) &&
    typeof v.as_of === "string" && Array.isArray(v.ranked) &&
    typeof v.selected_count === "number" && typeof v.incomparable_count === "number" &&
    typeof v.universe_count === "number" &&
    isObj(v.reason_counts) &&
    v.ranked.every((r: unknown) => isObj(r) && typeof r.symbol === "string" && isAshareSymbol(r.symbol) &&
      typeof r.scan_id === "string" && isRunID(r.scan_id) && typeof r.rank === "number" &&
      isObj(r.source) && typeof r.source.sha256 === "string" &&
      isObj(r.metrics) && ["close", "ma20", "ma60", "close_over_ma20", "ma20_over_ma60"].every(k =>
        typeof (r.metrics as Record<string,unknown>)[k] === "number" &&
        Number.isFinite((r.metrics as Record<string, number>)[k])));
}
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base}${path}`, { ...init, credentials: "same-origin", cache: "no-store" });
  if (!res.ok) {
    if (res.status === 401) throw Error("会话已过期，请重新登录");
    if (res.status === 503) throw Error("全市场任务服务不可用：请在 Go 服务端配置 TUSHARE_TOKEN");
    if (res.status === 409) throw Error("任务正在执行或当前状态不允许此操作");
    if (res.status === 422) throw Error("截止日不是交易日，请选择已结束的交易日");
    throw Error(`请求失败（HTTP ${res.status}）`);
  }
  return res.json() as Promise<T>;
}
const nowShanghai = () => new Date().toLocaleDateString("sv-SE",{timeZone:"Asia/Shanghai"}).replaceAll("-","");
function validDate(s: string): boolean {
  if (!/^\d{8}$/.test(s)||s>nowShanghai()) return false;
  const y=+s.slice(0,4),m=+s.slice(4,6),d=+s.slice(6,8);
  return new Date(Date.UTC(y,m-1,d)).toISOString().slice(0,10).replaceAll("-","")===s;
}
const fmtDate = (s: string) => s.length===8 ? `${s.slice(0,4)}-${s.slice(4,6)}-${s.slice(6,8)}` : s;
const label: Record<PoolState,string> = { RUNNING:"扫描中",PAUSED:"已暂停",FAILED:"批次失败",COMPLETED:"全部完成" };
const n = (x: number) => Number.isFinite(x)?x.toFixed(2):"—";

export default function MarketPoolPanel({
  onStartResearch, onOpenWatchlist, researching,
}: {
  onStartResearch: (symbol: string) => void; onOpenWatchlist: () => void; researching: boolean;
}) {
  const [history,setHistory] = useState<PoolSummary[]|null>(null);
  const [selectedId,setSelectedId] = useState<string|null>(null);
  const [selected,setSelected] = useState<PoolRecord|null>(null);
  const [comparison,setComparison] = useState<PoolComparison|null>(null);
  const [offset,setOffset] = useState(0);
  const [asOf,setAsOf] = useState(nowShanghai);
  const [ack,setAck] = useState(false);
  const [busy,setBusy] = useState(false);
  const [error,setError] = useState("");
  const [info,setInfo] = useState("");
  const [reason,setReason] = useState("");
  const [saved,setSaved] = useState<string[]>([]);

  const refreshList = useCallback(async () => {
    const data = await request<{ pools: unknown[] }>("/pools?limit=20");
    if (!Array.isArray(data.pools)||!data.pools.every(isSummary))
      throw Error("全市场任务历史数据校验失败");
    const pools = data.pools as PoolSummary[];
    setHistory(pools);
    setSelectedId(current=>current ?? pools[0]?.pool_id ?? null);
  },[]);
  useEffect(() => {
    let cancelled=false;
    void request<{pools:unknown[]}>("/pools?limit=20").then(data=>{
      if (cancelled) return;
      if (!Array.isArray(data.pools)||!data.pools.every(isSummary)) throw Error("任务历史数据无效");
      setHistory(data.pools);
      setSelectedId(current=>current??(data.pools as PoolSummary[])[0]?.pool_id??null);
    }).catch(e=>{if(!cancelled)setError((e as Error).message);});
    return ()=>{cancelled=true;};
  },[]);

  useEffect(()=>{
    if(!selectedId) return;
    let cancelled=false;
    const load = async ()=>{
      try {
        const data:unknown = await request(`/pools/${selectedId}`);
        if(!isRecord(data)||data.pool_id!==selectedId) throw Error("任务详情校验失败");
        if(cancelled)return;
        setSelected(data);
        setHistory(current=>current?.map(p=>p.pool_id===data.pool_id?{
          ...p,state:data.state,next_offset:data.next_offset,
          cache_days_done:data.cache_days_done,cache_days_total:data.cache_days_total,
          failure_code:data.failure_code,universe_count:data.symbols.length}:p)??null);
      }catch(e){if(!cancelled)setError((e as Error).message);}
    };
    void load();
    const id=window.setInterval(()=>{void load();},5000);
    return ()=>{cancelled=true;window.clearInterval(id);};
  },[selectedId]);

  useEffect(()=>{
    if(!selectedId||selected?.pool_id!==selectedId||selected.state!=="COMPLETED")return;
    let cancelled=false;
    void request<unknown>(`/pools/${selectedId}/comparison?offset=${offset}&limit=50`).then(data=>{
      if(cancelled)return;
      if(!isComparison(data)||data.pool_id!==selectedId||data.as_of!==selected.as_of||
        data.universe_count!==selected.symbols.length) throw Error("全市场候选结果校验失败");
      setComparison(data);
    }).catch(e=>{if(!cancelled)setError((e as Error).message);});
    return ()=>{cancelled=true;};
  },[selectedId,selected?.pool_id,selected?.state,selected?.as_of,selected?.symbols.length,offset]);

  async function start(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if(!ack){setError("请确认已了解真实行情请求量及额度风险");return;}
    if(!validDate(asOf)){setError("截止日必须是有效且不晚于今日的 YYYYMMDD 日期");return;}
    setBusy(true);setError("");setInfo("");
    try {
      const data:unknown=await request("/pools",{method:"POST",headers:{"Content-Type":"application/json"},
        body:JSON.stringify({as_of:asOf})});
      if(!isRecord(data))throw Error("任务可能已提交，但服务端没有返回有效 ID，请刷新任务列表核实");
      setSelected(data);setSelectedId(data.pool_id);setComparison(null);setOffset(0);setSaved([]);
      setInfo("已冻结历史股票范围，服务器将分批扫描。离开页面不会取消任务。");
      await refreshList();
    }catch(e){setError((e as Error).message);}
    finally{setBusy(false);}
  }
  async function command(commandName: "pause"|"resume"){
    if(!selected)return;
    setBusy(true);setError("");setInfo("");
    try{
      const data:unknown=await request(`/pools/${selected.pool_id}/${commandName}`,{method:"POST"});
      if(!isRecord(data))throw Error("服务器返回任务状态无效");
      setSelected(data);
      setInfo(commandName==="pause"?"已请求在当前批次完成后暂停":"已请求继续剩余批次。");
    }catch(e){setError((e as Error).message);}
    finally{setBusy(false);}
  }
  async function save(symbol:string,scanId:string){
    if(reason.trim().length<3){setError("请先填写至少 3 字的人工观察理由");return;}
    setBusy(true);setError("");
    try{
      await request("/watchlist",{method:"POST",headers:{"Content-Type":"application/json"},
        body:JSON.stringify({symbol,reason:`${reason.trim()}（市场扫描 ${selected?.as_of} / ${scanId.slice(0,10)}）`,conditions:""})});
      setSaved(old=>[...old,symbol]);setInfo(`${symbol} 已加入观察池`);
    }catch(e){setError((e as Error).message);}
    finally{setBusy(false);}
  }

  const total=selected?.symbols.length??0;
  const progress=total>0?Math.floor((selected!.next_offset/total)*100):0;
  const cacheDays=selected?.cache_days_total??0;
  const cacheReady=selected?.cache_days_done??0;
  const view=comparison?.pool_id===selectedId?comparison:null;
  return <section className="surface market-pool" aria-label="全市场主板策略候选池">
    <div className="panel-head"><div><h2>主板股票池 · 自动筛选</h2>
      <p>一次冻结沪深主板的历史股票范围，服务端分批计算候选。非全 A 股所有板块。</p></div>
      <span className="badge blue">PIT · 可恢复</span>
    </div>
    <form className="market-pool-start" onSubmit={start}>
      <label>筛选截止日（YYYYMMDD）<input inputMode="numeric" maxLength={8}
        value={asOf} onChange={e=>setAsOf(e.target.value)} required/></label>
      <label className="market-pool-ack"><input type="checkbox" checked={ack} onChange={e=>setAck(e.target.checked)}/>
        <span>了解可能消耗大量 TuShare 请求额度；失败不自动无限重试。</span></label>
      <button className="btn primary" type="submit" disabled={busy||!ack}>
        <PlayCircle size={14}/>{busy?"处理中…":"启动主板范围扫描"}
      </button>
    </form>
    {error&&<p className="live-alert" role="alert"><TriangleAlert size={15}/>{error}<button type="button" onClick={()=>setError("")}>关闭</button></p>}
    {info&&<p className="live-info" role="status">{info}</p>}
    <div className="market-pool-history">
      <div className="market-pool-history-heading"><strong>历史股票池</strong>
        <button type="button" className="btn" onClick={()=>void refreshList().catch(e=>setError((e as Error).message))}>
          <RefreshCcw size={13}/>刷新
        </button></div>
      {history===null?<p className="scan-result-message">正在读取服务端股票池记录…</p>:
        history.length===0?<p className="scan-result-message">暂无历史股票池。</p>:
        <div className="market-pool-history-list">{history.map(pool=><button type="button" key={pool.pool_id}
          className={selectedId===pool.pool_id?"market-pool-history-item selected":"market-pool-history-item"}
          onClick={()=>{setSelectedId(pool.pool_id);setSelected(null);setComparison(null);setOffset(0);setSaved([]);}}>
          <strong>{fmtDate(pool.as_of)}</strong><small>{pool.next_offset} / {pool.universe_count} 只</small>
          <span>{label[pool.state]}</span><ArrowRight size={12}/></button>)}</div>}
    </div>

    {selected&&selected.pool_id===selectedId&&<div className="market-pool-progress">
      <div className="market-pool-progress-title"><strong>{fmtDate(selected.as_of)} · {label[selected.state]}</strong>
        <span>{selected.next_offset} / {total} 只 · {progress}%</span></div>
      {cacheDays>0&&<div className="market-pool-progress-title">
        <span>行情快照：{cacheReady} / {cacheDays} 个交易日</span>
        <span>{cacheReady<cacheDays?"预热中或暂停":"预热完成"}</span>
      </div>}
      <progress max={total||1} value={selected.next_offset}/>
      <small>完整范围摘要 {selected.universe_sha256.slice(0,16)}… · 批量上限 20/批 · 失败不会计入完成覆盖</small>
      {selected.failure_code&&<div className="market-pool-failure" role="status">状态原因：{selected.failure_code}</div>}
      <div className="market-pool-actions">
        {selected.state==="RUNNING"&&<button type="button" className="btn" disabled={busy}
          onClick={()=>void command("pause")}><Pause size={14}/>批次完成后暂停</button>}
        {(selected.state==="PAUSED"||selected.state==="FAILED")&&<button type="button" className="btn primary"
          disabled={busy} onClick={()=>void command("resume")}><PlayCircle size={14}/>继续未完成批次</button>}
      </div>
      {selected.state!=="COMPLETED"?<p className="scan-disclaimer">
        完成全部 {total} 只后才会显示正式候选池。已覆盖数量不代表符合条件的候选数量；进程中断后需手动恢复。
      </p>:!view?<p className="scan-result-message">扫描结束，正在核验所有批次和候选排名…</p>:<>
        <div className="market-pool-counts"><strong>{view.selected_count} 只通过</strong>
          <span>{view.incomparable_count} 只未入选或数据不足</span>
          <span>完整覆盖 {view.universe_count} / {view.universe_count}</span></div>
        <label className="market-pool-note">加入观察池的人工理由
          <input value={reason} onChange={e=>setReason(e.target.value)} maxLength={800}
            placeholder="例如：继续关注回踩趋势和成交量确认"/>
        </label>
        {view.ranked.length===0?<p className="scan-result-message">这页没有候选股票。</p>:
          <div className="market-pool-ranked">{view.ranked.map(r=><div className="market-pool-row" key={r.symbol}>
            <strong>#{r.rank} · {r.symbol}</strong>
            <span>收盘 {n(r.metrics.close)} · MA20 {n(r.metrics.ma20)} · MA60 {n(r.metrics.ma60)}</span>
            <small>距 MA20 {(100*(r.metrics.close_over_ma20-1)).toFixed(2)}%</small>
            <a href={`${base}/scans/${r.scan_id}/results/${r.symbol}/snapshot`} target="_blank"
              rel="noopener noreferrer" title={r.source.sha256}><Database size={12}/>源快照</a>
            <div><button type="button" className="btn" disabled={busy||saved.includes(r.symbol)}
              onClick={()=>void save(r.symbol,r.scan_id)}><Plus size={13}/>{saved.includes(r.symbol)?"已保存":"加入观察"}</button>
              <button type="button" className="btn primary" disabled={researching}
                onClick={()=>onStartResearch(r.symbol)}><Search size={13}/>研究</button></div>
          </div>)}</div>}
        <div className="market-pool-paging">
          <button type="button" className="btn" disabled={offset===0} onClick={()=>setOffset(v=>Math.max(0,v-50))}>上一页</button>
          <span>第 {Math.floor(offset/50)+1} 页 · 共 {view.selected_count} 条候选</span>
          <button type="button" className="btn" disabled={offset+50>=view.selected_count} onClick={()=>setOffset(v=>v+50)}>下一页</button>
          <button type="button" className="btn" onClick={onOpenWatchlist}>打开观察池</button>
        </div>
        <details className="scan-excluded"><summary>未入选与数据不足原因（{view.incomparable_count}）</summary>
          {Object.entries(view.reason_counts).sort((a,b)=>b[1]-a[1]).map(([reason,count])=>
            <div className="scan-excluded-row" key={reason}><strong>{reason}</strong><span>{count} 只</span></div>)}
        </details>
      </>}
    </div>}
    <p className="scan-disclaimer">范围由 TuShare 历史上市状态生成，仅涵盖沪深主板。排名是确定性技术筛选，不代表收益预测，也不会自动下单。</p>
  </section>;
}
