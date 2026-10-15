"use client";

import { useState, type FormEvent } from "react";

type Exchange = { question: string; answer: string };

// The first R2 chat surface is deliberately stateless: every answer must be
// grounded in tools from persisted Runs, not fabricated browser-side history.
export default function ResearchConversationPanel() {
  const [message, setMessage] = useState("");
  const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const question = message.trim();
    if (!question || busy || question.length > 2048) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/api/workbench/conversation", {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: question }),
      });
      if (!response.ok) throw Error(response.status === 401 ? "登录已过期，请重新登录" : "对话服务暂不可用，请检查模型配置");
      const data: unknown = await response.json();
      if (!data || typeof data !== "object" || typeof (data as { answer?: unknown }).answer !== "string")
        throw Error("对话返回格式错误");
      setExchanges(current => [...current, { question, answer: (data as { answer: string }).answer }]);
      setMessage("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "对话请求失败");
    } finally { setBusy(false); }
  }
  return <section aria-label="研究对话" className="surface" style={{ padding: 20, marginBlock: 16 }}>
    <h2 style={{ fontWeight: 700 }}>研究对话</h2>
    <p style={{ marginBlock: 8, opacity: .75 }}>可询问已有 Run 的研究结论或多空依据。当前版本只读，不会通过聊天启动新研究；每次回答可能产生模型费用。</p>
    <div aria-live="polite">
      {exchanges.map((entry, index) => <div key={index} style={{ paddingBlock: 12, borderBottom: "1px solid var(--line, #ddd)" }}>
        <p><strong>你：</strong>{entry.question}</p>
        <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}><strong>Aegis：</strong>{entry.answer}</p>
      </div>)}
    </div>
    <form onSubmit={submit} style={{ display: "grid", gap: 8, marginTop: 12 }}>
      <label htmlFor="aegis-conversation-message">询问已有研究</label>
      <textarea id="aegis-conversation-message" value={message} onChange={event => setMessage(event.target.value)}
        maxLength={2048} rows={3} placeholder="例如：请列出最近的研究 Run；600519.SH 最近一次研究的空头依据是什么？"
        disabled={busy} style={{ width: "100%", padding: 10, borderRadius: 8 }} />
      <button type="submit" disabled={busy || !message.trim()}>{busy ? "正在分析…" : "发送问题"}</button>
      {error && <p role="alert" style={{ color: "var(--danger, #b42318)" }}>{error}</p>}
    </form>
  </section>;
}
