"use client";

import type { LiveArtifact, LiveRun } from "@/lib/live-contract";
import { useEffect, useRef } from "react";
import { readLiveReport, type ReportCard } from "@/lib/live-report";
import { readResearchBrief, readableResearchCode, summarizeBriefBlocks } from "@/lib/research-brief";
import { ArrowLeftRight, BarChart3, BookOpen, FileSearch, MessageCircle, ShieldAlert, ListChecks } from "lucide-react";
import LiveEvidenceNavigator from "@/components/live-evidence-navigator";
import LiveTechnicalVisual from "@/components/live-technical-visual";
import LiveHistoricalChart from "@/components/live-historical-chart";

function RawArtifact({ value, label }: { value: unknown; label: string }) {
  if (value === null || value === undefined) return null;
  const json = JSON.stringify(value, null, 2);
  return <details className="live-raw-panel"><summary>{label} · 后端原始工件</summary><pre>{json.length > 28000 ? json.slice(0, 28000) + "\n…预览已截断，完整数据请使用 JSON 导出。" : json}</pre></details>;
}

function ReportCardView({ card }: { card: ReportCard }) {
  return <section className="surface live-structured-card" id={"report-" + card.id}>
    <header className="live-structured-card-head">
      <h3>{card.title}</h3>
      <p>{card.description}</p>
    </header>
    {card.fields.length > 0 && <dl className="live-report-facts">
      {card.fields.map((field, i) => <div key={field.label + i}>
        <dt>{field.label}</dt><dd>{readableResearchCode(field.value)}</dd>
      </div>)}
    </dl>}
    {card.entries.length > 0 && <div className="live-structured-entries">
      {card.entries.map((entry, i) => <article key={entry.label + i} className="live-structured-entry">
        <h4>{entry.label}</h4><p>{entry.body}</p>
        {entry.details.length > 0 && <div className="live-reference-tags" aria-label="后端已声明的引用及元数据">
          {entry.details.map((detail, index) => <span key={index}>{detail}</span>)}
        </div>}
      </article>)}
    </div>}
  </section>;
}

/** The report UI never reads a different run's artifact. Caller keys artifact by Run ID. */
export default function LiveResearchReport({ artifact, run, onCompare }: { artifact: LiveArtifact; run: LiveRun; onCompare?: () => void }) {
  const report = readLiveReport(artifact);
  const brief = readResearchBrief(report);
  const evidenceSummary = summarizeBriefBlocks(brief.evidence);
  const riskSummary = summarizeBriefBlocks(brief.risks);
  const reportRoot = useRef<HTMLDivElement>(null);
  const evidenceDisclosure = useRef<HTMLDetailsElement>(null);
  const followUpEntries = report.cards.flatMap(card => card.entries
    .filter(entry => /观察|关注|触发|条件|后续|下一步|跟踪|复核/.test(entry.label))
    .map(entry => ({ label: entry.label, body: entry.body, source: card.title })))
    .filter(entry => entry.body.trim()).slice(0, 3);
  const setAllChapters = (expanded: boolean) => {
    // Scope to this report: never change the state of another Run or raw JSON panel.
    reportRoot.current?.querySelectorAll<HTMLDetailsElement>("details.live-brief-chapter").forEach(chapter => {
      chapter.open = expanded;
    });
  };
  // Direct links, browser history and stage trace must all reveal collapsed chapters.
  // Query only this report so switching runs never expands another report.
  useEffect(() => {
    const openTarget = (hash: string) => {
      const match = /^#report-([a-z-]+)$/.exec(hash);
      if (!match) return;
      const chapter = reportRoot.current?.querySelector<HTMLDetailsElement>(
        "#chapter-" + match[1],
      );
      if (chapter) chapter.open = true;
    };
    const onAnchorClick = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return;
      const anchor = event.target.closest('a[href^="#report-"]');
      if (anchor) openTarget(anchor.getAttribute("href") ?? "");
    };
    const onHashChange = () => openTarget(window.location.hash);
    onHashChange(); // Handles initial loads and run switches with an existing hash.
    document.addEventListener("click", onAnchorClick, true);
    window.addEventListener("hashchange", onHashChange);
    return () => {
      document.removeEventListener("click", onAnchorClick, true);
      window.removeEventListener("hashchange", onHashChange);
    };
  }, [run.run_id]);

  return <div ref={reportRoot} className="live-report-stack live-structured-report">
    <section id="report-current-conclusion" className="surface live-report-overview" aria-label="研究报告概览">
      <div className="eyebrow">研究判断</div>
      <div className="live-report-overview-top">
        <div><h2>{run.symbol} 的研究判断</h2><p>研究截止 {new Date(run.requested_as_of).toLocaleString("zh-CN", { hour12: false })}</p></div>
        <span className="badge blue">{report.mode === "PORTFOLIO_REVIEW" ? "组合审查" : report.mode === "DECISION" ? "单标的研究" : "研究类型未知"}</span>
      </div>
      <div className="live-brief-lead">
        <span className="live-brief-eyebrow">当前判断</span>
        <h3>{readableResearchCode(brief.marketView ?? brief.conclusion)}</h3>
        <p>{brief.summary ?? "该次研究没有提供简要结论。请查看后续详情与原始工件。"}</p>
        <div className="live-decision-grid" aria-label="研究决策摘要">
          <div><span>研究结论</span><strong>{readableResearchCode(brief.conclusion)}</strong></div>
          <div><span>研究动作</span><strong>{readableResearchCode(brief.action)}</strong></div>
          {brief.posture && <div><span>风险门禁</span><strong>{readableResearchCode(brief.posture)}</strong></div>}
          {brief.confidence && <div><span>判断置信度</span><strong>{readableResearchCode(brief.confidence)}</strong></div>}
        </div>
      </div>
      {report.overview.filter(item => !["标的", "研究结论", "市场观点", "研究动作（非委托）", "研究意向（非指令）", "最终风险门禁", "置信度"].includes(item.label)).length > 0 && <dl className="live-report-overview-facts">
        {report.overview.filter(item => !["标的", "研究结论", "市场观点", "研究动作（非委托）", "研究意向（非指令）", "最终风险门禁", "置信度"].includes(item.label)).map((item, i) => <div key={item.label + i}><dt>{item.label}</dt><dd>{readableResearchCode(item.value)}</dd></div>)}
      </dl>}
      <div className="live-brief-grid">
        <section className="live-brief-section" aria-label="研究关键依据"><h3><ListChecks size={17}/>关键依据</h3>
          {evidenceSummary.blocks.length ? <>{evidenceSummary.blocks.map(block=><div key={block.title} className="live-brief-block"><h4>{block.title}</h4><ul>{block.lines.map((line,i)=><li key={i}>{line}</li>)}</ul><small>来源：{block.source}</small></div>)}{evidenceSummary.omittedLines > 0 && <p className="live-brief-muted">摘要展示 3 条代表性依据；另有 {evidenceSummary.omittedLines} 条原始依据保留在完整研究章节中。</p>}</> :
           <p className="live-brief-muted">摘要层没有明确的支持观点；不代表存在看多依据。</p>}
        </section>
        <section className="live-brief-section" aria-label="研究风险与不确定性"><h3><ShieldAlert size={17}/>风险与不确定性</h3>
          {riskSummary.blocks.map(block=><div key={block.title} className="live-brief-block"><h4>{block.title}</h4><ul>{block.lines.map((line,i)=><li key={i}>{line}</li>)}</ul><small>来源：{block.source}</small></div>)}
          {riskSummary.omittedLines > 0 && <p className="live-brief-muted">摘要展示 3 条代表性风险；另有 {riskSummary.omittedLines} 条风险细节保留在完整研究章节中。</p>}
          {!brief.risks.length && <p className="live-brief-muted">摘要层未提取到风险记录，不意味着风险为零。</p>}
        </section>
      </div>
      {brief.unknowns.length > 0 && <section className="live-brief-unknowns" aria-label="尚未确认"><h3><ShieldAlert size={16}/>尚未确认</h3><ul>{brief.unknowns.map((line, i)=><li key={i}>{line}</li>)}</ul></section>}
      {(brief.market.length > 0 || brief.technical.length > 0) && <section className="live-brief-metrics" aria-label="研究时点关键数据">
        <h3><BarChart3 size={16}/>研究时点关键数据（非实时）</h3>
        <dl>{[...brief.market, ...brief.technical].map((f,i)=><div key={f.label+i}><dt>{f.label}</dt><dd>{f.value}</dd></div>)}</dl>
      </section>}
      <p className="live-report-notice">以上结论、数据与引用来自本次研究记录。研究时点不是实时报价时间；缺失字段不会被补成零。研究意向仅供分析，不包含下单功能。</p>
    </section>
    <nav className="live-report-actions" aria-label="研究报告操作">
      <a className="is-active" href="#report-current-conclusion">当前结论</a>
      {onCompare && <button type="button" onClick={onCompare}><ArrowLeftRight size={14}/>判断变化</button>}
      <button type="button" onClick={() => { if (evidenceDisclosure.current) { evidenceDisclosure.current.open = true; evidenceDisclosure.current.scrollIntoView({ behavior: "smooth", block: "start" }); } }}><FileSearch size={14}/>完整依据</button>
      <a href="#research-follow-up"><MessageCircle size={14}/>继续追问</a>
    </nav>
    <section className="live-follow-up-card" aria-label="后续关注">
      <h3>后续关注</h3>
      {followUpEntries.length > 0
        ? <ul>{followUpEntries.map((entry, index) => <li key={entry.label + index}>{entry.body}<small>来源：{entry.source} · {entry.label}</small></li>)}</ul>
        : <p>本次工件未声明结构化的后续观察条件。可在“继续追问”中针对当前 Run 明确需要补充的指标、事件或复核条件；系统不会自动补全缺失条件。</p>}
      <a href="#research-follow-up">继续追问本次研究 →</a>
    </section>
    {report.cards.length > 0 ? <section className="live-brief-appendix" aria-label="完整研究章节">
      <div className="live-brief-appendix-heading"><BookOpen size={18}/><div><h2>完整研究章节</h2><p>按需查看市场、技术、事件与论点分析。摘要方便快速阅读，不替代完整研究记录。</p></div></div>
      <nav className="live-chapter-index" aria-label="研究报告章节目录">
        <strong>快速跳转至章节</strong>
        <div className="live-chapter-index-links">
          {report.cards.map(card => <a key={card.id} href={"#report-" + card.id}>{card.title}</a>)}
        </div>
      </nav>
      <div className="live-chapter-controls" role="group" aria-label="研究章节批量操作">
        <button type="button" onClick={() => setAllChapters(true)}>展开所有章节</button>
        <button type="button" onClick={() => setAllChapters(false)}>收起所有章节</button>
      </div>
      {report.cards.map(card => <details className="live-brief-chapter" key={card.id} id={"chapter-" + card.id}>
        <summary>{card.title}<span>查看完整章节</span></summary>
        <div className="live-report-stack">{card.id === "technical" && <><LiveHistoricalChart run={run}/><LiveTechnicalVisual artifact={artifact} run={run}/></>}<ReportCardView card={card}/></div>
      </details>)}
    </section> : <section className="surface live-report-unavailable"><h3>无可识别的结构化段落</h3><p>原始工件仍可审计；请核对后端版本与 JSON 契约。</p></section>}
    <details id="report-evidence" ref={evidenceDisclosure} className="live-evidence-disclosure">
      <summary>完整研究依据与引用链 <span>结构化论点、反驳、事件引用与来源关联</span></summary>
      <div className="live-evidence-disclosure-body"><LiveEvidenceNavigator key={run.run_id} artifact={artifact}/></div>
    </details>
    <details className="live-report-source-details"><summary>高级：查看后端原始 JSON 工件</summary>
      <RawArtifact value={artifact.research_artifact} label="ResearchArtifact"/>
      <RawArtifact value={artifact.portfolio_review} label="PortfolioReview"/>
      <RawArtifact value={artifact.final_risk_gate} label="FinalRiskGate"/>
      <RawArtifact value={artifact} label="完整 Typed Artifact"/>
    </details>
  </div>;
}
