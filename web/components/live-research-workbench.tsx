"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeftRight, ArrowRight, Ban, BookOpenText, ChevronRight, CircleDot, LayoutDashboard,
  Database, Download, Eye, FileClock, LockKeyhole, LogOut, Radar, RefreshCcw,
  Search, ShieldCheck, TerminalSquare, TriangleAlert, RotateCcw,
} from "lucide-react";
import {
  isAshareSymbol, isLiveEvent, isLiveRun, isTerminal, mergeStageEvents,
  type LiveArtifact, type LiveRun, type LiveSession, type LiveStageEvent,
} from "@/lib/live-contract";
import { cn } from "@/lib/utils";
import { findPreviousComparableRun } from "@/lib/run-comparison";
import { renderResearchMarkdown } from "@/lib/research-markdown";
import ProviderSettings from "@/components/provider-settings";
import ResearchConversationPanel from "@/components/research-conversation-panel";
import LiveResearchReport from "@/components/live-research-report";
import LiveStageTrace from "@/components/live-stage-trace";
import LiveRunComparison from "@/components/live-run-comparison";
import LiveRunOverview from "@/components/live-run-overview";
import LiveEvidenceSourceIndex from "@/components/live-evidence-source-index";
import WatchlistPanel from "@/components/watchlist-panel";
import { readWorkbenchLocation, readComparisonLocation, comparisonHref, workbenchHref, type WorkbenchView } from "@/lib/workbench-location";

const base = "/api/workbench";
const states: Record<LiveRun["state"], string> = {
  PENDING: "等待执行", RUNNING: "研究中", COMPLETED: "已完成", FAILED: "失败", CANCELLED: "已取消",
};
const modeLabel = (mode: string) => ({
  DECISION: "个股决策研究",
  PORTFOLIO_REVIEW: "组合复核",
  RESEARCH: "研究分析",
}[mode] ?? mode.replaceAll("_", " "));
const statusColors: Record<LiveRun["state"], string> = {
  PENDING: "blue", RUNNING: "blue", COMPLETED: "green", FAILED: "red", CANCELLED: "gray",
};
const formatDate = (value: string | undefined) => {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "时间不可用" : date.toLocaleString("zh-CN", { hour12: false });
};

async function jsonRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, { cache: "no-store", credentials: "same-origin", ...init });
  if (!r.ok) {
    if (r.status === 401) throw Error(url.endsWith("/session") && init?.method === "POST" ? "访问口令不正确" : "登录已过期，请重新登录");
    if (r.status === 409) throw Error("当前研究状态不支持此操作");
    if (r.status === 429) throw Error("研究队列已满，请稍后再试");
    throw Error(`请求失败（HTTP ${r.status}）`);
  }
  return r.json() as Promise<T>;
}

export default function LiveResearchWorkbench() {
  const [session, setSession] = useState<LiveSession | null>(null);
  const [password, setPassword] = useState("");
  const [symbol, setSymbol] = useState("600519.SH");
  const [marketQuery, setMarketQuery] = useState("");
  const [rerunFrom, setRerunFrom] = useState<{ runId: string; symbol: string } | null>(null);
  const [runs, setRuns] = useState<LiveRun[]>([]);
  const [historySymbol, setHistorySymbol] = useState("");
  const [historyState, setHistoryState] = useState<"ALL" | LiveRun["state"]>("ALL");
  const [historyPage, setHistoryPage] = useState(0);
  const [historyResponse, setHistoryResponse] = useState<{ key: string; runs: LiveRun[]; total: number; hasMore: boolean } | null>(null);
  const historyQueryKey = JSON.stringify([historyPage, historySymbol.trim().toUpperCase(), historyState]);
  const visibleHistory = historyResponse?.key === historyQueryKey ? historyResponse : null;
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [historyRetry, setHistoryRetry] = useState(0);
  const historyLimit = 25;

  const [lastListSync, setLastListSync] = useState<string | null>(null);
  const [listSyncError, setListSyncError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // Associate responses with their Run ID so a selection change can never
  // flash a previous run's artifact or evidence events before effects execute.
  const [eventRecord, setEventRecord] = useState<{ runId: string; values: LiveStageEvent[] } | null>(null);
  const [artifactRecord, setArtifactRecord] = useState<{ runId: string; value: LiveArtifact } | null>(null);
  const events = eventRecord?.runId === selectedId ? eventRecord.values : [];
  const artifact = artifactRecord?.runId === selectedId ? artifactRecord.value : null;
  const [view, setView] = useState<WorkbenchView>("overview");
  const [comparisonPair, setComparisonPair] = useState<{leftId:string|null;rightId:string|null}>({leftId:null,rightId:null});
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [busy, setBusy] = useState(false);
  const [streamRecord, setStreamRecord] = useState<{ runId: string; status: "connected" | "disconnected" } | null>(null);
  const streamState = streamRecord?.runId === selectedId ? streamRecord.status : "idle";
  const selected = useMemo(() => runs.find(run => run.run_id === selectedId) ?? null, [runs, selectedId]);
  const selectedState = selected?.state;
  const mounted = useRef(true);
  // Keep the same key if a POST response is lost; a deliberate new study gets a new key.
  const pendingStart = useRef<{ payload: string; key: string } | null>(null);

  const navigate = useCallback((target: WorkbenchView, runId: string | null = null) => {
    const href = workbenchHref(window.location.pathname, target, runId);
    if (`${window.location.pathname}${window.location.search}` !== href) {
      window.history.pushState(null, "", href);
    }
    setView(target);
    if (target === "compare") setComparisonPair({leftId:null,rightId:null});
    if (target === "workspace" && runId) setSelectedId(runId);
  }, []);

  const openComparison = useCallback((leftId: string | null, rightId: string | null) => {
    const href = comparisonHref(window.location.pathname, leftId, rightId);
    if (window.location.pathname + window.location.search !== href) window.history.pushState(null, "", href);
    setComparisonPair({leftId, rightId});
    setView("compare");
  }, []);

  const compareWithPrevious = useCallback(async (runId: string) => {
    const current = runs.find(run => run.run_id === runId && run.state === "COMPLETED");
    if (!current) {
      openComparison(runId, null);
      return;
    }
    const candidates = new Map<string, LiveRun>();
    let foundCurrent = false;
    let offset = 0;
    try {
      // The default Run list is capped at 100 across all symbols. Query the
      // selected symbol's persisted history and page until its prior Run is found.
      for (let page = 0; page < 10; page++) {
        const params = new URLSearchParams({ limit: "100", offset: String(offset), symbol: current.symbol, state: "COMPLETED" });
        const data = await jsonRequest<{ runs: unknown[]; has_more?: boolean }>(`${base}/runs?${params}`);
        const pageRuns = Array.isArray(data.runs) ? data.runs.filter(isLiveRun) : [];
        for (const run of pageRuns) candidates.set(run.run_id, run);
        foundCurrent ||= pageRuns.some(run => run.run_id === runId);
        if (foundCurrent) {
          const previous = findPreviousComparableRun([...candidates.values()], runId);
          if (previous) {
            openComparison(previous.run_id, runId);
            return;
          }
        }
        if (data.has_more !== true || pageRuns.length === 0) break;
        offset += pageRuns.length;
      }
    } catch {
      // If history lookup is unavailable, fall back to the already loaded sample.
    }
    const available = foundCurrent ? [...candidates.values()] : runs;
    const previous = findPreviousComparableRun(available, runId);
    openComparison(previous?.run_id ?? runId, previous ? runId : null);
  }, [runs, openComparison]);

  const refresh = useCallback(async () => {
    const data = await jsonRequest<{ runs: unknown[] }>(`${base}/runs`);
    const next = Array.isArray(data.runs) ? data.runs.filter(isLiveRun) : [];
    // The list is capped by Go. Deep links must still load a legitimate older Run.
    const linked = readWorkbenchLocation(window.location.search);
    if (linked.runId && !next.some(run => run.run_id === linked.runId)) {
      try {
        const direct: unknown = await jsonRequest(`${base}/runs/${linked.runId}`);
        if (isLiveRun(direct) && direct.run_id === linked.runId) next.push(direct);
      } catch {
        // Keep the requested ID selected; never silently display another report.
      }
    }
    if (!mounted.current) return;
    // A newly created Run can be absent from a briefly stale or capped list.
    // Retain its last known record and selection rather than switching to
    // another Run without an explicit user action.
    setRuns(current => {
      const retained = current.filter(run => run.run_id === selectedId && !next.some(item => item.run_id === run.run_id));
      return [...next, ...retained];
    });
    setLastListSync(new Date().toISOString());
    setListSyncError(null);
    setSelectedId(current => linked.runId ?? current ?? next[0]?.run_id ?? null);
  }, [selectedId]);

  useEffect(() => {
    if (!session?.authenticated || view !== "history") return;
    let cancelled = false;
    const queryKey = historyQueryKey;
    const params = new URLSearchParams({ limit: String(historyLimit), offset: String(historyPage * historyLimit) });
    if (historySymbol.trim()) params.set("symbol", historySymbol.trim().toUpperCase());
    if (historyState !== "ALL") params.set("state", historyState);
    jsonRequest<{ runs: unknown[]; total: number; has_more: boolean }>(`${base}/runs?${params}`).then(data => {
      if (cancelled) return;
      setHistoryResponse({
        key: queryKey,
        runs: Array.isArray(data.runs) ? data.runs.filter(isLiveRun) : [],
        total: Number.isSafeInteger(data.total) && data.total >= 0 ? data.total : 0,
        hasMore: data.has_more === true,
      });
      setHistoryError(null);
    }).catch(error => { if (!cancelled) { setHistoryResponse(null); setHistoryError((error as Error).message); } });
    return () => { cancelled = true; };
  }, [session?.authenticated, view, historyPage, historySymbol, historyState, historyQueryKey, historyRetry]);

  useEffect(() => {
    const restoreLocation = () => {
      const location = readWorkbenchLocation(window.location.search);
      setView(location.view);
      setComparisonPair(readComparisonLocation(window.location.search));
      if (location.view === "workspace") setSelectedId(location.runId);
    };
    restoreLocation();
    window.addEventListener("popstate", restoreLocation);
    return () => window.removeEventListener("popstate", restoreLocation);
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
      try { await refresh(); }
      catch(e) { if (!cancelled) setListSyncError((e as Error).message); }
    };
    void update();
    const id = window.setInterval(() => { void update(); }, 3000);
    return () => { cancelled = true; window.clearInterval(id); };
  }, [session?.authenticated, refresh]);

  // SSE is a low-latency path, never the only source of truth. Poll the Go
  // persisted event log while active and once again on terminal transition.
  // History reload and dropped streams must produce the same trace.
  useEffect(() => {
    if (!session?.authenticated || !selectedId) return;
    let disposed = false;
    const load = async () => {
      try {
        const result = await jsonRequest<{ events: unknown[] }>(`${base}/runs/${selectedId}/events`);
        if (!disposed) setEventRecord(current => ({
          runId: selectedId,
          values: mergeStageEvents(current?.runId === selectedId ? current.values : [],
            Array.isArray(result.events) ? result.events.filter(isLiveEvent) : []),
        }));
      } catch { /* Keep the observed SSE events; a failed GET cannot invent stage state. */ }
    };
    void load();
    const timer = selectedState && !isTerminal(selectedState) ? window.setInterval(() => { void load(); }, 2500) : null;
    return () => { disposed = true; if (timer !== null) window.clearInterval(timer); };
  }, [selectedId, selectedState, session?.authenticated]);

  useEffect(() => {
    if (!session?.authenticated || !selectedId || selectedState !== "COMPLETED") return;
    let disposed = false;
    jsonRequest<LiveArtifact>(`${base}/runs/${selectedId}/artifact`).then(value => {
      if (!disposed && value && typeof value === "object" && !Array.isArray(value)) setArtifactRecord({ runId: selectedId, value });
    }).catch(err => { if (!disposed) setError((err as Error).message); });
    return () => { disposed = true; };
  }, [selectedId, selectedState, session?.authenticated]);

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
    pendingStart.current = null;
    setRerunFrom(null);
    setComparisonPair({leftId:null,rightId:null});
    setSession(current => current && { ...current, authenticated: false }); setRuns([]); setLastListSync(null); setListSyncError(null); setSelectedId(null); setEventRecord(null); setArtifactRecord(null);
  }
  function prepareRerun() {
    if (!selected || !isTerminal(selected.state) || selected.mode !== session?.mode || busy) return;
    // The Go public Run DTO does not store previous market_query or account
    // inputs. Never claim this is an exact replay of the earlier research.
    pendingStart.current = null;
    setRerunFrom({ runId: selected.run_id, symbol: selected.symbol });
    setSymbol(selected.symbol);
    setMarketQuery("");
    setError("");
    setInfo(`已选取 ${selected.run_id.slice(0, 12)}… 作为来源。请确认新的研究关注点并手动提交；后端会按新的研究时点执行并可能产生模型费用。`);
    const form = document.querySelector(".live-create");
    if (form instanceof HTMLDetailsElement) form.open = true;
    form?.scrollIntoView({ behavior: "smooth", block: "start" });
  }
  // Shared submission path for the create form and the watchlist one-click
  // entry: both go through the same existing research API and idempotency
  // handling; nothing watchlist-specific reaches the run payload.
  async function submitResearch(name: string, query: string, rerun: { runId: string; symbol: string } | null) {
    if (!session?.authenticated) return false;
    setBusy(true);
    try {
      const payload = { symbol: name, mode: session.mode, ...(query ? { market_query: query } : {}),
        ...(rerun?.symbol === name ? { rerun_of: rerun.runId } : {}),
        ...(session.mode === "PORTFOLIO_REVIEW" ? { portfolio: { state: "NOT_PROVIDED" } } : {}) };
      const encoded = JSON.stringify(payload);
      const previous = pendingStart.current;
      const key = previous?.payload === encoded ? previous.key : crypto.randomUUID();
      pendingStart.current = { payload: encoded, key };
      const created: unknown = await jsonRequest(`${base}/runs`, { method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": key }, body: encoded });
      if (!isLiveRun(created)) throw Error("服务器返回的 Run 结构无效");
      pendingStart.current = null;
      setRerunFrom(null);
      setRuns(current => [created, ...current.filter(r => r.run_id !== created.run_id)]);
      navigate("workspace", created.run_id); setInfo(`研究任务已提交至 Go 服务；新任务 ${created.run_id.slice(0, 12)}… 拥有独立研究时点，模型费用及数据源请求可能开始发生。`);
      return true;
    } catch(e) { setError((e as Error).message); return false; } finally { setBusy(false); }
  }
  async function startRun(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault(); setError(""); setInfo("");
    const name = symbol.trim().toUpperCase();
    if (!isAshareSymbol(name)) { setError("股票代码必须为 6 位代码加 .SH 或 .SZ，例如 600519.SH"); return; }
    await submitResearch(name, marketQuery.trim(), rerunFrom);
  }
  async function startResearchFor(name: string) {
    setError(""); setInfo(`正在为观察池标的 ${name} 启动研究…`);
    await submitResearch(name, "", null);
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

  function downloadMarkdown() {
    if (!artifact || !selected || selected.state !== "COMPLETED") return;
    const report = renderResearchMarkdown(artifact, selected);
    const url = URL.createObjectURL(new Blob([report], { type: "text/markdown;charset=utf-8" }));
    const a = document.createElement("a"); a.href = url; a.download = `aegis-${selected.run_id}-report.md`; a.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1500);
  }
  // The create form lives below the report so reading stays the primary task;
  // keep a permanently visible entry point that scrolls straight to it.
  function focusNewResearch() {
    const form = document.querySelector(".live-create");
    if (form instanceof HTMLDetailsElement) form.open = true;
    form?.scrollIntoView({ behavior: "smooth", block: "center" });
    form?.querySelector("input")?.focus({ preventScroll: true });
  }

  if (!session) return <div className="live-gate"><Radar size={36} /><h1>AEGIS · Live Research</h1><p>正在验证本机研究工作台配置…</p>{error && <p role="alert">{error}</p>}</div>;
  if (!session.configured) return <div className="live-gate"><TriangleAlert size={34}/><h1>真实研究模式尚未配置</h1><p>请在 Next.js 服务端配置 Go API URL、API Token、Web 访问口令与会话密钥。不会自动切换为 Mock 数据。</p><p>参见 web/README.md</p></div>;
  if (!session.authenticated) return <div className="live-gate"><span className="brand-mark"><Radar size={24}/></span><div className="eyebrow">Research intelligence / secure access</div><h1>AEGIS · 研究工作台</h1><p>真实 Go/Eino 工作流 · 当前仅支持单操作者的受保护本地服务</p><form onSubmit={signIn} className="live-login"><label htmlFor="password">工作台访问口令</label><input id="password" autoComplete="current-password" type="password" minLength={16} value={password} onChange={e=>setPassword(e.target.value)} placeholder="输入服务端配置的访问口令" required/><button className="btn primary" disabled={busy} type="submit"><LockKeyhole size={15}/>{busy?"验证中…":"解锁研究工作台"}</button></form>{error && <div className="live-alert" role="alert">{error}</div>}<small>页面不会接收 Go API Token。访问口令仅提交给本站服务端建立 HttpOnly 会话。</small></div>;

  return <div className="workspace live-workspace">
    <aside className="sidebar" aria-label="实时研究导航">
      <div className="brand"><span className="brand-mark"><Radar size={24}/></span><div><div className="brand-title">AEGIS<span style={{color:"#7ceac7"}}>.</span></div><div className="brand-sub">研究判断与证据</div></div></div>
      <nav aria-label="研究导航"><div className="nav-label">工作空间</div><button type="button" onClick={()=>navigate("overview")} className={cn("nav-item",(view==="overview"||view==="workspace")&&"active")}><LayoutDashboard size={17}/><span>工作台</span></button><button type="button" onClick={()=>navigate("history")} className={cn("nav-item",view==="history"&&"active")}><FileClock size={17}/><span>研究档案</span></button><button type="button" onClick={()=>navigate("watchlist")} className={cn("nav-item",view==="watchlist"&&"active")}><Eye size={17}/><span>观察池</span></button><button type="button" onClick={()=>navigate("settings")} className={cn("nav-item",view==="settings"&&"active")}><ShieldCheck size={17}/><span>数据源设置</span></button></nav>
      <div className="sidebar-bottom"><div className="terminal-note"><ShieldCheck size={18}/><span><strong>研究数据可追溯</strong><br/>来自服务端记录，不含交易执行</span></div><button type="button" className="live-signout" onClick={()=>void signOut()}><LogOut size={15}/><span>退出会话</span></button></div>
    </aside>
    <main className="main-area"><header className="topbar"><div className="breadcrumb"><span>研究工作台</span><ChevronRight size={13}/><strong>{view==="overview"?"工作台":view==="history"?"研究档案":view==="compare"?"研究对比":view==="watchlist"?"观察池":view==="settings"?"数据源设置":"研究详情"}</strong></div><div className="topbar-spacer"/><span className={`badge ${listSyncError ? "red" : lastListSync ? "green" : "gray"}`} role="status"><CircleDot size={12}/>{listSyncError ? "研究库同步失败" : lastListSync ? "研究库已同步" : "研究库同步中"}</span><button className="icon-button" aria-label="刷新" type="button" onClick={()=>void refresh().catch(e=>{setListSyncError((e as Error).message);setError((e as Error).message);})}><RefreshCcw size={16}/></button></header>
    <div className={cn("content","live-content",view==="overview"&&"is-overview")}><div className="page-heading"><div><div className="eyebrow">AEGIS · 研究工作台</div><h1>{view==="overview"?"工作台":view==="history"?"研究档案":view==="compare"?"研究对比":view==="watchlist"?"观察池":view==="settings"?"数据源设置":"研究详情"}</h1><p>{view==="settings"?"管理模型与数据源凭证；密钥仅在服务端处理":view==="workspace"?"先看研究结论，再查看关键依据、风险与后续关注":view==="watchlist"?"保存标的、理由与关注条件，并从池内直接启动研究":"查看近期研究结果与任务状态；完整依据按需展开"}</p></div><div className="heading-right">{view==="workspace"&&<button type="button" className="btn primary" onClick={focusNewResearch}><Search size={14}/>发起新研究</button>}{view==="workspace"&&artifact&&selected?.state==="COMPLETED"&&<details className="live-export-menu"><summary className="btn"><Download size={14}/>导出</summary><div><button type="button" onClick={downloadMarkdown}>研究报告（Markdown）</button><button type="button" onClick={downloadArtifact}>原始工件（JSON）</button></div></details>}</div></div>
    <div className="live-context-note"><ShieldCheck size={14}/><span>研究截止时间以服务端记录为准 · 不包含交易执行</span></div>
    {error&&<div role="alert" className="live-alert"><TriangleAlert size={15}/>{error}<button onClick={()=>setError("")} type="button">关闭</button></div>}
    {info&&<div role="status" className="live-info">{info}</div>}
    {listSyncError&&view!=="overview"&&<div role="alert" className="live-alert"><TriangleAlert size={15}/>研究任务列表同步失败，当前记录可能已过期。<button type="button" onClick={()=>void refresh().catch(e=>setListSyncError((e as Error).message))}>重新同步</button></div>}
    {view==="settings"?<ProviderSettings localDev={session.local_dev === true}/>:view==="overview"?<LiveRunOverview runs={runs} syncedAt={lastListSync} syncError={listSyncError} onOpen={id=>{navigate("workspace",id);setError("");}} onNew={()=>{navigate("workspace",selectedId);window.setTimeout(focusNewResearch,0)}} onCompare={()=>navigate("compare")} onRefresh={()=>void refresh().catch(e=>{setListSyncError((e as Error).message);setError((e as Error).message);})}/>:view==="compare"?<LiveRunComparison runs={runs} pair={comparisonPair} onPairChange={openComparison}/>:view==="history"?<section className="surface live-history"><div className="panel-head"><h2>服务端持久化记录</h2><div className="live-history-actions"><span className="badge blue">已匹配 {visibleHistory?.total ?? "—"} 条 · 每页 25 条</span><button type="button" className="btn" onClick={()=>navigate("compare")}><ArrowLeftRight size={14}/>对比历史研究</button></div></div><div className="live-history-filters"><label>按股票代码筛选<input aria-label="按股票代码筛选" value={historySymbol} onChange={e=>{setHistorySymbol(e.target.value);setHistoryPage(0);}} placeholder="例如 600519 或 .SH"/></label><label>按任务状态筛选<select aria-label="按任务状态筛选" value={historyState} onChange={e=>{setHistoryState(e.target.value as "ALL" | LiveRun["state"]);setHistoryPage(0);}}><option value="ALL">全部状态</option>{(Object.keys(states) as LiveRun["state"][]).map(state=><option key={state} value={state}>{states[state]}</option>)}</select></label><button type="button" className="btn" disabled={!historySymbol && historyState==="ALL"} onClick={()=>{setHistorySymbol("");setHistoryState("ALL");setHistoryPage(0);}}>清除筛选</button></div>{historyError?<div role="alert" className="live-alert">历史查询失败：{historyError}<button type="button" onClick={()=>{setHistoryError(null);setHistoryRetry(n=>n+1);}}>重新查询</button></div>:!visibleHistory?<p className="live-empty">正在查询服务端研究历史…</p>:visibleHistory.total===0?<p className="live-empty">没有符合条件的研究记录。</p>:<div className="live-history-table">{visibleHistory.runs.map(r=><button key={r.run_id} className="live-history-row" onClick={()=>{setRuns(current=>current.some(item=>item.run_id===r.run_id)?current:[r,...current]);navigate("workspace",r.run_id);setError("");}} type="button"><strong>{r.symbol}</strong><code>{r.run_id.slice(0,12)}…</code><small>{formatDate(r.created_at)}</small><span className={`badge ${statusColors[r.state]}`}>{states[r.state]}</span>{r.state==="FAILED"&&<span className="live-history-failure" title="后端返回的失败代码，非根因诊断">{r.failure_code ? `错误：${r.failure_code}` : "错误代码未提供"}</span>}<ArrowRight size={15}/></button>)}</div>}<div className="live-history-pagination"><button type="button" className="btn" disabled={historyPage===0} onClick={()=>setHistoryPage(page=>Math.max(0,page-1))}>上一页</button><span>第 {historyPage+1} 页 · {visibleHistory?.total ?? 0} 条匹配记录</span><button type="button" className="btn" disabled={!visibleHistory?.hasMore} onClick={()=>setHistoryPage(page=>page+1)}>下一页</button></div></section>:view==="watchlist"?<WatchlistPanel onOpenRun={id=>{navigate("workspace",id);setError("");}} onStartResearch={name=>{void startResearchFor(name);}} researching={busy}/>:<div className="live-workspace-layout">
      {selected && <section className="surface live-current-run" aria-label="当前研究任务">
        <div className="live-current-run-main"><span className="eyebrow">当前研究</span><strong>{selected.symbol}</strong><small>研究时点 {formatDate(selected.requested_as_of)}</small></div>
        <span className={`badge ${statusColors[selected.state]}`}>{states[selected.state]}</span>
        <span className="live-current-run-note">研究结论优先展示 · 运行过程按需查看</span>
        <div className="live-current-run-actions">{!isTerminal(selected.state)?<button type="button" className="btn danger" disabled={busy} onClick={()=>void cancelRun()}><Ban size={14}/>请求取消</button>:<button type="button" className="btn" disabled={busy} onClick={prepareRerun}><RotateCcw size={14}/>再次研究该标的</button>}{selected.state==="COMPLETED"&&<button type="button" className="btn" onClick={()=>void compareWithPrevious(selected.run_id)}><ArrowLeftRight size={14}/>对比历史研究</button>}</div>
      </section>}
      <div className="live-primary-report">{selected?.state==="COMPLETED"&&artifact?<LiveResearchReport artifact={artifact} run={selected} onCompare={()=>void compareWithPrevious(selected.run_id)}/>:<section className="surface live-report-empty"><BookOpenText size={21}/><div><h2>研究结果</h2><p>{selected?.state==="COMPLETED"?"正在加载后端已完成工件…":selected?.state==="FAILED"||selected?.state==="CANCELLED"?"任务没有完成，研究报告不可用；不会显示上一次任务的结果。":"只有后端状态为 COMPLETED 才会展示真实报告。"}</p></div></section>}</div>
      <div className="live-secondary-grid">
        <details className="surface live-create" open={!selected || selected.state !== "COMPLETED"}>
          <summary>开始新的研究 <span>输入标的和本次关注点</span></summary>
          <div className="live-create-body"><div className="eyebrow">New research run</div><h2>发起股票研究</h2><p>研究时点由后端创建任务时确定。正式执行可能触发数据源请求和模型费用。</p>{rerunFrom&&<div className="live-rerun-notice" role="status"><strong>再次研究 · 来源任务 {rerunFrom.runId.slice(0, 12)}…</strong><span>仅预填标的，不会复制旧的关注点、账户状态或研究时间。请确认本次研究参数后手动启动全新任务。</span><button type="button" onClick={()=>{setRerunFrom(null);pendingStart.current=null;}}>取消再次研究</button></div>}<form onSubmit={startRun}><label>股票代码<input value={symbol} onChange={e=>{setSymbol(e.target.value.toUpperCase());setRerunFrom(null);pendingStart.current=null;}} placeholder="600519.SH" required/></label><label>研究关注点（可选）<input maxLength={1024} value={marketQuery} onChange={e=>setMarketQuery(e.target.value)} placeholder="例如：近期市场环境与标的事件"/></label><div className="live-create-foot"><span className="badge blue">{modeLabel(session.mode)}</span>{session.mode==="PORTFOLIO_REVIEW"&&<span className="badge gray">账户：NOT_PROVIDED · 未知</span>}<button className="btn primary" type="submit" disabled={busy}><Search size={15}/>{busy?"处理中…":"启动真实研究"}</button></div></form></div>
        </details>
        <details className="live-secondary-panel" open={!selected && Boolean(selectedId)}>
          <summary>研究信息 <span>状态、研究时点、请求约束与来源索引</span></summary>
          <div className="live-secondary-panel-body"><section className="surface live-run-detail"><div className="panel-head"><div><h2>任务运行详情</h2></div>{selected&&<span className={`badge ${statusColors[selected.state]}`}>{states[selected.state]}</span>}</div>{selected?<div className="live-run-fields"><div><small>标的</small><strong>{selected.symbol}</strong></div><div><small>研究时点 (PIT)</small><strong>{formatDate(selected.requested_as_of)}</strong></div><div><small>任务 ID</small><code>{selected.run_id}</code><a className="live-permalink" href={workbenchHref("/", "workspace", selected.run_id)}>任务固定链接 ↗</a></div><div><small>状态</small><strong>{states[selected.state]}</strong></div><div><small>开始</small><strong>{formatDate(selected.started_at)}</strong></div><div><small>结束</small><strong>{formatDate(selected.finished_at)}</strong></div>{selected.failure_code&&<div className="live-full"><small>错误代码</small><strong className="down">{selected.failure_code}</strong></div>}{selected.rerun_of&&<div className="live-full"><small>来源研究任务</small><a className="live-permalink" href={workbenchHref("/", "workspace", selected.rerun_of)}>{selected.rerun_of} ↗</a></div>}</div>:<p className="live-empty">{selectedId ? `找不到任务 ${selectedId}，或当前会话无权访问。不会显示其他任务的报告。` : "尚未选择任务。发起一次研究或从历史中打开。"}</p>}{selected&&<details className="live-request-snapshot">
        <summary>提交时研究配置快照 · {selected.request_snapshot?.schema_version===1?"已记录":"旧任务未记录"}</summary>
        {selected.request_snapshot?.schema_version===1?<div className="live-request-snapshot-grid">
          <div><small>研究关注点</small><strong>{selected.request_snapshot.market_query_provided?"已提交（正文不保存）":"未提供"}</strong></div>
          <div><small>证据截止时间</small><strong>{formatDate(selected.request_snapshot.event_window_end)}</strong></div>
          <div><small>证据检索起点</small><strong>{formatDate(selected.request_snapshot.event_window_start)}</strong></div>
          <div><small>时点安全要求</small><strong>{selected.request_snapshot.event_pit_safe?"要求 PIT-safe":"未声明 PIT-safe"}</strong></div>
          <div><small>请求的来源类型</small><strong>{selected.request_snapshot.preferred_source_types?.length?selected.request_snapshot.preferred_source_types.join("、"):"未声明"}</strong></div>
          <div><small>证据最少文档 / 独立来源</small><strong>{selected.request_snapshot.min_documents??"—"} / {selected.request_snapshot.min_independent_sources??"—"}</strong></div>
          {selected.request_snapshot.portfolio_state&&<div><small>提交时账户状态</small><strong>{selected.request_snapshot.portfolio_state==="NOT_PROVIDED"?"NOT_PROVIDED · 未知":selected.request_snapshot.portfolio_state}</strong></div>}
        </div>:<p>此任务创建时尚未保存请求配置快照；不会追溯推断过去的研究参数。</p>}
        <p>此处仅显示提交时已记录的请求约束，不代表实际命中的数据源，也不代表模型或 Provider 版本。未留存原始研究关注点及账户明细。</p>
      </details>}{selected&&<details className="live-model-provenance">
        <summary>本次运行模型配置 · {selected.model_provenance?.schema_version===1?"已记录":"未记录"}</summary>
        {selected.model_provenance?.schema_version===1?<div>
          <p>这是 Go 在本次成功构建研究流程后记录的模型配置身份；模型 ID 不是第三方 Provider 保证的固定版本号。</p>
          <div className="live-model-provenance-rows">
            {selected.model_provenance.bindings.map(binding=><div key={binding.role}>
              <strong>{binding.role}</strong>
              <span>{binding.profile}</span>
              <code>{binding.provider} / {binding.model}</code>
            </div>)}
          </div>
          <p>配置身份指纹（仅模型角色映射，不包含秘钥或端点）：<code>{selected.model_provenance.config_fingerprint}</code></p>
        </div>:<p>此任务未保存模型构建身份。可能是旧运行记录，或模型流程尚未成功初始化；不能据当前配置推断该任务当时使用的模型。</p>}
      </details>}{selected?.state==="COMPLETED"&&<LiveEvidenceSourceIndex index={selected.evidence_index} mode={selected.mode}/>}</section></div>
        </details>
        <details className="live-secondary-panel live-trace-panel">
          <summary>查看研究过程与审计信息 <span>阶段、事件与失败诊断</span></summary>
          <div className="live-secondary-panel-body"><LiveStageTrace run={selected} events={events} artifact={artifact} stream={streamState}/><section className="surface live-help"><div className="panel-head"><h2>研究安全边界</h2></div><p><Database size={14}/>结果仅来自 Go Typed Artifact</p><p><ShieldCheck size={14}/>账户未知不等于空仓或零现金</p><p><TerminalSquare size={14}/>无委托、资金划拨与自动交易</p></section></div>
        </details>
      </div>
    </div>}
    {session?.authenticated && <details id="research-follow-up" className="live-follow-up-disclosure"><summary>继续追问本次研究 <span>针对当前 Run 提问 · 默认只读</span></summary><ResearchConversationPanel selectedRunId={selectedId} onOpenRun={id => { navigate("workspace", id); setError(""); void refresh().catch(e => setError((e as Error).message)); }} /></details>}
    <footer className="live-footer">AEGIS RESEARCH · 研究结果仅供分析，不包含交易执行</footer></div></main></div>;
}
