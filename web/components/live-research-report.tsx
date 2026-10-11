"use client";

import { useEffect, useRef } from "react";
import { ArrowDown, ArrowUpRight, BookOpenText, ChevronDown, FileText, ShieldAlert } from "lucide-react";
import type { LiveArtifact, LiveRun } from "@/lib/live-contract";
import { readLiveReport, type ReportCard } from "@/lib/live-report";
import { buildReadingDigest, chapterCaption, groups, type ReadingPoint } from "@/lib/report-reader";
import LiveEvidenceNavigator from "@/components/live-evidence-navigator";
import LiveTechnicalVisual from "@/components/live-technical-visual";

function RawArtifact({ value, label }: { value: unknown; label: string }) {
  if (value === null || value === undefined) return null;
  const json = JSON.stringify(value, null, 2);
  return <details className="live-raw-panel"><summary>{label} · 后端原始工件</summary>
    <pre>{json.length > 28000 ? json.slice(0, 28000) + "\n…预览已截断，完整数据请使用 JSON 导出。" : json}</pre>
  </details>;
}

function ReportChapter({ card, artifact, run }: { card: ReportCard; artifact: LiveArtifact; run: LiveRun }) {
  return <details className="surface reader-chapter live-structured-card" id={"report-" + card.id}>
    <summary className="reader-chapter-header">
      <span className="reader-chapter-heading"><strong>{card.title}</strong><small>{chapterCaption(card)}</small></span>
      <span className="reader-chapter-toggle">展开原始内容 <ChevronDown size={16}/></span>
    </summary>
    <div className="reader-chapter-content">
      <p className="reader-chapter-description">{card.description}</p>
      {card.id === "technical" && <LiveTechnicalVisual artifact={artifact} run={run}/>}
      {card.fields.length > 0 && <dl className="live-report-facts">
        {card.fields.map((field, i) => <div key={field.label + i}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}
      </dl>}
      {card.entries.length > 0 && <div className="live-structured-entries">
        {card.entries.map((entry, i) => <article className="live-structured-entry" key={entry.label + i}>
          <h4>{entry.label}</h4><p>{entry.body}</p>
          {entry.details.length > 0 && <div className="live-reference-tags" aria-label="Go 原始引用与元数据">
            {entry.details.map((detail, index) => <span key={index}>{detail}</span>)}
          </div>}
        </article>)}
      </div>}
    </div>
  </details>;
}
function KeyPoints({title, items, totalMore, emptyText, kind}:{
  title: string; items: ReadingPoint[]; totalMore: number; emptyText: string; kind: "support" | "conflict" | "risk";
}) {
  return <section className={"reader-key-group reader-key-"+kind}>
    <div className="reader-key-heading"><h3>{title}</h3><span>{items.length + totalMore} 条</span></div>
    {items.length === 0 ? <p className="reader-key-empty">{emptyText}</p> :
      <ol>{items.map((item, i) => <li key={item.sectionId + "-" + item.label + "-" + i}>
        <span className="reader-key-number">{String(i+1).padStart(2,"0")}</span>
        <div><p>{item.text}</p>
          <a href={"#report-" + item.sectionId}>{item.label} · 查看原文 <ArrowUpRight size={13}/></a>
        </div>
      </li>)}</ol>}
    {totalMore > 0 && <p className="reader-key-overflow">另有 {totalMore} 条记录，可在下方对应章节展开查看。</p>}
  </section>;
}

/**
 * All displayed research claims come from the same Run's typed artifact.
 * This is a readable *projection*, not a regenerated LLM recommendation.
 */
export default function LiveResearchReport({ artifact, run }: { artifact: LiveArtifact; run: LiveRun }) {
  const report = readLiveReport(artifact);
  const digest = buildReadingDigest(report);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Agent trace links point to #report-market, #report-event, etc. A user
    // following a deep link must never land on a silently closed section.
    const reveal = () => {
      const hash = window.location.hash.slice(1);
      if (!/^report-[a-z]+$/.test(hash)) return;
      const target = document.getElementById(hash);
      if (target instanceof HTMLDetailsElement && rootRef.current?.contains(target)) {
        target.open = true;
        target.scrollIntoView({ block: "start", behavior: "smooth" });
      }
    };
    reveal();
    window.addEventListener("hashchange", reveal);
    return () => window.removeEventListener("hashchange", reveal);
  }, [run.run_id]);

  const setChapters = (open: boolean) =>
    rootRef.current?.querySelectorAll<HTMLDetailsElement>(".reader-chapter").forEach(node => { node.open = open; });

  const formatDate = new Date(run.requested_as_of).toLocaleString("zh-CN", { hour12: false });
  return <div className="live-report-stack live-structured-report reader-report" ref={rootRef}>
    <section className="surface live-report-overview reader-lead" aria-label="研究报告概览">
      <div className="reader-topline"><span><BookOpenText size={15}/> 股票研究 / 阅读版</span><span>来源：Go Typed Artifact · 只读</span></div>
      <div className="live-report-overview-top">
        <div><h2>结构化研究报告</h2>
          <p className="reader-runmeta"><strong>{run.symbol}</strong><span>研究截止：{formatDate}</span><span>{report.mode}</span></p></div>
        <span className="reader-document-label">RESEARCH BRIEF</span>
      </div>
      <div className="reader-conclusion" aria-label="本次研究的原始结论">
        <div className="reader-section-kicker">01 / 核心结论</div>
        {digest.headline
          ? <p className="reader-conclusion-text">{digest.headline}</p>
          : <p className="reader-missing">当前 Run 未提供可读取的研究摘要。不能把空白解释为“建议持有”或“无风险”；请核对下方原始章节。</p>}
      </div>
      <div className="reader-status-grid" aria-label="原始结论、门禁与账户状态">
        {digest.status.length > 0 ? digest.status.map(item =>
          <div className="reader-status-item" key={item.label}>
            <span>{item.label}</span><strong>{item.display}</strong>
          </div>) : <div className="reader-status-item"><span>结构化状态</span><strong>后端未提供</strong></div>}
      </div>
      <div className="reader-provenance-note"><ShieldAlert size={17}/>
        <p>这是同一次真实研究任务保存的 Agent 观点和数据快照，不是新的 AI 生成报告或实时行情。
          <strong> 模型判断 ≠ 已核实事实；研究意向 ≠ 下单指令。</strong> 缺失或未知的账户、证据、数据不会推断为零或安全。</p>
      </div>
    </section>

    <section className="surface reader-highlights" aria-label="研究要点速读">
      <div className="reader-section-heading"><div><div className="reader-section-kicker">02 / 研究要点</div><h2>先看支持、分歧与风险</h2>
        <p>下面是从原文中直接抽取的观点，不自动改写、不判定真假。点击来源可展开完整内容。</p></div></div>
      <div className="reader-key-grid">
        <KeyPoints kind="support" title="支持观点 / 机会" items={digest.supporting} totalMore={digest.omittedSupporting}
          emptyText="本次工件没有明确提供可提取的支持观点；不代表没有机会。"/>
        <KeyPoints kind="conflict" title="分歧与待解决" items={digest.conflicting} totalMore={digest.omittedConflicting}
          emptyText="未记录明确分歧或未解决的论点 ID；不能据此断言研究各方已达成共识。"/>
        <KeyPoints kind="risk" title="风险与限制" items={digest.risks} totalMore={digest.omittedRisks}
          emptyText="未提供可直接提取的风险条目；这不等于低风险。"/>
      </div>
    </section>

    <section className="reader-chapters" aria-label="研究报告详细章节">
      <div className="reader-chapters-header">
        <div><div className="reader-section-kicker">03 / 原始分析</div><h2>按主题阅读完整分析</h2>
          <p>原始 Go 工件全部保留，不必一次看完所有字段。</p></div>
        <div className="reader-chapter-actions"><button type="button" onClick={()=>setChapters(true)}>展开全部 <ArrowDown size={14}/></button>
          <button type="button" onClick={()=>setChapters(false)}>收起全部</button></div>
      </div>
      {groups.map(group => {
        const cards = group.cardIds.map(id=>report.cards.find(c=>c.id===id)).filter((c):c is ReportCard=>c!==undefined);
        if (!cards.length) return null;
        return <section className="reader-chapter-group" key={group.id} aria-label={group.title}>
          <div className="reader-chapter-group-heading"><h3>{group.title}</h3><p>{group.intro}</p></div>
          {cards.map(card => <ReportChapter key={card.id} card={card} artifact={artifact} run={run}/>)}
        </section>;
      })}
      {report.cards.length === 0 && <section className="surface reader-no-chapters"><h3>没有可读取的结构化章节</h3><p>请展开下方原始 JSON 核对工件结构。</p></section>}
    </section>
    <section className="reader-audit-zone" aria-label="证据审计">
      <div className="reader-section-kicker">04 / 证据与原文审计</div>
      <LiveEvidenceNavigator key={run.run_id} artifact={artifact}/>
      <details className="live-report-source-details"><summary><FileText size={16}/> 技术审计：查看后端完整 JSON 工件</summary>
        <p className="reader-audit-note">原始 JSON 仅用于核查字段、ID 和来源关系。预览可能截断，完整数据可在页面上方下载。</p>
        <RawArtifact value={artifact.research_artifact} label="ResearchArtifact"/>
        <RawArtifact value={artifact.portfolio_review} label="PortfolioReview"/>
        <RawArtifact value={artifact.final_risk_gate} label="FinalRiskGate"/>
        <RawArtifact value={artifact} label="完整 Typed Artifact"/>
      </details>
    </section>
  </div>;
}
