"use client";

import { useMemo, useState } from "react";
import {
  Activity, ArrowRight, ArrowUpRight, BarChart3, CircleAlert,
  Clock3, FileCheck2, History, PlayCircle, RefreshCcw, Search, XCircle,
} from "lucide-react";
import type { LiveRun } from "@/lib/live-contract";
import { filterOverviewRuns, runStateLabel, summarizeRunOverview, type OverviewFilter } from "@/lib/run-overview";

const filters: Array<{ key: OverviewFilter; label: string }> = [
  { key: "all", label: "全部" }, { key: "active", label: "执行中" },
  { key: "completed", label: "已完成" }, { key: "failed", label: "失败" },
  { key: "cancelled", label: "已取消" },
];
const ts = (value: string) => new Date(value).toLocaleString("zh-CN", { hour12: false });

export default function LiveRunOverview({
  runs, syncedAt, syncError, onOpen, onNew, onCompare, onRefresh,
}: {
  runs: LiveRun[];
  syncedAt: string | null;
  syncError: string | null;
  onOpen: (runId: string) => void;
  onNew: () => void;
  onCompare: () => void;
  onRefresh: () => void;
}) {
  const [filter, setFilter] = useState<OverviewFilter>("all");
  const [query, setQuery] = useState("");
  const overview = useMemo(() => summarizeRunOverview(runs, new Date()), [runs]);
  const visible = useMemo(() => filterOverviewRuns(overview.runs, filter, query), [overview.runs, filter, query]);
  const peak = Math.max(1, ...overview.daily.map(day => day.count));
  return <div className="run-overview" aria-label="研究任务概览">
    <section className="run-overview-hero">
      <div>
        <div className="eyebrow">Operations center / Go ResearchRun</div>
        <h2>研究任务总览</h2>
        <p>研究进展、失败排查和历史报告，从这里进入。全部信息只读取服务端已保存的任务元数据。</p>
      </div>
      <div className="run-overview-actions">
        <button className="btn" type="button" onClick={onCompare}><History size={15}/>对比研究</button>
        <button className="btn primary" type="button" onClick={onNew}><PlayCircle size={15}/>发起新研究</button>
      </div>
    </section>
    <div className="run-overview-sync" role="status">
      <span className={syncError ? "run-overview-sync-error" : ""}>
        <Activity size={14}/>
        {!syncedAt ? "尚未获取服务端任务列表" : syncError ?
          "最近一次同步失败 · 数据可能已过期" : "最近一次浏览器同步：" + ts(syncedAt)}
      </span>
      <button type="button" className="run-overview-refresh" onClick={onRefresh}><RefreshCcw size={13}/>立即刷新</button>
    </div>
    {syncError && <p className="run-overview-warning" role="alert">无法刷新任务列表：{syncError}。以下可能是上一次成功返回的记录，不代表当前实时任务状态。</p>}
    {!syncedAt ? <section className="surface run-overview-zero" role="status">正在获取 Go 研究任务列表；尚不显示统计，避免将未同步误报为 0 条任务。</section> : <>
      <div className="run-overview-metrics" aria-label="已返回任务样本状态分布">
        <div className="run-overview-kpi"><div className="run-overview-kpi-top"><span>返回样本</span><BarChart3 size={18}/></div><strong>{overview.total}</strong><small>后端当前任务列表</small></div>
        <div className="run-overview-kpi active"><div className="run-overview-kpi-top"><span>等待 / 执行中</span><Clock3 size={18}/></div><strong>{overview.active}</strong><small>需要关注进度</small></div>
        <div className="run-overview-kpi complete"><div className="run-overview-kpi-top"><span>已完成</span><FileCheck2 size={18}/></div><strong>{overview.completed}</strong><small>可查看原始报告</small></div>
        <div className="run-overview-kpi failed"><div className="run-overview-kpi-top"><span>失败</span><CircleAlert size={18}/></div><strong>{overview.failed}</strong><small>点击查看真实失败代码</small></div>
        <div className="run-overview-kpi"><div className="run-overview-kpi-top"><span>已取消</span><XCircle size={18}/></div><strong>{overview.cancelled}</strong><small>不会生成完成报告</small></div>
      </div>
      <p className="run-overview-sample-note">以上仅统计当前 Go 列表返回的 <strong>{overview.total}</strong> 条有效唯一 Run，
        {overview.capped ? "列表已达到接口上限 100 条，历史实际数量可能更多。" : "不是全量历史计数或交易绩效。"}
        涉及 <strong>{overview.distinctSymbols}</strong> 个不同标的。样本最多 100 条。
      </p>
      <div className="run-overview-two-col">
        <section className="surface run-overview-panel" aria-label="近七日任务活动">
          <div className="run-overview-panel-title"><h3>近七日任务创建量</h3><span>UTC · 样本记录</span></div>
          <div className="run-overview-week" role="img" aria-label={"最近七个 UTC 日、基于返回样本的新建任务量：" + overview.daily.map(d => d.utcDate + " " + d.count).join("；")}>
            {overview.daily.map(day => <div key={day.utcDate} className="run-overview-day">
              <strong>{day.count}</strong>
              <div className="run-overview-bar-track"><span style={{height:(day.count / peak * 100)+"%"}}/></div>
              <small>{day.utcDate.slice(5)}</small>
            </div>)}
          </div>
          <p>仅统计任务创建数量，不代表研究有效性、模型成本或市场收益。</p>
        </section>
        <section className="surface run-overview-panel run-overview-attention" aria-label="需要关注的任务">
          <div className="run-overview-panel-title"><h3>待关注</h3><span>最近失败任务</span></div>
          {overview.recentFailures.length === 0 ? <p className="run-overview-ok">当前返回样本中没有失败任务；这不保证历史没有失败。</p> :
          <div className="run-overview-failures">{overview.recentFailures.map(run =>
            <button type="button" key={run.run_id} onClick={()=>onOpen(run.run_id)} className="run-overview-failure">
              <div><strong>{run.symbol}</strong><small>{ts(run.created_at)}</small></div>
              <code>{run.failure_code || "未提供失败代码"}</code><ArrowUpRight size={14}/>
            </button>)}</div>}
          {overview.active > 0 && <p className="run-overview-active-hint"><Activity size={13}/>另有 {overview.active} 条等待或运行中的研究，点击下方对应任务追踪阶段。</p>}
        </section>
      </div>
      <section className="surface run-overview-list" aria-label="近期研究任务列表">
        <div className="run-overview-list-head">
          <div><h3>近期研究任务</h3><p>按服务端创建时间倒序 · 可直接进入完整 Run 详情</p></div>
          <label className="run-overview-search"><Search size={15}/><span className="sr-only">筛选股票或任务 ID</span>
            <input aria-label="筛选股票或任务 ID" value={query} onChange={e=>setQuery(e.target.value)} placeholder="搜索股票 / Run ID"/></label>
        </div>
        <div className="run-overview-filterbar" role="group" aria-label="研究任务状态筛选">
          {filters.map(item => <button key={item.key} type="button" aria-pressed={filter===item.key}
            onClick={()=>setFilter(item.key)} className={filter===item.key?"selected":""}>{item.label}</button>)}
        </div>
        {overview.total === 0 ? <div className="run-overview-zero">
          <h4>暂无已保存的研究任务</h4><p>首次发起研究后，这里会显示 Go 服务端返回的任务。不会填充模拟任务。</p>
          <button type="button" className="btn primary" onClick={onNew}>发起第一项研究 <ArrowRight size={14}/></button>
        </div> : visible.length === 0 ? <div className="run-overview-zero">
          <h4>没有符合筛选条件的任务</h4><p>可切换状态或清空搜索条件。</p>
          <button className="btn" type="button" onClick={()=>{setFilter("all");setQuery("");}}>清除筛选</button>
        </div> : <div className="run-overview-rows">
          {visible.map(run => <button key={run.run_id} type="button" className="run-overview-row" onClick={()=>onOpen(run.run_id)}>
            <div className="run-overview-identity"><strong>{run.symbol}</strong><code>{run.run_id.slice(0,12)}…</code></div>
            <div className="run-overview-meta"><span>{ts(run.created_at)}</span><small>{run.mode}</small></div>
            <span className={"run-overview-state state-"+run.state.toLowerCase()}>{runStateLabel(run.state)}</span>
            <span className="run-overview-action">{run.state==="COMPLETED"?"查看报告":run.state==="FAILED"?"排查失败":run.state==="CANCELLED"?"查看记录":"查看进度"} <ArrowUpRight size={13}/></span>
          </button>)}
        </div>}
        <div className="run-overview-list-footer">显示 <strong>{visible.length}</strong> / {overview.total} 条当前返回样本 · 点击任务查看服务端运行详情与原始记录</div>
      </section>
    </>}
  </div>;
}
