"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, ArrowRight, Ban, BookOpenText, ChevronRight, CircleDot,
  Clock3, Database, Download, FileClock, LockKeyhole, LogOut, Radar, RefreshCcw,
  Search, ShieldCheck, TerminalSquare, TriangleAlert, Wifi, WifiOff,
} from "lucide-react";
import {
  isAshareSymbol, isLiveEvent, isLiveRun, isTerminal, mergeStageEvents,
  readReportSummary, type LiveArtifact, type LiveRun, type LiveSession, type LiveStageEvent,
} from "@/lib/live-contract";
import { cn } from "@/lib/utils";

const base = "/api/workbench";
const states: Record<LiveRun["state"], string> = {
  PENDING: "等待执行", RUNNING: "研究中", COMPLETED: "已完成", FAILED: "失败", CANCELLED: "已取消",
};
const statusColors: Record<LiveRun["state"], string> = {
  PENDING: "blue", RUNNING: "blue", COMPLETED: "green", FAILED: "red", CANCELLED: "gray",
};
const formatDate = (value: string | undefined) => value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";

async function jsonRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, { cache: "no-store", credentials: "same-origin", ...init });
  if (!r.ok) {
    if (r.status === 401) throw Error("登录已过期，请重新登录");
    if (r.status === 409) throw Error("当前研究状态不支持此操作");
    if (r.status === 429) throw Error("研究队列已满，请稍后再试");
    throw Error(`请求失败（HTTP ${r.status}）`);
  }
  return r.json() as Promise<T>;
}

function classifyState(status: string) {
  const normalized = status.toUpperCase();
  if (normalized === "DONE" || normalized === "COMPLETED") return "green";
  if (normalized === "FAIL" || normalized === "FAILED") return "red";
  return "blue";
}

function JsonEvidence({ value, label }: { value: unknown; label: string }) {
  if (value === undefined || value === null) return null;
  const json = JSON.stringify(value, null, 2);
  return <details className="live-raw-panel"><summary>{label} · 原始后端工件</summary><pre>{json.length > 28000 ? `${json.slice(0, 28000)}\n… 已截断预览，完整内容请使用导出功能。` : json}</pre></details>;
}

function ResearchArtifactPanel({ artifact }: { artifact: LiveArtifact }) {
  const report = readReportSummary(artifact);
  const synthesis = artifact.research_artifact && typeof artifact.research_artifact === "object"
    ? (artifact.research_artifact as Record<string, unknown>).synthesis : null;
  const evidence = Array.isArray(artifact.evidence) ? artifact.evidence.filter(v => typeof v === "string") : [];
  return <div className="live-report-stack">
    {report.headline && <section className="surface live-summary"><div className="eyebrow">Research synthesis · 后端真实输出</div><h2>研究摘要</h2><p>{report.headline}</p></section>}
    {report.sections.length > 0 && <section className="surface live-report-section"><h2>决策与风控字段</h2><div className="live-facts">{report.sections.map((item, i) => <div key={`${item.label}-${i}`}><small>{item.label}</small><p>{item.value}</p></div>)}</div></section>}
    {evidence.length > 0 && <section className="surface live-report-section"><h2>Decision 引用证据</h2>{evidence.map((item, i) => <p className="live-evidence-text" key={i}>{item}</p>)}</section>}
    {!!synthesis && <JsonEvidence value={synthesis} label="Synthesis / 论证与分歧" />}
    {artifact.portfolio_review !== undefined && <JsonEvidence value={artifact.portfolio_review} label="Portfolio Review / 辩论与证据索引" />}
    {artifact.final_risk_gate !== undefined && <JsonEvidence value={artifact.final_risk_gate} label="Final Risk Gate / 风险门禁" />}
    <JsonEvidence value={artifact} label="完整 Typed Artifact（只读预览）" />
    <p className="live-boundary">报告来自后端已完成任务的原始 JSON。缺少的数据不会通过前端推测或补全；任何结论都不等于交易委托或投资授权。</p>
  </div>;
}

export default function LiveResearchWorkbench() {
  const [session, setSession] = useState<LiveSession | null>(null);
  const [password, setPassword] = useState("");
  const [symbol, setSymbol] = useState("600519.SH");
  const [marketQuery, setMarketQuery] = useState("");
  const [runs, setRuns] = useState<LiveRun[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // Associate responses with their Run ID so a selection change can never
  // flash a previous run's artifact or evidence events before effects execute.
  const [eventRecord, setEventRecord] = useState<{ runId: string; values: LiveStageEvent[] } | null>(null);
  const [artifactRecord, setArtifactRecord] = useState<{ runId: string; value: LiveArtifact } | null>(null);
  const events = eventRecord?.runId === selectedId ? eventRecord.values : [];
  const artifact = artifactRecord?.runId === selectedId ? artifactRecord.value : null;
  const [view, setView] = useState<"workspace" | "history">("workspace");
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [busy, setBusy] = useState(false);
  const [streamRecord, setStreamRecord] = useState<{ runId: string; status: "connected" | "disconnected" } | null>(null);
  const streamState = streamRecord?.runId === selectedId ? streamRecord.status : "idle";
  const selected = useMemo(() => runs.find(run => run.run_id === selectedId) ?? null, [runs, selectedId]);
  const mounted = useRef(true);

  const refresh = useCallback(async () => {
    const data = await jsonRequest<{ runs: unknown[] }>(`${base}/runs`);
    const next = Array.isArray(data.runs) ? data.runs.filter(isLiveRun) : [];
    if (!mounted.current) return;
    setRuns(next);
    setSelectedId(current => current && next.some(run => run.run_id === current) ? current : next[0]?.run_id ?? null);
  }, []);

  useEffect(() => {
    mounted.current = true;
    jsonRequest<LiveSession>(`${base}/session`).then(s => { if (mounted.current) setSession(s); }).catch(() => { if (mounted.current) setError("无法验证当前 Web 会话"); });
    return () => { mounted.current = false; };
  }, []);
  useEffect(() => {
    if (!session?.authenticated) return;
    let cancelled = false;
    const update = async () => {
      try { await refresh(); if (!cancelled) setError(""); }
      catch(e) { if (!cancelled) setError((e as Error).message); }
    };
    void update();
    const id = window.setInterval(() => { void update(); }, 3000);
    return () => { cancelled = true; window.clearInterval(id); };
  }, [session?.authenticated, refresh]);

  useEffect(() => {
    if (!session?.authenticated || !selectedId) return;
    let disposed = false;
    const load = async () => {
      try {
        const result = await jsonRequest<{ events: unknown[] }>(`${base}/runs/${selectedId}/events`);
        if (!disposed) setEventRecord(current => ({ runId: selectedId, values: mergeStageEvents(current?.runId === selectedId ? current.values : [], Array.isArray(result.events) ? result.events.filter(isLiveEvent) : []) }));
      } catch { /* SSE can still provide events; list polling keeps status visible. */ }
    };
    void load();
    return () => { disposed = true; };
  }, [selectedId, session?.authenticated]);

  useEffect(() => {
    if (!session?.authenticated || !selectedId || selected?.state !== "COMPLETED") return;
    let disposed = false;
    jsonRequest<LiveArtifact>(`${base}/runs/${selectedId}/artifact`).then(value => {
      if (!disposed && value && typeof value === "object" && !Array.isArray(value)) setArtifactRecord({ runId: selectedId, value });
    }).catch(err => { if (!disposed) setError((err as Error).message); });
    return () => { disposed = true; };
  }, [selectedId, selectedState, session?.authenticated]);

  const selectedState = selected?.state;
  useEffect(() => {
    if (!session?.authenticated || !selectedId || !selectedState || isTerminal(selectedState)) return;
    const stream = new EventSource(`${base}/runs/${selectedId}/stream`);
    stream.onopen = () => setStreamRecord({ runId: selectedId, status: "connected" });
    stream.onerror = () => setStreamRecord({ runId: selectedId, status: "disconnected" });
    stream.addEventListener("research_stage", e => {
      try {
        const parsed: unknown = JSON.parse((e as MessageEvent<string>).data);
        if (isLiveEvent(parsed)) setEventRecord(current => ({ runId: selectedId, values: mergeStageEvents(current?.runId === selectedId ? current.values : [], [parsed]) }));
      } catch { /* malformed event is not presented as a stage */ }
    });
    return () => { stream.close(); };
  }, [selectedId, selectedState, session?.authenticated]);

  async function signIn(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(""); setBusy(true);
    try {
      const logged = await jsonRequest<LiveSession>(`${base}/session`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password }) });
      setPassword(""); setSession(logged); setInfo("已连接至后端研究服务（单操作者会话）");
    } catch(e) { setError((e as Error).message); } finally { setBusy(false); }
  }
  async function signOut() {
    try { await jsonRequest(`${base}/session`, { method: "DELETE" }); } catch { /* local session closes regardless */ }
    setSession(current => current && { ...current, authenticated: false }); setRuns([]); setSelectedId(null); setEventRecord(null); setArtifactRecord(null);
  }
  async function startRun(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault(); setError(""); setInfo("");
    const name = symbol.trim().toUpperCase();
    if (!isAshareSymbol(name)) { setError("股票代码必须为 6 位代码加 .SH 或 .SZ，例如 600519.SH"); return; }
    if (!session?.authenticated) return;
    setBusy(true);
    try {
      const payload = { symbol: name, mode: session.mode, ...(marketQuery.trim() ? { market_query: marketQuery.trim() } : {}),
        ...(session.mode === "PORTFOLIO_REVIEW" ? { portfolio: { state: "NOT_PROVIDED" } } : {}) };
      const created: unknown = await jsonRequest(`${base}/runs`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
      if (!isLiveRun(created)) throw Error("服务器返回的 Run 结构无效");
      setRuns(current => [created, ...current.filter(r => r.run_id !== created.run_id)]);
      setSelectedId(created.run_id); setView("workspace"); setInfo("研究任务已提交至 Go 服务；模型费用及数据源请求可能开始发生。");
    } catch(e) { setError((e as Error).message); } finally { setBusy(false); }
  }
  async function cancelRun() {
    if (!selected || isTerminal(selected.state)) return;
    setBusy(true); setError("");
    try { await jsonRequest(`${base}/runs/${selected.run_id}/cancel`, { method: "POST" }); setInfo("已请求取消，等待 Go 后端确认终态。"); await refresh(); }
    catch(e) { setError((e as Error).message); } finally { setBusy(false); }
  }
  function downloadArtifact() {
    if (!artifact || selected?.state !== "COMPLETED") return;
    const url = URL.createObjectURL(new Blob([JSON.stringify(artifact, null, 2)], { type: "application/json" }));
    const a = document.createElement("a"); a.href = url; a.download = `aegis-${selected.run_id}-artifact.json`; a.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1500);
  }

  if (!session) return <div className="live-gate"><Radar size={36} /><h1>AEGIS · Live Research</h1><p>正在验证本机研究工作台配置…</p>{error && <p role="alert">{error}</p>}</div>;
  if (!session.configured) return <div className="live-gate"><TriangleAlert size={34}/><h1>真实研究模式尚未配置</h1><p>请在 Next.js 服务端配置 Go API URL、API Token、Web 访问口令与会话密钥。不会自动切换为 Mock 数据。</p><p>参见 web/README.md</p></div>;
  if (!session.authenticated) return <div className="live-gate"><span className="brand-mark"><Radar size={24}/></span><div className="eyebrow">Research intelligence / secure access</div><h1>AEGIS · 研究工作台</h1><p>真实 Go/Eino 工作流 · 当前仅支持单操作者的受保护本地服务</p><form onSubmit={signIn} className="live-login"><label htmlFor="password">工作台访问口令</label><input id="password" autoComplete="current-password" type="password" minLength={16} value={password} onChange={e=>setPassword(e.target.value)} placeholder="输入服务端配置的访问口令" required/><button className="btn primary" disabled={busy} type="submit"><LockKeyhole size={15}/>{busy?"验证中…":"解锁研究工作台"}</button></form>{error && <div className="live-alert" role="alert">{error}</div>}<small>页面不会接收 Go API Token。访问口令仅提交给本站服务端建立 HttpOnly 会话。</small></div>;

  return <div className="workspace live-workspace">
    <aside className="sidebar" aria-label="实时研究导航">
      <div className="brand"><span className="brand-mark"><Radar size={24}/></span><div><div className="brand-title">AEGIS<span style={{color:"#7ceac7"}}>.</span></div><div className="brand-sub">Live research console</div></div></div>
      <nav aria-label="研究导航"><div className="nav-label">WORKSPACE</div><button type="button" onClick={()=>setView("workspace")} className={cn("nav-item",view==="workspace"&&"active")}><Radar size={17}/>研究工作区</button><button type="button" onClick={()=>setView("history")} className={cn("nav-item",view==="history"&&"active")}><FileClock size={17}/>研究历史<span className="nav-count">{runs.length}</span></button></nav>
      <div className="sidebar-sep"/><div className="watch-title">最新研究 <span>LIVE</span></div><div className="live-side-list">{runs.slice(0,12).map(r=><button type="button" key={r.run_id} onClick={()=>{setSelectedId(r.run_id);setView("workspace");setError("");}} className={cn("live-side-run",selectedId===r.run_id&&"chosen")}><strong>{r.symbol}</strong><small>{states[r.state]} · {formatDate(r.created_at)}</small></button>)}{runs.length===0&&<p className="live-empty">尚无真实研究任务</p>}</div>
      <div className="sidebar-bottom"><div className="terminal-note"><ShieldCheck size={18}/><span><strong>Go Agent 已连接</strong><br/>任务和模型运行均由服务端负责</span></div><button type="button" className="live-signout" onClick={()=>void signOut()}><LogOut size={15}/>退出会话</button></div>
    </aside>
    <main className="main-area"><header className="topbar"><div className="breadcrumb"><span>Research workspace</span><ChevronRight size={13}/><strong>{view==="history"?"研究历史":"实时研究"}</strong></div><div className="topbar-spacer"/><span className="badge green"><CircleDot size={12}/> LIVE · GO API</span><button className="icon-button" aria-label="刷新" type="button" onClick={()=>void refresh()}><RefreshCcw size={16}/></button></header>
    <div className="content live-content"><div className="page-heading"><div><div className="eyebrow">Multi-agent research / production source</div><h1>{view==="history"?"研究历史":"实时研究工作台"}</h1><p>真实 Go 后端 · 严格数据时点 · Agent 执行过程可追踪</p></div><div className="heading-right"><button className="btn" type="button" disabled={!artifact||selected?.state!=="COMPLETED"} onClick={downloadArtifact}><Download size={14}/>导出真实 JSON</button></div></div>
    <div className="live-scope-banner"><ShieldCheck size={16}/><span>当前为真实研究连接。仅使用 Go 返回的 Run、阶段事件和工件；未接入真实行情视图时不绘制假价格图。组合账户默认显式标记 NOT_PROVIDED（未知），不执行交易。</span></div>
    {error&&<div role="alert" className="live-alert"><TriangleAlert size={15}/>{error}<button onClick={()=>setError("")} type="button">关闭</button></div>}
    {info&&<div role="status" className="live-info">{info}</div>}
    {view==="history"?<section className="surface live-history"><div className="panel-head"><h2>服务端持久化记录</h2><span className="badge blue">{runs.length} 条（接口返回上限 100）</span></div>{runs.length===0?<p className="live-empty">尚无研究记录。</p>:<div className="live-history-table">{runs.map(r=><button key={r.run_id} className="live-history-row" onClick={()=>{setSelectedId(r.run_id);setView("workspace");setError("");}} type="button"><strong>{r.symbol}</strong><code>{r.run_id.slice(0,12)}…</code><small>{formatDate(r.created_at)}</small><span className={`badge ${statusColors[r.state]}`}>{states[r.state]}</span><ArrowRight size={15}/></button>)}</div>}</section>:<div className="main-grid live-grid"><div className="left-stack">
      <section className="surface live-create"><div className="eyebrow">New research run</div><h2>发起股票研究</h2><p>研究时点由后端创建任务时确定。正式执行可能触发数据源请求和模型费用。</p><form onSubmit={startRun}><label>股票代码<input value={symbol} onChange={e=>setSymbol(e.target.value.toUpperCase())} placeholder="600519.SH" required/></label><label>研究关注点（可选）<input maxLength={1024} value={marketQuery} onChange={e=>setMarketQuery(e.target.value)} placeholder="例如：近期市场环境与标的事件"/></label><div className="live-create-foot"><span className="badge blue">{session.mode}</span>{session.mode==="PORTFOLIO_REVIEW"&&<span className="badge gray">账户：NOT_PROVIDED · 未知</span>}<button className="btn primary" type="submit" disabled={busy}><Search size={15}/>{busy?"处理中…":"启动真实研究"}</button></div></form></section>
      <section className="surface live-run-detail"><div className="panel-head"><div><h2>任务运行详情</h2><p>异步任务进程不依赖浏览器标签页</p></div>{selected&&<span className={`badge ${statusColors[selected.state]}`}>{states[selected.state]}</span>}</div>{selected?<div className="live-run-fields"><div><small>标的</small><strong>{selected.symbol}</strong></div><div><small>研究时点 (PIT)</small><strong>{formatDate(selected.requested_as_of)}</strong></div><div><small>任务 ID</small><code>{selected.run_id}</code></div><div><small>状态</small><strong>{states[selected.state]}</strong></div><div><small>开始</small><strong>{formatDate(selected.started_at)}</strong></div><div><small>结束</small><strong>{formatDate(selected.finished_at)}</strong></div>{selected.failure_code&&<div className="live-full"><small>错误代码</small><strong className="down">{selected.failure_code}</strong></div>}</div>:<p className="live-empty">尚未选择任务。发起一次研究或从历史中打开。</p>}{selected&&!isTerminal(selected.state)&&<div className="live-cancel"><button type="button" className="btn danger" disabled={busy} onClick={()=>void cancelRun()}><Ban size={14}/>请求取消</button></div>}</section>
      {selected?.state==="COMPLETED"&&artifact?<ResearchArtifactPanel artifact={artifact}/>:<section className="surface live-report-empty"><BookOpenText size={21}/><div><h2>研究结果</h2><p>{selected?.state==="COMPLETED"?"正在加载后端已完成工件…":selected?.state==="FAILED"||selected?.state==="CANCELLED"?"任务没有完成，研究报告不可用；不会显示上一次任务的结果。":"只有后端状态为 COMPLETED 才会展示真实报告。"}</p></div></section>}
    </div><aside className="right-stack"><section className="surface live-events"><div className="panel-head"><div><h2>Agent 执行轨迹</h2><p>来源：Go 结构化事件与持久化记录</p></div><span title="SSE 状态">{streamState==="connected"?<Wifi size={17} color="#65e1c4"/>:streamState==="disconnected"?<WifiOff size={17} color="#e9b16c"/>:<Clock3 size={17} color="#71849b"/>}</span></div>{!selected?<p className="live-empty">选择任务后查看阶段事件</p>:events.length===0?<p className="live-empty">尚无执行事件。状态会自动刷新。</p>:<div className="live-event-list">{events.map(ev=><div key={ev.seq} className="live-event-item"><span className={`live-event-dot ${classifyState(ev.status)}`}/><div><strong>{ev.stage}</strong><small>{ev.status} · {formatDate(ev.at)}{typeof ev.duration_ms==="number"?` · ${ev.duration_ms}ms`:""}</small></div><code>#{ev.seq}</code></div>)}</div>}<div className="live-stream-foot"><Activity size={13}/>SSE 实时订阅 · 接口事件历史可重放{streamState==="disconnected"?" · 断开后尝试重连":""}</div></section><section className="surface live-help"><div className="panel-head"><h2>研究安全边界</h2></div><p><Database size={14}/>结果仅来自 Go Typed Artifact</p><p><ShieldCheck size={14}/>账户未知不等于空仓或零现金</p><p><TerminalSquare size={14}/>无委托、资金划拨与自动交易</p></section></aside></div>}
    <footer className="live-footer">AEGIS RESEARCH · SINGLE-OPERATOR LIVE MODE · 不包含交易执行</footer></div></main></div>;
}
