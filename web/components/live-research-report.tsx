"use client";

import type { LiveArtifact, LiveRun } from "@/lib/live-contract";
import { readLiveReport, type ReportCard } from "@/lib/live-report";
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
  return <div className="live-report-stack live-structured-report">
    <section className="surface live-report-overview" aria-label="研究报告概览">
      <div className="eyebrow">Verified Run · read-only research artifact</div>
      <div className="live-report-overview-top">
        <div><h2>结构化研究报告</h2><p>标的 {run.symbol} · 研究截止 {new Date(run.requested_as_of).toLocaleString("zh-CN", { hour12: false })}</p></div>
        <span className="badge blue">{report.mode}</span>
      </div>
      {report.headline ? <p className="live-report-headline">{report.headline}</p> : <p className="live-report-unavailable">该模式的后端结果没有可提取的研究摘要，请查看原始工件。</p>}
      {report.overview.length > 0 && <dl className="live-report-overview-facts">
        {report.overview.map((item, i) => <div key={item.label + i}><dt>{item.label}</dt><dd>{item.value}</dd></div>)}
      </dl>}
      <p className="live-report-notice">以下字段逐项取自同一 Run 的 Go Typed Artifact。研究日期不是实时报价时间；缺失字段不会被补成零。所有交易意向均仅供研究，不包含任何下单功能。</p>
    </section>
    {report.cards.length > 0 ? report.cards.map(card => <div key={card.id} className="live-report-stack">{card.id === "technical" && <LiveTechnicalVisual artifact={artifact} run={run}/>}<ReportCardView card={card}/></div>) : <section className="surface live-report-unavailable"><h3>无可识别的结构化段落</h3><p>原始工件仍可审计；请核对后端版本与 JSON 契约。</p></section>}
    <LiveEvidenceNavigator key={run.run_id} artifact={artifact}/>
    <details className="live-report-source-details"><summary>高级：查看后端原始 JSON 工件</summary>
      <RawArtifact value={artifact.research_artifact} label="ResearchArtifact"/>
      <RawArtifact value={artifact.portfolio_review} label="PortfolioReview"/>
      <RawArtifact value={artifact.final_risk_gate} label="FinalRiskGate"/>
      <RawArtifact value={artifact} label="完整 Typed Artifact"/>
    </details>
  </div>;
}
