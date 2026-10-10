"use client";

import { useState } from "react";
import { ArrowRight, BookOpenText, GitBranch, Link2, ShieldAlert, TrendingDown, TrendingUp } from "lucide-react";
import type { StockFixture } from "@/lib/mock-research";
import { type DemoResearchTrace, type DemoClaim } from "@/lib/research-trace";
import { cn } from "@/lib/utils";

type Tab = "claims" | "rounds" | "handoff";
const sideLabel = { BULL: "Bull · 多头", BEAR: "Bear · 空头", SYNTHESIS: "Research Manager", RISK: "Risk · 风险" } as const;
const assessmentLabel = { SUPPORTS: "支持假设的样例", CONTESTS: "反方证据样例", UNVERIFIED: "未核实 / 条件不足" } as const;

function ClaimCard({ claim, stock, selected, onSelectClaim, onSelectEvidence }: {
  claim: DemoClaim; stock: StockFixture; selected: boolean;
  onSelectClaim: (id: string) => void; onSelectEvidence: (id: string) => void;
}) {
  return <article className={cn("claim-card", selected && "selected", `claim-${claim.side.toLowerCase()}`)}>
    <button type="button" className="claim-top" aria-pressed={selected} onClick={() => onSelectClaim(claim.id)}>
      <span className="claim-side">{claim.side === "BULL" ? <TrendingUp size={14}/> : claim.side === "BEAR" ? <TrendingDown size={14}/> : claim.side === "RISK" ? <ShieldAlert size={14}/> : <GitBranch size={14}/>} {sideLabel[claim.side]}</span>
      <span className="claim-status">{assessmentLabel[claim.assessment]}</span>
    </button>
    <h3>{claim.title}</h3>
    <p>{claim.description}</p>
    {claim.respondsTo && <p className="claim-reply">↳ 回应 {claim.respondsTo.split("-").slice(-1)[0]} 样例观点 · 并非真实 Agent Trace</p>}
    <div className="claim-links">
      <span><Link2 size={12}/> 样例引用</span>
      {claim.evidenceIds.filter(id => stock.evidence.some(e => e.id === id)).map(id =>
        <button key={id} className="reference-chip" type="button" onClick={() => onSelectEvidence(id)} aria-label={`查看证据 ${id}`}>
          {id} <ArrowRight size={11}/>
        </button>
      )}
    </div>
  </article>;
}

export default function ResearchTracePanel({ trace, stock, selectedClaimId, onSelectClaim, onSelectEvidence }: {
  trace: DemoResearchTrace; stock: StockFixture; selectedClaimId: string;
  onSelectClaim: (id: string) => void; onSelectEvidence: (id: string) => void;
}) {
  const [tab, setTab] = useState<Tab>("claims");
  const tabs: Array<{ id: Tab; title: string }> = [
    { id: "claims", title: "观点与引用" }, { id: "rounds", title: "辩论过程" }, { id: "handoff", title: "结论交接" },
  ];
  return <section className="surface trace-panel" id="research-trace" aria-label="演示研究观点与证据链">
    <div className="trace-heading">
      <div><div className="trace-kicker"><BookOpenText size={14}/> RESEARCH / TRACE VIEW · {trace.runId}</div><h2>研究推理与证据链</h2><p>按观点定位引用、查看反驳关系和结论交接。所有内容均为静态演示样本，同一股票的历史 Demo Run 会复用相同观点。</p></div>
      <div className="trace-count"><strong>{trace.claims.length}</strong><span>模拟观点</span></div>
    </div>
    <div className="trace-tabs" role="tablist" aria-label="研究内容切换">
      {tabs.map(item => <button type="button" role="tab" id={`trace-tab-${item.id}`} key={item.id} aria-controls={`trace-content-${item.id}`} aria-selected={tab === item.id} onClick={() => setTab(item.id)} className={cn(tab === item.id && "active")}>{item.title}</button>)}
    </div>
    <div className="trace-content" id={`trace-content-${tab}`} role="tabpanel" aria-labelledby={`trace-tab-${tab}`}>
      {tab === "claims" && <div className="claim-grid">{trace.claims.map(claim => <ClaimCard key={claim.id} claim={claim} stock={stock} selected={selectedClaimId === claim.id} onSelectClaim={onSelectClaim} onSelectEvidence={onSelectEvidence}/>)}</div>}
      {tab === "rounds" && <div className="debate-timeline">
        {trace.rounds.map((round) => <div className="debate-round" key={round.id}><div className="round-marker"/><div className="round-content"><span className="round-eyebrow">{round.label} · DEMO</span><p>{round.summary}</p><div className="round-claims">{round.claimIds.map(id => {
          const claim = trace.claims.find(c => c.id === id);if (!claim) return null;
          return <button key={id} type="button" onClick={() => {onSelectClaim(id); setTab("claims");}}>{sideLabel[claim.side]} <ArrowRight size={12}/></button>;
        })}</div></div></div>)}
      </div>}
      {tab === "handoff" && <div className="handoff-flow">
        <div className="handoff-step"><span>01 / Evidence</span><strong>Research Artifact</strong><p>{stock.evidence.length} 条静态样例引用，尚未接真实 Provider。</p></div>
        <ArrowRight size={17}/>
        <div className="handoff-step"><span>02 / Debate</span><strong>Bull & Bear</strong><p>相互竞争的假设与反驳，不是实时模型推理。</p></div>
        <ArrowRight size={17}/>
        <div className="handoff-step"><span>03 / Manager</span><strong>MONITOR · 演示</strong><p>仅形成研究观察意图，无任何交易执行。</p></div>
        <ArrowRight size={17}/>
        <div className="handoff-step"><span>04 / Risk</span><strong>NOT_PROVIDED</strong><p>账户未知，无法计算仓位；不推断零仓位。</p></div>
      </div>}
    </div>
    <div className="trace-footer"><ShieldAlert size={14}/><span>模拟观点的「引用关联」仅说明页面关系，<strong>不表示已验证真实市场事实</strong>。真实上线必须使用 Go EvidenceLedger 与运行阶段产物。</span></div>
  </section>;
}
