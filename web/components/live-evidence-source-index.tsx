import type { LiveMode, RunEvidenceIndex } from "@/lib/live-contract";

/**
 * An audit of saved EventContext identities, not independent verification of
 * publisher authenticity or an archive/document download browser.
 */
export default function LiveEvidenceSourceIndex({
  index, mode,
}: { index?: RunEvidenceIndex; mode: LiveMode }) {
  return <details className="live-evidence-source-index" aria-label="运行证据来源索引">
    <summary>本次事件与文档引用索引 · {index?.schema_version === 1 ? "已记录" : "未记录"}</summary>
    {!index ? <p>{mode === "DECISION"
      ? "Decision 模式只保存决策结果，不包含独立可复查的 EventContext；无法生成事件来源索引。"
      : "该运行未保存事件来源索引（可能是旧记录或缺少 EventContext），不能据当前数据库反推过去使用的证据。"
    }</p> : <>
      <div className="live-evidence-source-meta">
        <div><small>事件研究状态</small><strong>{index.status}</strong></div>
        <div><small>请求截止时间</small><strong>{index.requested_as_of}</strong></div>
        <div><small>证据观察时间（如有）</small><strong>{index.evidence_as_of ?? "未记录"}</strong></div>
        <div><small>本次引用事件 / 不重复文档 ID</small><strong>{index.events.length} / {index.document_ids.length}</strong></div>
      </div>
      {index.status === "NO_EVENT" && <p>该次 EventContext 明确返回 NO_EVENT；不是自动证明市场没有相关新闻。</p>}
      {index.status === "INSUFFICIENT_EVIDENCE" && <p>该次 EventContext 返回证据不足，不能当作没有风险或没有信息。</p>}
      {index.events.length > 0 && <div className="live-evidence-source-events">
        {index.events.map(item=><article key={item.event_id}>
          <div className="live-evidence-source-event-title">
            <strong>事件 ID：<code>{item.event_id}</code></strong>
            <span>工件报告来源数：{item.source_count}</span>
          </div>
          {item.occurred_at && <small>事件发生时间：{item.occurred_at}</small>}
          <div className="live-evidence-source-docs">
            {item.document_ids.length ? item.document_ids.map(id=><code key={id}>{id}</code>)
              : <span>没有可索引的文档 ID</span>}
          </div>
        </article>)}
      </div>}
      <p>仅从同次完成的 Go Typed Artifact 中提取事件 / 文档 ID。这里没有原文 URL、文档正文或独立的来源真实性认证；来源数量是研究工件给出的值，不代表已经对原文逐项审计。</p>
    </>}
  </details>;
}
