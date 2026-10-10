"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, ArrowRight, ArrowUpRight, Bell, BookOpenText, Check, Ban, RotateCcw, Link2,
  CheckCircle2, ChevronRight, CircleDot, Database, Download, FileClock,
  LayoutDashboard, Layers3, MoreHorizontal, Play, Radar, Search,
  Settings2, ShieldCheck, SlidersHorizontal, Sparkles,
  TrendingUp, TriangleAlert, Zap,
} from "lucide-react";
import {
  Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts";
import {
  demoStocks, findStock, stages,
  type Evidence, type StockFixture, type StockId,
} from "@/lib/mock-research";
import {
  advanceDemoRun, cancelDemoRun, createDemoRun, demoStorageKey,
  describeDemoRun, formatDemoRunTime, initialDemoRuns, keepRecentDemoRuns,
  restoreDemoRuns, type DemoRun, type DemoScenario,
} from "@/lib/demo-run-store";
import { claimsForEvidence, claimsForStage, demoTraceForRun, type DemoResearchTrace } from "@/lib/research-trace";
import ResearchTracePanel from "@/components/research-trace-panel";
import { cn } from "@/lib/utils";

type View = "overview" | "research" | "history" | "settings";
type Timeframe = "1W" | "1M" | "3M";

function AegisMark() {
  return <span className="brand-mark" aria-hidden="true"><Radar size={24} strokeWidth={2.1} /></span>;
}

function ModeBadge() {
  return <span className="badge demo"><CircleDot size={11} /> MOCK DATA</span>;
}

function SmallPill({ children, tone = "gray" }: { children: React.ReactNode; tone?: "gray" | "green" | "red" | "blue" }) {
  return <span className={`badge ${tone}`}>{children}</span>;
}

function StockChart({ stock, timeframe, onChange }: { stock: StockFixture; timeframe: Timeframe; onChange: (t: Timeframe) => void }) {
  const data = useMemo(() => {
    const count = timeframe === "1W" ? 7 : timeframe === "1M" ? 18 : 25;
    return stock.closes.map((close, i) => ({
      day: `D${i + 1}`,
      close,
      volume: stock.volumes[i],
    })).slice(-count);
  }, [stock, timeframe]);
  const min = Math.floor(Math.min(...data.map(d => d.close)) * 0.985 * 10) / 10;
  const max = Math.ceil(Math.max(...data.map(d => d.close)) * 1.015 * 10) / 10;
  return (
    <section className="surface chart-panel" aria-label="模拟行情趋势图">
      <div className="panel-head">
        <div><h2>价格趋势 <span style={{ color: "#627e97", fontWeight: 450 }}> / Price action</span></h2><p>示例数据点 · 不代表真实交易日</p></div>
        <div className="chart-head-tools" aria-label="时间范围">
          {(["1W", "1M", "3M"] as const).map(t => <button type="button" aria-pressed={timeframe === t} className={cn(timeframe === t && "active")} onClick={() => onChange(t)} key={t}>{t}</button>)}
        </div>
      </div>
      <div className="chart-holder">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ left: 0, right: 0, top: 10, bottom: 0 }}>
            <defs>
              <linearGradient id="priceGradient" x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor="#65e1c4" stopOpacity={0.28} />
                <stop offset="100%" stopColor="#65e1c4" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid stroke="#2c3c50" strokeDasharray="3 5" vertical={false} opacity={0.42} />
            <XAxis dataKey="day" axisLine={false} tickLine={false} tick={{ fill: "#7087a2", fontSize: 10 }} tickMargin={12} interval="preserveStartEnd" />
            <YAxis orientation="right" domain={[min, max]} axisLine={false} tickLine={false} width={55} tick={{ fill: "#7087a2", fontSize: 10 }} tickFormatter={(v: number) => Number(v).toFixed(stock.price < 100 ? 2 : 0)} />
            <Tooltip contentStyle={{ background: "#20334a", border: "1px solid #496379", borderRadius: 9, fontSize: 12, color: "#e4f1fb" }} formatter={(value) => [Number(value ?? 0).toFixed(2), "演示价格"]} labelStyle={{ color: "#9eb2c7" }} />
            <Area isAnimationActive={false} type="monotone" dataKey="close" stroke="#69e2c0" strokeWidth={2.6} fill="url(#priceGradient)" dot={false} activeDot={{ r: 5, stroke: "#071525", strokeWidth: 2 }} />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      <div className="chart-legend"><span><i className="legend-dot" />示例价格走势</span><span><i className="legend-dot secondary" />研究窗口</span><span className="chart-hint">Synthetic OHLCV / DEMO</span></div>
    </section>
  );
}

function StockHero({ stock, isRunning, onRun }: { stock: StockFixture; isRunning: boolean; onRun: () => void }) {
  return (
    <section className="surface stock-hero">
      <div className="stock-top">
        <div>
          <h2 className="stock-name">{stock.name}</h2>
          <div className="stock-sub"><span>{stock.symbol}</span><i className="dot" /><span>{stock.sector}</span><i className="dot" /><span>沪深 A 股 · 演示</span></div>
        </div>
        <SmallPill tone="blue"><Activity size={12} /> 研究观察中</SmallPill>
      </div>
      <div className="stock-price"><strong>¥{stock.price.toFixed(2)}</strong><span className={stock.change > 0 ? "up" : "down"}>{stock.change > 0 ? "+" : ""}{stock.change.toFixed(2)}%</span></div>
      <div className="stock-note">模拟价格 / 非实时行情 · 数据演示日期：2026-09-24</div>
      <div className="hero-foot">
        <span className="tag">市场状态 · 示例</span><span className="tag">策略研究 · 结构分析</span>
        <span className="spacer" />
        <button className="btn primary" type="button" onClick={onRun} disabled={isRunning}>
          {isRunning ? <Activity size={15} /> : <Play size={14} fill="currentColor" />}{isRunning ? "正在模拟运行…" : "开始演示研究"}<ArrowRight size={14} />
        </button>
      </div>
    </section>
  );
}

function Metrics({ stock }: { stock: StockFixture }) {
  return (
    <div className="metric-grid">
      <section className="surface metric"><div className="metric-label">研究倾向 <TrendingUp size={13} /></div><div className="metric-value up">观察偏强</div><div className="metric-sub">基于虚构情景</div></section>
      <section className="surface metric"><div className="metric-label">论证指标 <Layers3 size={13} /></div><div className="metric-value">{stock.score}<span style={{ fontSize: 11, color: "#7e91ac" }}> / 100</span></div><div className="metric-sub">UI 示例评分，未经校准</div></section>
      <section className="surface metric"><div className="metric-label">研究覆盖 <ShieldCheck size={13} /></div><div className="metric-value">3<span style={{ fontSize: 11, color: "#7e91ac" }}> 条证据</span></div><div className="metric-sub">模拟引用，非真实来源</div></section>
    </div>
  );
}

function InsightPanel({ stock, onNavigate }: { stock: StockFixture; onNavigate: () => void }) {
  return (
    <section className="surface summary-panel">
      <div className="summary-head"><Sparkles size={14} /> Research synthesis · 演示</div>
      <h2>研究摘要</h2>
      <p>{stock.researchSummary}</p>
      <div className="summary-foot"><SmallPill tone="green">趋势结构 · 观察</SmallPill><SmallPill tone="gray">账户状态 · 未提供</SmallPill>
        <button type="button" className="btn ghost" onClick={onNavigate} style={{ marginLeft: "auto", padding: "6px 9px" }}>查看研究报告 <ArrowRight size={13} /></button>
      </div>
    </section>
  );
}

function AgentPanel({ run, trace, selected, onSelect, onSelectClaim }: { run: DemoRun | null; trace: DemoResearchTrace | null; selected: string; onSelect: (s: string) => void; onSelectClaim: (id: string) => void }) {
  const current = stages.find(s => s.id === selected) ?? stages[0];
  const completed = run?.completedStages ?? 0;
  const status = run?.status ?? "PENDING";
  const running = status === "RUNNING";
  return (
    <section className="surface run-panel">
      <div className="panel-head">
        <div><h2>Agent 运行链路</h2><p>{run ? `${run.id} · Demo graph` : "当前股票没有选中研究任务"}</p></div>
        <div className="run-stat"><i className={cn("status-dot", running && "running", status === "FAILED" && "failed", status === "CANCELLED" && "cancelled")} />{run ? describeDemoRun(run) : "未开始"}</div>
      </div>
      <div className="progress-bar" role="progressbar" aria-label="模拟研究进度" aria-valuenow={completed} aria-valuemin={0} aria-valuemax={stages.length}><div style={{ width: `${Math.round(completed / stages.length * 100)}%` }} /></div>
      <div className="stage-list">
        {stages.map((stage, index) => {
          const state = index < completed ? "done" :
            index === completed && running ? "running" :
            index === completed && status === "FAILED" ? "failed" : "pending";
          return <button type="button" key={stage.id} className={cn("stage", state, selected === stage.id && "selected")} aria-pressed={selected === stage.id} onClick={() => onSelect(stage.id)}>
            <span className="stage-icon">{state === "done" ? <Check size={15} /> : state === "failed" ? <TriangleAlert size={14} /> : stage.short}</span>
            <span className="copy"><strong>{stage.name}</strong><small>{stage.title}</small></span>
            {state === "done" ? <span className="stage-time">{stage.duration}</span> : state === "running" ? <span className="stage-time">执行中</span> : state === "failed" ? <span className="stage-time">失败</span> : <span className="stage-time">等待</span>}
          </button>;
        })}
      </div>
      <div className="stage-detail"><strong style={{ color: "#cbd8e7" }}>{current.name}</strong><br />
        {run && stages.findIndex(item => item.id === current.id) < completed
          ? current.detail
          : "本次阶段尚未完成，没有可查看的研究产物。"}<br />
        {run?.failureStage === current.id && <span className="stage-error">{run.failureReason}<br /></span>}
        <span style={{ color: "#5e9f9b" }}>数据源：DEMO / 仅用于页面演示</span>
        {trace && claimsForStage(trace, current.id).length > 0 && <div className="stage-linked-claims"><span>关联观点（示例）</span>{claimsForStage(trace, current.id).map(claim => <button type="button" key={claim.id} onClick={() => onSelectClaim(claim.id)}><Link2 size={12} /> {claim.title} <ArrowRight size={12} /></button>)}</div>}
      </div>
      {run?.status === "FAILED" && <div className="run-failure" role="alert"><TriangleAlert size={14}/><span>{run.failureReason}</span></div>}
    </section>
  );
}

function EvidencePanel({ stock, trace, selected, selectedClaimId, onSelect, onSelectClaim }: {
  stock: StockFixture; trace: DemoResearchTrace | null; selected: string; selectedClaimId: string;
  onSelect: (id: string) => void; onSelectClaim: (id: string) => void;
}) {
  const active: Evidence | undefined = trace ? stock.evidence.find(e => e.id === selected) ?? stock.evidence[0] : undefined;
  const related = trace && active ? claimsForEvidence(trace, active.id) : [];
  return (
    <section className="surface evidence-panel" id="evidence-inspector">
      <div className="panel-head"><div><h2>证据检查</h2><p>论点追踪 · Evidence Inspector</p></div><span style={{ color: "#869eb6", fontSize: 11 }}>{trace ? `${stock.evidence.length} 条 · DEMO` : "尚无证据"}</span></div>
      {!trace ? <div className="evidence-empty"><BookOpenText size={19}/><p>本次研究尚未成功完成。为避免把股票静态样例当作本次运行证据，暂不展示关联引用。</p></div> : <>
        <div className="evidence-list">
          {stock.evidence.map(entry => <button key={entry.id} type="button" className={cn("evidence-entry", selected === entry.id && "active")} onClick={() => onSelect(entry.id)} aria-pressed={selected === entry.id}>
            <span className={cn("evidence-icon", entry.kind)}>{entry.kind === "support" ? <CheckCircle2 size={14} /> : entry.kind === "risk" ? <TriangleAlert size={14} /> : <BookOpenText size={14} />}</span>
            <span style={{ flex: 1 }}><strong>{entry.title}</strong><small>{entry.category} · {entry.id}</small></span><ChevronRight size={14} color="#6e8aa1" />
          </button>)}
        </div>
        {active && <div className="evidence-expanded">
          <div className="reference-provenance">{active.id} · SYNTHETIC REFERENCE</div>
          <h3>{active.title}</h3><p>{active.note}</p>
          <div className="reference-meta"><span>来源：{active.source}</span><span>样例截止：{active.asOf}</span></div>
          <div className="evidence-related"><strong>关联研究观点 · {related.length}</strong>
            {related.length ? related.map(claim => <button className={cn("related-claim", selectedClaimId === claim.id && "active")} key={claim.id} type="button" onClick={() => onSelectClaim(claim.id)}><Link2 size={12}/>{claim.title}<ArrowRight size={12}/></button>) : <small>该引用未被任何观点使用</small>}
          </div>
        </div>}
      </>}
    </section>
  );
}

function HistoryPanel({ runs, onViewAll, onOpen }: { runs: DemoRun[]; onViewAll: () => void; onOpen: (id: string) => void }) {
  return (
    <section className="surface history-panel">
      <div className="panel-head"><div><h2>最近研究</h2><p>Research archive · 演示</p></div><button type="button" className="icon-button" onClick={onViewAll} aria-label="查看全部历史"><ArrowUpRight size={16} /></button></div>
      <div className="history-list">
        {runs.slice(0, 3).map(h => <button type="button" key={h.id} className="history-row" onClick={() => onOpen(h.id)}>
          <span className="evidence-icon neutral"><FileClock size={15} /></span><span className="history-text"><strong>{findStock(h.symbol).name}</strong><small>{formatDemoRunTime(h)} · {describeDemoRun(h)}</small></span><ChevronRight size={15} color="#70869f" />
        </button>)}
      </div>
      <div className="pale-foot">本地演示记录 · 无真实研究产物</div>
    </section>
  );
}

function HistoryView({ runs, onOpen }: { runs: DemoRun[]; onOpen: (id: string) => void }) {
  const [filter, setFilter] = useState<"ALL" | "COMPLETED" | "FAILED" | "RUNNING" | "CANCELLED">("ALL");
  const filtered = filter === "ALL" ? runs : runs.filter(h => h.status === filter);
  const labels: Array<{ value: typeof filter; label: string }> = [
    { value: "ALL", label: "全部" }, { value: "COMPLETED", label: "已完成" },
    { value: "RUNNING", label: "进行中" }, { value: "FAILED", label: "失败" },
    { value: "CANCELLED", label: "已取消" },
  ];
  return <div className="view-page">
    <div className="surface detail-intro"><div className="eyebrow">Research / Archive</div><h2>研究历史</h2><p>每次 Mock 运行都有独立 ID、状态和阶段记录。点击历史可以打开当时的模拟任务；数据保存在本地浏览器，不是 Go Agent 归档。</p></div>
    <div className="history-filters" aria-label="过滤研究状态">{labels.map(item => <button key={item.value} type="button" className={cn("filter-chip", filter === item.value && "active")} aria-pressed={filter === item.value} onClick={() => setFilter(item.value)}>{item.label}</button>)}</div>
    <div className="surface table-wrap"><table className="data-table"><thead><tr><th>股票标的</th><th>研究时间（演示）</th><th>研究结论 / 失败原因</th><th>任务状态</th><th>阶段</th><th>详情</th></tr></thead><tbody>{filtered.map(h => <tr key={h.id}><td><strong>{findStock(h.symbol).name}</strong><div style={{ color: "#728ba4", fontSize: 10, marginTop: 5 }}>{h.symbol}</div></td><td style={{ color: "#9fb2c6" }}>{formatDemoRunTime(h)}<div className="run-id-short">{h.id}</div></td><td>{h.failureReason || h.headline || "前端模拟研究"}</td><td><SmallPill tone={h.status === "FAILED" ? "red" : h.status === "COMPLETED" ? "green" : "gray"}>{describeDemoRun(h)}</SmallPill></td><td style={{ color: "#9fb2c6" }}>{h.completedStages}/{stages.length}</td><td><button className="btn ghost" type="button" onClick={() => onOpen(h.id)} aria-label={`打开 ${h.id}`}><ArrowUpRight size={15} /> 打开</button></td></tr>)}</tbody></table>
      {filtered.length === 0 && <div className="empty-notice">此状态下没有演示任务。可返回研究工作区启动新的模拟运行。</div>}</div>
    <div className="surface empty-notice">所有价格与结论依然是固定模拟样本。只有状态为「演示完成」的运行，才能查看对应样本报告；失败或取消的运行不生成研究结论。</div>
  </div>;
}

function SettingsView() {
  return <div className="view-page">
    <div className="surface detail-intro"><div className="eyebrow">Workspace / Configuration</div><h2>工作区与数据源</h2><p>当前处于前端演示模式。研究来源、模型和 Go 服务尚未接入，以下状态不可视为生产环境的健康检查。</p></div>
    <div className="settings-grid">
      <section className="surface settings-card"><h3><Database size={17} color="#82e2c5" /> 服务连接</h3><p>此版本无需 API key 或真实市场数据，仅验证前端界面和交互流程。</p><div className="provider-row"><span>UI Data Source</span><SmallPill tone="green">LOCAL MOCK</SmallPill></div><div className="provider-row"><span>Aegis Go API</span><SmallPill>未接入</SmallPill></div><div className="provider-row"><span>Agent 运行事件</span><SmallPill>模拟阶段</SmallPill></div></section>
      <section className="surface settings-card"><h3><ShieldCheck size={17} color="#82e2c5" /> 研究与交易边界</h3><p>所有演示股票、价格、观点、评分和历史任务均为模拟内容。Aegis 只输出研究性判断，不提供任何券商委托或自动调仓。</p><div className="provider-row"><span>交易执行</span><SmallPill tone="gray">不支持</SmallPill></div><div className="provider-row"><span>组合账户</span><SmallPill>NOT_PROVIDED</SmallPill></div><div className="provider-row"><span>数据时点</span><SmallPill>演示 2026-09-24</SmallPill></div></section>
    </div>
    <section className="surface settings-card"><h3><SlidersHorizontal size={17} color="#82e2c5" /> UI 样式说明</h3><p>深色金融研究终端视觉系统 · Tailwind CSS + 自定义语义 token · Lucide 图标 · Recharts 图表。已预留 shadcn/ui 组件配置，真实后台接入将在后续独立开发中完成。</p></section>
  </div>;
}

export default function ResearchWorkbench() {
  const [view, setView] = useState<View>("overview");
  const [symbol, setSymbol] = useState<StockId>("600519.SH");
  const [query, setQuery] = useState("");
  const [timeframe, setTimeframe] = useState<Timeframe>("3M");
  const [selectedStage, setSelectedStage] = useState("debate");
  const [selectedEvidence, setSelectedEvidence] = useState("EV-001");
  const [selectedClaimId, setSelectedClaimId] = useState("");
  const [runs, setRuns] = useState<DemoRun[]>(initialDemoRuns);
  const [runsLoaded, setRunsLoaded] = useState(false);
  const [activeRunId, setActiveRunId] = useState<string | null>(initialDemoRuns[0].id);
  const [scenario, setScenario] = useState<DemoScenario>("success");
  const [toast, setToast] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const stock = findStock(symbol);
  const selectedRun = runs.find(run => run.id === activeRunId) ?? null;
  const isRunning = selectedRun?.status === "RUNNING";
  const hasCompletedReport = selectedRun?.status === "COMPLETED";
  const trace = demoTraceForRun(selectedRun, stock);

  // Browser-only demo data; server rendering starts with deterministic fixtures.
  useEffect(() => {
    let active = true;
    // Defer synchronization of the browser-only store until after hydration.
    queueMicrotask(() => {
      if (!active) return;
      let saved: string | null = null;
      try { saved = window.localStorage.getItem(demoStorageKey); }
      catch { /* Private browsing can disable storage. */ }
      const restored = restoreDemoRuns(saved);
      setRuns(restored);
      setActiveRunId(restored[0]?.id ?? null);
      setRunsLoaded(true);
    });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!runsLoaded) return;
    try { window.localStorage.setItem(demoStorageKey, JSON.stringify(runs)); }
    catch { /* A private browser may disable storage; keep the in-memory demo usable. */ }
  }, [runs, runsLoaded]);

  // Tick every active local mock run, not just the selected stock. Switching views
  // must never leave a fake Agent run stuck or complete an unrelated stock.
  useEffect(() => {
    if (!runs.some(run => run.status === "RUNNING")) return;
    const timer = window.setInterval(() => {
      setRuns(current => current.map(advanceDemoRun));
    }, 760);
    return () => window.clearInterval(timer);
  }, [runs]);

  useEffect(() => {
    if (!toast) return;
    const timeout = window.setTimeout(() => setToast(""), 4000);
    return () => window.clearTimeout(timeout);
  }, [toast]);

  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault(); searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, []);

  const chooseStock = (next: StockId) => {
    setSymbol(next);
    setQuery("");
    const mostRecent = runs.find(run => run.symbol === next);
    setActiveRunId(mostRecent?.id ?? null);
    setSelectedEvidence(findStock(next).evidence[0].id);
    setSelectedClaimId("");
    setSelectedStage(mostRecent?.status === "FAILED" ? mostRecent.failureStage ?? "event" : "debate");
    setView("overview");
    setToast(`已切换至 ${findStock(next).name}（演示样本）`);
  };

  const openRun = (id: string) => {
    const run = runs.find(item => item.id === id);
    if (!run) { setToast("当前演示历史中不存在该任务。"); return; }
    setSymbol(run.symbol);
    setActiveRunId(id);
    setSelectedStage(run.failureStage ?? (run.status === "RUNNING" ? stages[Math.min(run.completedStages, stages.length - 1)].id : "debate"));
    setSelectedEvidence(findStock(run.symbol).evidence[0].id);
    setSelectedClaimId("");
    setView("research");
    setToast(`正在查看 ${id} · ${describeDemoRun(run)}`);
  };

  const submitSearch = () => {
    const q = query.trim().toUpperCase();
    const found = demoStocks.find(item => item.symbol === q || item.symbol.startsWith(q) || item.name.includes(query.trim()));
    if (!q) { searchRef.current?.focus(); return; }
    if (!found) { setToast("当前 Mock 仅支持 600519.SH、000001.SZ 和 300750.SZ。未访问证券服务。"); return; }
    chooseStock(found.symbol);
  };

  const startDemo = (nextScenario: DemoScenario = scenario) => {
    if (!runsLoaded) { setToast("正在加载本地演示记录。"); return; }
    const created = createDemoRun(symbol, nextScenario);
    setRuns(current => keepRecentDemoRuns([created, ...current]));
    setActiveRunId(created.id);
    setSelectedStage("market");
    setSelectedClaimId("");
    setView("research");
    setToast(nextScenario === "provider_failure" ? "正在模拟 Event 数据源失败与安全停止。" : "本地模拟运行已开始，没有调用真实 Go Agent。");
  };

  const focusClaim = (id: string) => {
    if (!trace) return;
    const claim = trace.claims.find(item => item.id === id);
    if (!claim) return;
    setSelectedClaimId(id);
    setSelectedStage(claim.stageId);
    const validRef = claim.evidenceIds.find(ref => stock.evidence.some(item => item.id === ref));
    if (validRef) setSelectedEvidence(validRef);
    setView("research");
  };

  const focusEvidence = (id: string) => {
    if (!trace || !stock.evidence.some(item => item.id === id)) return;
    setSelectedEvidence(id);
    const matchingClaim = claimsForEvidence(trace, id)[0];
    if (matchingClaim) {
      setSelectedClaimId(matchingClaim.id);
      setSelectedStage(matchingClaim.stageId);
    } else setSelectedClaimId("");
  };

  const cancelRun = () => {
    if (!selectedRun || selectedRun.status !== "RUNNING") return;
    setRuns(current => current.map(run => run.id === selectedRun.id ? cancelDemoRun(run) : run));
    setToast("已取消当前模拟任务；不会生成本次研究结论。");
  };

  const downloadReport = () => {
    if (!selectedRun || selectedRun.status !== "COMPLETED") {
      setToast("只能导出已完成的演示研究；失败、取消、运行中的任务没有报告。");
      return;
    }
    const reportStock = findStock(selectedRun.symbol);
    const file = {
      demo: true, warning: "SYNTHETIC UI FIXTURE — NOT FINANCIAL ADVICE — NO LIVE MARKET DATA",
      run_id: selectedRun.id, status: selectedRun.status, symbol: reportStock.symbol,
      started_at: selectedRun.createdAt, research_as_of: "2026-09-24",
      source: "frontend-local-mock", conclusion: reportStock.researchSummary,
      bull: reportStock.bull, bear: reportStock.bear, risk: reportStock.risk,
      portfolio_context: { state: "NOT_PROVIDED" }, evidence: reportStock.evidence,
      research_trace: demoTraceForRun(selectedRun, reportStock),
    };
    const url = URL.createObjectURL(new Blob([JSON.stringify(file, null, 2)], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url; link.download = `aegis-${selectedRun.id}.json`;
    document.body.appendChild(link); link.click(); link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1500);
    setToast("已导出明确标注为演示数据的 JSON 文件。");
  };

  const sectionTitle = view === "history" ? "研究历史" : view === "settings" ? "工作区设置" : view === "research" ? "研究详情" : "研究总览";
  const navItems: Array<{ id: View; label: string; icon: typeof LayoutDashboard }> = [
    { id: "overview", label: "研究总览", icon: LayoutDashboard },
    { id: "research", label: "研究工作区", icon: Radar },
    { id: "history", label: "研究历史", icon: FileClock },
    { id: "settings", label: "设置", icon: Settings2 },
  ];

  return <div className="workspace">
    <aside className="sidebar" aria-label="主导航">
      <div className="brand"><AegisMark /><div><div className="brand-title">AEGIS<span style={{ color: "#7ceac7" }}>.</span></div><div className="brand-sub">Research intelligence</div></div></div>
      <nav aria-label="研究导航"><div className="nav-label">WORKSPACE</div>{navItems.map(item => <button key={item.id} type="button" className={cn("nav-item", view === item.id && "active")} onClick={() => setView(item.id)} aria-current={view === item.id ? "page" : undefined}><item.icon size={17} strokeWidth={1.9} /><span>{item.label}</span>{item.id === "history" && <span className="nav-count">{runs.length}</span>}</button>)}</nav>
      <div className="sidebar-sep" />
      <div className="watch-title">快速切换 <span>DEMO</span></div>
      {demoStocks.map(item => <button key={item.symbol} type="button" className={cn("watch-item", symbol === item.symbol && "chosen")} onClick={() => chooseStock(item.symbol)}><div><strong>{item.name}</strong><small>{item.symbol}</small></div><span className={cn("watch-change", item.change >= 0 ? "up" : "down")}>{item.change >= 0 ? "+" : ""}{item.change.toFixed(2)}%</span></button>)}
      <div className="sidebar-bottom"><div className="terminal-note"><Zap size={17} /><span><strong>UI Preview</strong><br />前端 Mock · 无实时交易或模型连接</span></div><div className="user"><div className="avatar">DE</div><div><strong>Demo workspace</strong><small>Local development</small></div><MoreHorizontal size={16} style={{ marginLeft: "auto", color: "#7c91a9" }} /></div></div>
    </aside>

    <main className="main-area">
      <header className="topbar">
        <div className="breadcrumb"><span>Workspace</span><ChevronRight size={13} /><strong>{sectionTitle}</strong></div>
        <div className="topbar-spacer" />
        <div className="searchbox"><Search size={15} /><input ref={searchRef} value={query} onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === "Enter") submitSearch(); }} aria-label="股票代码或名称" placeholder="搜索股票代码或名称" /><span className="kbd">⌘ K</span></div>
        <button className="icon-button" type="button" onClick={submitSearch} aria-label="提交搜索"><ArrowRight size={17} /></button>
        <button className="icon-button" type="button" onClick={() => setToast("Mock 模式没有实时通知。")} aria-label="通知"><Bell size={18} /></button>
        <span className="profile-mini" aria-label="演示用户">DE</span>
      </header>

      <div className="content">
        <div className="page-heading">
          <div><div className="eyebrow">Investment research / Workspace</div><h1>{sectionTitle}</h1><p>{view === "history" ? "回顾研究过程与结论，不让研究只停留在一次执行。" : view === "settings" ? "查看演示环境以及未来 API 接入边界。" : "让每一次研究都可观察、可解释、可追溯。"}</p></div>
          <div className="heading-right"><ModeBadge /><button type="button" className="btn" onClick={downloadReport} disabled={!hasCompletedReport} title={!hasCompletedReport ? "只有已完成的演示任务可导出" : undefined}><Download size={15} />导出演示 JSON</button></div>
        </div>

        <div className="demo-banner"><TriangleAlert size={16} /><span><strong>演示环境：</strong> 股票价格、走势、观点、Agent 进度与历史任务均为虚构样本。未接入真实行情、LLM、Go API 或账户。</span><CircleDot size={12} /></div>

        {view === "history" ? <HistoryView runs={runs} onOpen={openRun} /> : view === "settings" ? <SettingsView /> : <div className="main-grid">
          <div className="left-stack">
            <StockHero stock={stock} isRunning={isRunning} onRun={() => startDemo()} />
            <section className="surface run-toolbar" aria-label="演示任务控制">
              <div className="run-toolbar-meta"><span>当前任务</span><strong>{selectedRun?.id ?? "未选择"}</strong><small>{selectedRun ? `${describeDemoRun(selectedRun)} · ${formatDemoRunTime(selectedRun)}` : "切换股票或运行一次演示"}</small></div>
              <div className="run-toolbar-actions">
                <label className="demo-scenario">演示场景 <select aria-label="下一次研究的模拟场景" value={scenario} onChange={event => setScenario(event.target.value as DemoScenario)}><option value="success">正常完成</option><option value="provider_failure">数据源失败</option></select></label>
                <button type="button" className="btn" onClick={() => startDemo()} disabled={!runsLoaded || isRunning}><RotateCcw size={14} />{selectedRun?.status === "FAILED" || selectedRun?.status === "CANCELLED" ? "重新演示" : "新建演示"}</button>
                {isRunning && <button type="button" className="btn danger" onClick={cancelRun}><Ban size={14} />取消任务</button>}
              </div>
              {selectedRun?.status === "FAILED" && <div className="run-toolbar-warning" role="alert">{selectedRun.failureReason}</div>}
              {selectedRun?.status === "CANCELLED" && <div className="run-toolbar-warning">当前模拟任务已取消。可选择重新演示。</div>}
            </section>
            <Metrics stock={stock} />
            <StockChart stock={stock} timeframe={timeframe} onChange={setTimeframe} />
            {hasCompletedReport ? <>
              <InsightPanel stock={stock} onNavigate={() => setView("research")} />
              {trace && <ResearchTracePanel trace={trace} stock={stock} selectedClaimId={selectedClaimId} onSelectClaim={focusClaim} onSelectEvidence={focusEvidence} />}
            </> : <section className="surface report-placeholder"><FileClock size={19}/><div><h2>本次研究尚无有效结论</h2><p>{isRunning ? "Agent 演示正在运行；本次报告会在全部阶段模拟完成后显示。" : selectedRun?.status === "FAILED" ? "模拟数据源失败后，流程已安全停止。本次不生成研究报告。" : selectedRun?.status === "CANCELLED" ? "任务已经取消，本次不生成研究报告。" : "选择已完成的演示任务，或者启动一次模拟研究。"}</p></div></section>}
            {view === "research" && hasCompletedReport && <section className="surface detail-intro"><div className="eyebrow">Research artifact / Demo</div><h2>完整研究结论</h2><p>Research Manager：{stock.researchSummary}</p><p style={{ marginTop: 13 }}>Risk Analyst：{stock.risk}</p><p style={{ marginTop: 13, color: "#81d5bb" }}>组合状态：NOT_PROVIDED · 当前仓位未知 · 不生成目标仓位。</p><div style={{ marginTop: 17, display: "flex", gap: 9, flexWrap: "wrap" }}><SmallPill tone="blue">TradeIntent · MONITOR（演示）</SmallPill><SmallPill>Portfolio · REVIEW_REQUIRED（演示）</SmallPill></div></section>}
          </div>
          <div className="right-stack">
            <AgentPanel run={selectedRun} trace={trace} selected={selectedStage} onSelect={setSelectedStage} onSelectClaim={focusClaim} />
            <EvidencePanel stock={stock} trace={trace} selected={selectedEvidence} selectedClaimId={selectedClaimId} onSelect={focusEvidence} onSelectClaim={focusClaim} />
            <HistoryPanel runs={runs} onViewAll={() => setView("history")} onOpen={openRun} />
          </div>
        </div>}
        <footer style={{ marginTop: 24, fontSize: 11, color: "#698099", textAlign: "right" }}>AEGIS RESEARCH WORKSPACE · MOCK PREVIEW · 研究辅助，不含交易执行</footer>
      </div>
    </main>
    {toast && <div role="status" className="toast">{toast}</div>}
  </div>;
}
