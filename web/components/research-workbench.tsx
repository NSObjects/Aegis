"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, ArrowRight, ArrowUpRight, Bell, BookOpenText, Check,
  CheckCircle2, ChevronRight, CircleDot, Database, Download, FileClock,
  LayoutDashboard, Layers3, MoreHorizontal, Play, Radar, Search,
  Settings2, ShieldCheck, SlidersHorizontal, Sparkles, TrendingDown,
  TrendingUp, TriangleAlert, Zap,
} from "lucide-react";
import {
  Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts";
import {
  demoHistory, demoStocks, findStock, stages,
  type Evidence, type StockFixture, type StockId,
} from "@/lib/mock-research";
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
            <Tooltip contentStyle={{ background: "#20334a", border: "1px solid #496379", borderRadius: 9, fontSize: 12, color: "#e4f1fb" }} formatter={(v: number | undefined) => [Number(v ?? 0).toFixed(2), "演示价格"]} labelStyle={{ color: "#9eb2c7" }} />
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

function DebatePanel({ stock }: { stock: StockFixture }) {
  return (
    <section className="surface">
      <div className="panel-head"><div><h2>多空研究辩论</h2><p>观点碰撞 · Bull / Bear debate</p></div><SmallPill tone="gray">2 组观点 · 演示</SmallPill></div>
      <div className="debate-grid">
        <article className="debate"><div className="debate-title"><TrendingUp size={15} /> BULL · 多头观点</div><p>{stock.bull}</p><small>论点引用 · EV / sample</small></article>
        <article className="debate bear"><div className="debate-title"><TrendingDown size={15} /> BEAR · 空头观点</div><p>{stock.bear}</p><small>反证与失效条件 · sample</small></article>
      </div>
    </section>
  );
}

function AgentPanel({ completed, running, selected, onSelect }: { completed: number; running: boolean; selected: string; onSelect: (s: string) => void }) {
  const current = stages.find(s => s.id === selected) ?? stages[0];
  return (
    <section className="surface run-panel">
      <div className="panel-head">
        <div><h2>Agent 运行链路</h2><p>研究任务 · Demo graph</p></div>
        <div className="run-stat"><i className={cn("status-dot", running && "running")} />{running ? "演示运行中" : "演示已完成"}</div>
      </div>
      <div className="progress-bar" aria-label="模拟研究进度"><div style={{ width: `${Math.round(completed / stages.length * 100)}%` }} /></div>
      <div className="stage-list">
        {stages.map((stage, index) => {
          const state = index < completed ? "done" : index === completed && running ? "running" : "pending";
          return <button type="button" key={stage.id} className={cn("stage", state, selected === stage.id && "selected")} aria-pressed={selected === stage.id} onClick={() => onSelect(stage.id)}>
            <span className="stage-icon">{state === "done" ? <Check size={15} /> : stage.short}</span>
            <span className="copy"><strong>{stage.name}</strong><small>{stage.title}</small></span>
            {state === "done" ? <span className="stage-time">{stage.duration}</span> : state === "running" ? <span className="stage-time">执行中</span> : <span className="stage-time">等待</span>}
          </button>;
        })}
      </div>
      <div className="stage-detail"><strong style={{ color: "#cbd8e7" }}>{current.name}</strong><br />{current.detail}<br /><span style={{ color: "#5e9f9b" }}>数据源：DEMO / 仅用于页面演示</span></div>
    </section>
  );
}

function EvidencePanel({ stock, selected, onSelect }: { stock: StockFixture; selected: string; onSelect: (id: string) => void }) {
  const active: Evidence | undefined = stock.evidence.find(e => e.id === selected) ?? stock.evidence[0];
  return (
    <section className="surface evidence-panel">
      <div className="panel-head"><div><h2>研究证据</h2><p>可追溯信息 · Evidence</p></div><span style={{ color: "#869eb6", fontSize: 11 }}>{stock.evidence.length} 条</span></div>
      <div style={{ paddingTop: 10 }}>
        {stock.evidence.map((entry) => <button key={entry.id} type="button" className={cn("evidence-entry", selected === entry.id && "active")} onClick={() => onSelect(entry.id)} aria-pressed={selected === entry.id}>
          <span className={cn("evidence-icon", entry.kind)}>{entry.kind === "support" ? <CheckCircle2 size={14} /> : entry.kind === "risk" ? <TriangleAlert size={14} /> : <BookOpenText size={14} />}</span>
          <span style={{ flex: 1 }}><strong>{entry.title}</strong><small>{entry.category} · {entry.id}</small></span><ChevronRight size={14} color="#6e8aa1" />
        </button>)}
      </div>
      {active && <div className="evidence-expanded"><h3>{active.title}</h3><p>{active.note}</p><small>{active.source} · {active.asOf}</small></div>}
    </section>
  );
}

function HistoryPanel({ onViewAll, onOpen }: { onViewAll: () => void; onOpen: (symbol: StockId) => void }) {
  return (
    <section className="surface history-panel">
      <div className="panel-head"><div><h2>最近研究</h2><p>Research archive · 演示</p></div><button type="button" className="icon-button" onClick={onViewAll} aria-label="查看全部历史"><ArrowUpRight size={16} /></button></div>
      <div className="history-list">
        {demoHistory.slice(0, 3).map(h => <button type="button" key={h.id} className="history-row" onClick={() => onOpen(h.symbol)}>
          <span className="evidence-icon neutral"><FileClock size={15} /></span><span className="history-text"><strong>{findStock(h.symbol).name}</strong><small>{h.when}</small></span><ChevronRight size={15} color="#70869f" />
        </button>)}
      </div>
      <div className="pale-foot">历史记录为固定演示样本</div>
    </section>
  );
}

function HistoryView({ onOpen }: { onOpen: (symbol: StockId) => void }) {
  return <div className="view-page">
    <div className="surface detail-intro"><div className="eyebrow">Research / Archive</div><h2>研究历史</h2><p>比较每次研究的标的、结论和状态。当前展示的是固定的演示样本；持久化 API 将在 Go 服务阶段对接。</p></div>
    <div className="surface table-wrap"><table className="data-table"><thead><tr><th>股票标的</th><th>研究时间（示例）</th><th>研究结论</th><th>任务状态</th><th>耗时</th><th></th></tr></thead><tbody>{demoHistory.map(h => <tr key={h.id} tabIndex={0} onClick={() => onOpen(h.symbol)} onKeyDown={event => { if (event.key === "Enter") onOpen(h.symbol); }}><td><strong>{findStock(h.symbol).name}</strong><div style={{ color: "#728ba4", fontSize: 10, marginTop: 5 }}>{h.symbol}</div></td><td style={{ color: "#9fb2c6" }}>{h.when}</td><td>{h.headline}</td><td><SmallPill tone={h.state === "FAILED" ? "red" : "green"}>{h.state === "FAILED" ? "演示失败" : "演示完成"}</SmallPill></td><td style={{ color: "#9fb2c6" }}>{h.duration}</td><td><ArrowUpRight size={15} /></td></tr>)}</tbody></table></div>
    <div className="surface empty-notice">注意：这是用于验证表格操作的前端 Mock。点击历史记录会切换到相应股票的示例研究，而非加载真实归档或复现历史决策。</div>
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
  const [completedCount, setCompletedCount] = useState(stages.length);
  const [isRunning, setIsRunning] = useState(false);
  const [lastRunId, setLastRunId] = useState("DEMO-20260924-001");
  const [toast, setToast] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const stock = findStock(symbol);

  useEffect(() => {
    if (!isRunning) return;
    if (completedCount >= stages.length) return;
    const timeout = window.setTimeout(() => setCompletedCount(value => value + 1), 760);
    return () => window.clearTimeout(timeout);
  }, [isRunning, completedCount]);

  useEffect(() => {
    if (!isRunning || completedCount < stages.length) return;
    const timeout = window.setTimeout(() => {
      setIsRunning(false);
      setToast("演示研究完成：未调用 Go Agent，也未访问真实市场数据。");
    }, 200);
    return () => window.clearTimeout(timeout);
  }, [isRunning, completedCount]);

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
    setIsRunning(false);
    setCompletedCount(stages.length);
    setSelectedEvidence(findStock(next).evidence[0].id);
    setSelectedStage("debate");
    setView("overview");
    setToast(`已切换至 ${findStock(next).name}（演示样本）`);
  };

  const submitSearch = () => {
    const q = query.trim().toUpperCase();
    const found = demoStocks.find(item => item.symbol === q || item.symbol.startsWith(q) || item.name.includes(query.trim()));
    if (!q) { searchRef.current?.focus(); return; }
    if (!found) { setToast("当前 Mock 仅支持 600519.SH、000001.SZ 和 300750.SZ。未访问证券服务。"); return; }
    chooseStock(found.symbol);
  };

  const startDemo = () => {
    if (isRunning) return;
    setView("research");
    setLastRunId(`DEMO-${symbol}-${Date.now().toString().slice(-6)}`);
    setCompletedCount(0);
    setSelectedStage("market");
    setIsRunning(true);
    setToast("已启动本地研究流程演示（无真实模型或 API 调用）。");
  };

  const downloadReport = () => {
    const file = {
      demo: true, warning: "SYNTHETIC UI FIXTURE — NOT FINANCIAL ADVICE — NO LIVE MARKET DATA",
      run_id: lastRunId, symbol: stock.symbol, research_as_of: "2026-09-24",
      source: "frontend-local-mock", conclusion: stock.researchSummary,
      bull: stock.bull, bear: stock.bear, risk: stock.risk,
      portfolio_context: { state: "NOT_PROVIDED" }, evidence: stock.evidence,
    };
    const url = URL.createObjectURL(new Blob([JSON.stringify(file, null, 2)], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url; link.download = `aegis-DEMO-${stock.symbol}.json`;
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
      <nav aria-label="研究导航"><div className="nav-label">WORKSPACE</div>{navItems.map(item => <button key={item.id} type="button" className={cn("nav-item", view === item.id && "active")} onClick={() => setView(item.id)} aria-current={view === item.id ? "page" : undefined}><item.icon size={17} strokeWidth={1.9} /><span>{item.label}</span>{item.id === "history" && <span className="nav-count">4</span>}</button>)}</nav>
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
          <div className="heading-right"><ModeBadge /><button type="button" className="btn" onClick={downloadReport}><Download size={15} />导出演示 JSON</button></div>
        </div>

        <div className="demo-banner"><TriangleAlert size={16} /><span><strong>演示环境：</strong> 股票价格、走势、观点、Agent 进度与历史任务均为虚构样本。未接入真实行情、LLM、Go API 或账户。</span><CircleDot size={12} /></div>

        {view === "history" ? <HistoryView onOpen={chooseStock} /> : view === "settings" ? <SettingsView /> : <div className="main-grid">
          <div className="left-stack">
            <StockHero stock={stock} isRunning={isRunning} onRun={startDemo} />
            <Metrics stock={stock} />
            <StockChart stock={stock} timeframe={timeframe} onChange={setTimeframe} />
            <InsightPanel stock={stock} onNavigate={() => setView("research")} />
            <DebatePanel stock={stock} />
            {view === "research" && <section className="surface detail-intro"><div className="eyebrow">Research artifact / Demo</div><h2>完整研究结论</h2><p>Research Manager：{stock.researchSummary}</p><p style={{ marginTop: 13 }}>Risk Analyst：{stock.risk}</p><p style={{ marginTop: 13, color: "#81d5bb" }}>组合状态：NOT_PROVIDED · 当前仓位未知 · 不生成目标仓位。</p><div style={{ marginTop: 17, display: "flex", gap: 9, flexWrap: "wrap" }}><SmallPill tone="blue">TradeIntent · MONITOR（演示）</SmallPill><SmallPill>Portfolio · REVIEW_REQUIRED（演示）</SmallPill></div></section>}
          </div>
          <div className="right-stack">
            <AgentPanel completed={completedCount} running={isRunning} selected={selectedStage} onSelect={setSelectedStage} />
            <EvidencePanel stock={stock} selected={selectedEvidence} onSelect={setSelectedEvidence} />
            <HistoryPanel onViewAll={() => setView("history")} onOpen={chooseStock} />
          </div>
        </div>}
        <footer style={{ marginTop: 24, fontSize: 11, color: "#698099", textAlign: "right" }}>AEGIS RESEARCH WORKSPACE · MOCK PREVIEW · 研究辅助，不含交易执行</footer>
      </div>
    </main>
    {toast && <div role="status" className="toast">{toast}</div>}
  </div>;
}
