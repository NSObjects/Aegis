"use client";

import { useMemo, useState } from "react";
import { projectEvidenceNavigator, type EvidenceLink, type EvidenceEvent } from "@/lib/live-evidence";
import type { LiveArtifact } from "@/lib/live-contract";

type Selection = { kind: "argument"; id: string } | { kind: "insight"; id: number };

function ReferenceLinks({ references, onSelect }: {
  references: EvidenceLink[];
  onSelect: (id: string) => void;
}) {
  if (!references.length) return <p className="evidence-nav-empty">未提供结构化 Reference；不能从自由文本推导证据。</p>;
  return <div className="evidence-nav-references" aria-label="结构化引用">
    {references.map((ref, index) => {
      const description = ref.status === "MATCHED_EVENT"
        ? "匹配到本次研究中的事件记录"
        : ref.status === "EVENT_NOT_FOUND"
          ? "事件 ID 未在本次工件中唯一匹配"
          : ref.status === "DOMAIN_ONLY"
            ? "仅有领域引用，没有具体事件 ID"
            : "无效的引用结构";
      return ref.status === "MATCHED_EVENT" && ref.eventId
        ? <button key={index} type="button" className="evidence-nav-ref linked"
            onClick={() => onSelect(ref.eventId!)} aria-label={`查看事件 ${ref.eventId}`}>
            <strong>{ref.domain} · {ref.eventId}</strong><small>{description} →</small>
          </button>
        : <div key={index} className="evidence-nav-ref unavailable">
            <strong>{ref.domain}{ref.eventId ? " · " + ref.eventId : ""}</strong><small>{description}</small>
          </div>;
    })}
  </div>;
}

function EventDetail({ event }: { event: EvidenceEvent }) {
  return <section className="evidence-nav-event" aria-label="已匹配的事件详情">
    <h4>事件记录 · {event.id}</h4>
    <strong>{event.headline}</strong>
    <p>{event.summary}</p>
    <dl>
      {event.type && <div><dt>事件类型</dt><dd>{event.type}</dd></div>}
      {event.occurredAt && <div><dt>发生时间（后端原值）</dt><dd>{event.occurredAt}</dd></div>}
      {event.sourceCount !== null && <div><dt>已记录来源数量</dt><dd>{event.sourceCount}</dd></div>}
    </dl>
    <h5>引用文档 ID（非原文链接）</h5>
    {event.documents.length
      ? <div className="evidence-nav-documents">{event.documents.map((doc, i) => <code key={i}>{doc.id}</code>)}</div>
      : <p className="evidence-nav-empty">本次 Artifact 未携带文档 ID；不能声称有可查看的原始文件。</p>}
    <small>事件 ID 与文档 ID 仅来自同一 Go 工件。匹配到 ID 不代表已经验证原文内容或来源真实性。</small>
  </section>;
}

export default function LiveEvidenceNavigator({ artifact }: { artifact: LiveArtifact }) {
  const evidence = useMemo(() => projectEvidenceNavigator(artifact), [artifact]);
  const [selection, setSelection] = useState<Selection | null>(() =>
    evidence.arguments[0] ? { kind: "argument", id: evidence.arguments[0].id }
      : evidence.insights[0] ? { kind: "insight", id: 0 } : null);
  const [eventId, setEventId] = useState<string | null>(null);

  if (!evidence.arguments.length && !evidence.insights.length) return null;
  const argument = selection?.kind === "argument"
    ? evidence.arguments.find(item => item.id === selection.id) ?? null : null;
  const insight = selection?.kind === "insight"
    ? evidence.insights[selection.id] ?? null : null;
  const references = argument?.references ?? insight?.references ?? [];
  const event = eventId ? evidence.events.find(item => item.id === eventId) ?? null : null;

  return <section className="surface live-evidence-navigator" aria-label="交互式证据与论点导航">
    <div className="evidence-nav-heading">
      <div className="eyebrow">EvidenceLedger · 结构化引用追溯</div>
      <h3>论点与事件证据导航</h3>
      <p>先选 Bull/Bear 论点或 Research Manager 观点，再打开明确引用的事件。这里展示的是后端已记录的关联，不是网页原文审计，也不是模型隐藏推理。</p>
    </div>
    <div className="evidence-nav-columns">
      <nav className="evidence-nav-claims" aria-label="选择观点">
        <h4>辩论论点 <span>{evidence.arguments.length}</span></h4>
        {evidence.arguments.map(item => <button type="button" key={item.id}
          aria-pressed={selection?.kind === "argument" && selection.id === item.id}
          className={selection?.kind === "argument" && selection.id === item.id ? "selected" : ""}
          onClick={() => { setSelection({ kind: "argument", id: item.id }); setEventId(null); }}>
          <small>{item.side ?? "未指定阵营"} · {item.disposition ?? "未分类"} · {item.id}</small>
          <strong>{item.claim}</strong>
        </button>)}
        {evidence.insights.length > 0 && <>
          <h4>研究经理观点 <span>{evidence.insights.length}</span></h4>
          {evidence.insights.map((item, i) => <button type="button" key={i}
            aria-pressed={selection?.kind === "insight" && selection.id === i}
            className={selection?.kind === "insight" && selection.id === i ? "selected" : ""}
            onClick={() => { setSelection({ kind: "insight", id: i }); setEventId(null); }}>
            <small>{item.kind ?? "INSIGHT"} · 明确引用</small><strong>{item.claim}</strong>
          </button>)}
        </>}
      </nav>
      <div className="evidence-nav-inspection">
        {argument && <>
          <h4>{argument.side ?? "未指定阵营"} 论点 · {argument.id}</h4>
          <p className="evidence-nav-claim-body">{argument.claim}</p>
          <dl className="evidence-nav-meta">
            {argument.round !== null && <div><dt>辩论回合</dt><dd>{argument.round}</dd></div>}
            {argument.disposition && <div><dt>Research Manager 分类</dt><dd>{argument.disposition}</dd></div>}
          </dl>
        </>}
        {insight && <>
          <h4>Research Manager · {insight.kind ?? "观点"}</h4>
          <p className="evidence-nav-claim-body">{insight.claim}</p>
          <h5>显式关联论点 ID</h5>
          {insight.argumentIds.length
            ? <div className="evidence-nav-arguments">{insight.argumentIds.map(id => {
              const available = evidence.arguments.some(item => item.id === id);
              return available
                ? <button type="button" key={id} onClick={() => { setSelection({ kind: "argument", id }); setEventId(null); }}>打开 {id}</button>
                : <span key={id}>未找到论点 {id}</span>;
            })}</div>
            : <p className="evidence-nav-empty">Research Manager 未声明结构化论点 ID；不能据文本相似度创建关联。</p>}
        </>}
        <h5>结构化来源引用</h5>
        <ReferenceLinks references={references} onSelect={setEventId}/>
        {argument && <>
          <h5>针对本论点的反驳 <span>({argument.challenges.length})</span></h5>
          {argument.challenges.length
            ? argument.challenges.map((challenge, i) => <article className="evidence-nav-challenge" key={i}>
                <strong>{challenge.side ?? "未注明阵营"} · {challenge.round === null ? "回合未注明" : "第 " + challenge.round + " 轮"}</strong>
                <p>{challenge.claim}</p>
                <ReferenceLinks references={challenge.references} onSelect={setEventId}/>
              </article>)
            : <p className="evidence-nav-empty">无已记录的定向反驳，不代表论点已被证实。</p>}
        </>}
        {event && <EventDetail event={event}/>}
      </div>
    </div>
  </section>;
}
