"use client";

import type { LiveArtifact, LiveRun } from "@/lib/live-contract";
import { useEffect } from "react";
import { readLiveReport, type ReportCard } from "@/lib/live-report";
import { readResearchBrief, readableResearchCode } from "@/lib/research-brief";
import { BookOpen, ShieldAlert, ListChecks, BarChart3 } from "lucide-react";
import LiveEvidenceNavigator from "@/components/live-evidence-navigator";
import LiveTechnicalVisual from "@/components/live-technical-visual";

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
        <dt>{field.label}</dt><dd>{field.value}</dd>
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
export default function LiveResearchReport({ artifact, run }: { artifact: LiveArtifact; run: LiveRun }) {
  const report = readLiveReport(artifact);
  const brief = readResearchBrief(report);
  // Stage trace and evidence links retain their existing #report-... targets.
  // Expand the actual chapter before the browser navigates to the anchor.
  useEffect(() => {
    const openLinkedChapter = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return;
      const anchor = event.target.closest('a[href^="#report-"]');
      const target = anchor?.getAttribute("href");
      if (!target || !/^#report-[a-z-]+$/.test(target)) return;
      const section = document.getElementById("chapter-" + target.slice("#report-".length));
      if (section instanceof HTMLDetailsElement) section.open = true;
    };
    document.addEventListener("click", openLinkedChapter, true);
    return () => document.removeEventListener("click", openLinkedChapter, true);
  }, [run.run_id]);

  return <div className="live-report-stack live-structured-report">
    <section className="surface live-report-overview" aria-label="研究报告概览">
      <div className="eyebrow">已完成研究 · 只读原始工件</div>
      <div className="live-report-overview-top">
        <div><h2>结构化研究报告</h2><p>标的 {run.symbol} · 研究截止 {new Date(run.requested_as_of).toLocaleString("zh-CN", { hour12: false })}</p></div>
        <span className="badge blue">{report.mode === "PORTFOLIO_REVIEW" ? "组合研究" : report.mode === "DECISION" ? "研究决策" : "研究模式"} · {report.mode}</span>
      </div>
      <div className="live-brief-lead">
        <span className="live-brief-eyebrow">研究结论 · 源自 Go 工件</span>
        <h3>{readableResearchCode(brief.conclusion)}</h3>
        <p>{brief.summary ?? "该次研究没有提供简要结论。请查看后续详情与原始工件。"}</p>
        {brief.posture && <div className="live-brief-gate"><ShieldAlert size={15}/>风险门禁：<strong>{readableResearchCode(brief.posture)}</strong></div>}
      </div>
      {report.overview.length > 0 && <dl className="live-report-overview-facts">
        {report.overview.map((item, i) => <div key={item.label + i}><dt>{item.label}</dt><dd>{readableResearchCode(item.value)}</dd></div>)}
      </dl>}
      <div className="live-brief-grid">
        <section className="live-brief-section" aria-label="研究关键依据"><h3><ListChecks size={17}/>关键依据</h3>
          {brief.evidence.length ? brief.evidence.map(block=><div key={block.title} className="live-brief-block"><h4>{block.title}</h4><ul>{block.lines.map((line,i)=><li key={i}>{line}</li>)}</ul><small>来源：{block.source}</small></div>) :
          <p className="live-brief-muted">摘要层没有明确的支持观点；不代表存在看多依据。</p>}
        </section>
        <section className="live-brief-section" aria-label="研究风险与不确定性"><h3><ShieldAlert size={17}/>风险与不确定性</h3>
          {brief.risks.map(block=><div key={block.title} className="live-brief-block"><h4>{block.title}</h4><ul>{block.lines.map((line,i)=><li key={i}>{line}</li>)}</ul><small>来源：{block.source}</small></div>)}
          {brief.unknowns.map((line,i)=><p className="live-brief-warning" key={i}>{line}</p>)}
          {!brief.risks.length && !brief.unknowns.length && <p className="live-brief-muted">摘要层未提取到风险记录，不意味着风险为零。</p>}
        </section>
      </div>
      {(brief.market.length > 0 || brief.technical.length > 0) && <section className="live-brief-metrics" aria-label="研究时点关键数据">
        <h3><BarChart3 size={16}/>研究时点关键数据（非实时）</h3>
        <dl>{[...brief.market, ...brief.technical].map((f,i)=><div key={f.label+i}><dt>{f.label}</dt><dd>{readableResearchCode(f.value)}</dd></div>)}</dl>
      </section>}
      <p className="live-report-notice">以下字段逐项取自同一 Run 的 Go Typed Artifact。研究日期不是实时报价时间；缺失字段不会被补成零。所有交易意向均仅供研究，不包含任何下单功能。</p>
    </section>
    {report.cards.length > 0 ? <section className="live-brief-appendix" aria-label="完整研究章节">
      <div className="live-brief-appendix-heading"><BookOpen size={18}/><div><h2>完整研究章节</h2><p>按需展开数据、分析过程、论点与引用。以上简报仅是原始报告的阅读索引。</p></div></div>
      {report.cards.map(card => <details className="live-brief-chapter" key={card.id} id={"chapter-" + card.id}>
        <summary>{card.title}<span>查看完整章节</span></summary>
        <div className="live-report-stack">{card.id === "technical" && <LiveTechnicalVisual artifact={artifact} run={run}/>}<ReportCardView card={card}/></div>
      </details>)}
    </section> : <section className="surface live-report-unavailable"><h3>无可识别的结构化段落</h3><p>原始工件仍可审计；请核对后端版本与 JSON 契约。</p></section>}
    <LiveEvidenceNavigator key={run.run_id} artifact={artifact}/>
    <details className="live-report-source-details"><summary>高级：查看后端原始 JSON 工件</summary>
      <RawArtifact value={artifact.research_artifact} label="ResearchArtifact"/>
      <RawArtifact value={artifact.portfolio_review} label="PortfolioReview"/>
      <RawArtifact value={artifact.final_risk_gate} label="FinalRiskGate"/>
      <RawArtifact value={artifact} label="完整 Typed Artifact"/>
    </details>
  </div>;
}
