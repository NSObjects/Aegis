"use client";

import { useMemo, useState } from "react";
import { AlertTriangle, ArrowRight, GitBranch, Link2 } from "lucide-react";
import { readLiveEvidence, type EvidenceReference } from "@/lib/live-evidence";
import styles from "./live-evidence-inspector.module.css";

function References({ references }: { references: EvidenceReference[] }) {
  if (references.length === 0) return <p className={styles.muted}>后端未声明结构化引用，不能推断证据支持关系。</p>;
  return <div className={styles.references}>
    {references.map((ref, i) => <div className={styles.reference} key={ref.domain + (ref.eventId ?? "") + i}>
      <div className={styles.referenceLabel}>
        <code>{ref.domain}{ref.eventId ? " · " + ref.eventId : ""}</code>
        <span>{ref.status === "EVENT_MATCHED" ? "本次工件有匹配事件" :
          ref.status === "EVENT_UNRESOLVED" ? "无法定位该事件" :
          ref.status === "UNKNOWN_DOMAIN" ? "未知引用领域" : "领域级引用"}</span>
      </div>
      {ref.status === "EVENT_UNRESOLVED" && <p className={styles.warning}>该事件 ID 未在本次 Event 工件中唯一匹配，不能展示为已有事件事实。</p>}
      {ref.status === "UNKNOWN_DOMAIN" && <p className={styles.warning}>后端返回未识别的引用领域，请核对原始工件。</p>}
      {ref.status === "DOMAIN_ONLY" && <p className={styles.muted}>仅声明引用领域，没有具体文档或事件来源链接。</p>}
      {ref.event && <div className={styles.event}>
        {ref.event.headline && <strong>{ref.event.headline}</strong>}
        {ref.event.factSummary && <p>{ref.event.factSummary}</p>}
        <div className={styles.metadata}>
          {ref.event.occurredAt && <span>事件发生时间：{ref.event.occurredAt}</span>}
          {ref.event.sourceCount !== null && <span>后端标注来源数：{ref.event.sourceCount}</span>}
          {ref.event.documentIds.map((id, index) => <span key={index}>文档 ID：{id}</span>)}
        </div>
        <p className={styles.muted}>事件内容来自本次后端工件；文档 ID 不是可访问的原文 URL。</p>
      </div>}
    </div>)}
  </div>;
}

export default function LiveEvidenceInspector({ artifact }: { artifact: Record<string, unknown> }) {
  const data = useMemo(() => readLiveEvidence(artifact), [artifact]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const active = data.arguments.find(argument => argument.id === selectedId) ?? data.arguments[0] ?? null;
  const relatedInsights = data.insights.filter(insight => active && insight.argumentIds.includes(active.id));

  return <section className={"surface " + styles.inspector} id="report-evidence-inspector" aria-label="论点与证据检查器">
    <header className={styles.header}>
      <div className={styles.heading}>
        <span className={styles.icon}><GitBranch size={19}/></span>
        <div><h3>论点与证据检查器</h3><p>EvidenceLedger · 结构化引用与反驳关系</p></div>
      </div>
      {data.available && <span className={styles.count}>{data.arguments.length} 条论点 · {data.insights.length} 条经理观点</span>}
    </header>
    {!data.available ? <p className={styles.empty}>当前研究工件未提供 EvidenceLedger。无法展示可追溯的论点关系，请查看原始 JSON。</p> :
      <>
        <div className={styles.notice}>所有关系均取自本次运行的后端工件。引用领域、事件 ID 匹配或引用重叠不等于事实已核实，也不等于某个观点获得了证据支持。</div>
        {data.invalidArgumentCount > 0 && <p className={styles.warning}><AlertTriangle size={15}/> {data.invalidArgumentCount} 条论点缺少有效 ID、正文或 ID 重复，已停止为这些论点创建链接。</p>}
        {data.conclusion && <p className={styles.conclusion}>研究经理结论 <strong>{data.conclusion}</strong></p>}
        <div className={styles.grid}>
          <div className={styles.argumentList}>
            <h4>多空论点</h4>
            {data.arguments.length === 0 && <p className={styles.empty}>该 Ledger 没有可辨识的论点，不会推测或补充。</p>}
            {data.arguments.map(argument => <button type="button" key={argument.id}
              className={styles.argumentButton + (active?.id === argument.id ? " " + styles.selected : "")}
              aria-pressed={active?.id === argument.id}
              onClick={() => setSelectedId(argument.id)}>
              <span className={styles.argumentMeta}>
                <code>{argument.id}</code><span>{argument.side ?? "未声明立场"} · 第 {argument.round ?? "?"} 轮</span>
              </span>
              <strong>{argument.claim}</strong>
              <span className={styles.disposition}>{argument.disposition ?? "未声明分类"} <ArrowRight size={13}/></span>
            </button>)}
          </div>
          <div className={styles.details}>
            {active ? <>
              <div className={styles.detailHeader}>
                <h4>当前选中：<code>{active.id}</code></h4>
                <span>{active.disposition ?? "未分类"} · {active.side ?? "未知阵营"}</span>
              </div>
              <p className={styles.claim}>{active.claim}</p>
              <h5><Link2 size={14}/> 论点声明的引用</h5>
              <References references={active.references}/>
              <h5>针对该论点的反驳 · {active.challenges.length}</h5>
              {active.challenges.length === 0 && <p className={styles.muted}>Ledger 没有记录针对该论点的反驳，不能视为已被证实。</p>}
              {active.challenges.map((challenge, i) => <article className={styles.challenge} key={i}>
                <small>{challenge.side ?? "未标注阵营"} · 第 {challenge.round ?? "?"} 轮</small>
                <p>{challenge.claim}</p>
                <References references={challenge.references}/>
              </article>)}
              <h5>明确关联的研究经理观点 · {relatedInsights.length}</h5>
              {relatedInsights.length === 0 && <p className={styles.muted}>没有通过 argument_ids 明确连接的经理观点，不能由相同关键词推断关联。</p>}
              {relatedInsights.map((insight, i) => <article className={styles.insight} key={i}>
                <span>{insight.kind ?? "未标注类型"}</span>
                <p>{insight.claim}</p>
                {insight.audits.filter(audit => audit.argumentId === active.id).map((audit, j) =>
                  <small key={j}>引用重叠诊断：{audit.alignment}（非语义证明）</small>)}
                <References references={insight.references}/>
              </article>)}
            </> : <p className={styles.empty}>请选择一个论点查看引用关系。</p>}
          </div>
        </div>
        {data.insights.length > 0 && <div className={styles.manager}>
          <h4>研究经理的全部观点</h4>
          {data.insights.map((insight, i) => <article className={styles.managerItem} key={i}>
            <span>{insight.kind ?? "未标注类型"}</span><p>{insight.claim}</p>
            <div className={styles.links}>
              {insight.argumentIds.length === 0 && <small>后端未声明关联论点 ID</small>}
              {insight.argumentIds.map((id, j) => data.arguments.some(argument => argument.id === id) ?
                <button key={j} type="button" onClick={() => setSelectedId(id)} aria-label={"查看论点 " + id}>
                  查看论点 {id} <ArrowRight size={12}/>
                </button> :
                <small className={styles.warning} key={j}>关联 ID {id} 未找到匹配论点</small>)}
            </div>
          </article>)}
        </div>}
      </>
    }
  </section>;
}
