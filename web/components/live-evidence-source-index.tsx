import type { LiveMode, RunEvidenceIndex } from "@/lib/live-contract";

/**
 * An audit of saved EventContext identities, not independent verification of
 * publisher authenticity or an archive/document download browser.
 */
function isSafeSourceLink(raw: string): boolean {
  try {
    const parsed = new URL(raw);
    return parsed.protocol === "https:" && Boolean(parsed.hostname) &&
      !parsed.username && !parsed.password && !parsed.search && !parsed.hash &&
      (!parsed.port || parsed.port === "443");
  } catch { return false; }
}

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
      {index.documents?.length ? <div className="live-evidence-source-records">
        <h4>本次实际引用的规范化来源元数据</h4>
        {index.documents.map(source => <article key={source.document_id}>
          <div className="live-evidence-source-event-title">
            <strong>文档 <code>{source.document_id}</code></strong>
            <span>{source.source_type || "来源类型未注明"} · {source.authority || "权威等级未注明"}</span>
          </div>
          <dl>
            <div><dt>发布方</dt><dd>{source.publisher || "未记录"}</dd></div>
            <div><dt>发布时间</dt><dd>{source.published_at || "未记录"}</dd></div>
            <div><dt>可用时间（PIT）</dt><dd>{source.available_at || "未记录"}</dd></div>
            <div><dt>首次观察时间</dt><dd>{source.first_seen_at || "未记录"}</dd></div>
            <div><dt>规范化版本 ID</dt><dd><code>{source.version_id || "未记录"}</code></dd></div>
            <div><dt>已处理正文 SHA256</dt><dd><code>{source.content_sha256 || "未记录"}</code></dd></div>
            <div><dt>来源地址</dt><dd>{source.url && isSafeSourceLink(source.url)
              ? <a href={source.url} target="_blank" rel="noopener noreferrer">访问记录中的来源地址 ↗</a>
              : "未安全记录可访问地址"}</dd></div>
          </dl>
        </article>)}
      </div> : index.document_ids.length > 0 ? <p>该次运行只有文档 ID，没有可追溯的规范化文档元数据；不使用当前源数据库补写历史。</p> : null}
      <p>以上字段直接保留自本次分析引用的 CanonicalDocument。来源地址不保证今天仍可访问；哈希不是独立原文审计，也不证明具体论点真实。包含查询参数或认证信息的 URL 会被省略。</p>
    </>}
  </details>;
}
