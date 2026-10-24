"use client";

import { useEffect, useState, type FormEvent } from "react";

type Exchange = { question: string; answer: string };

// The first R2 chat surface is deliberately stateless: every answer must be
// grounded in tools from persisted Runs, not fabricated browser-side history.
export default function ResearchConversationPanel({ onOpenRun, selectedRunId }: { onOpenRun?: (runId: string) => void; selectedRunId?: string | null }) {
  const [message, setMessage] = useState("");
  const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [busy, setBusy] = useState(false);
  const [startSymbol, setStartSymbol] = useState("");
  const [confirmStart, setConfirmStart] = useState(false);
  const [error, setError] = useState("");
  const [restored, setRestored] = useState("");
  const [currentSession, setCurrentSession] = useState("");
  const [currentRunId, setCurrentRunId] = useState("");
  const [runScopePreference, setRunScopePreference] = useState<{ runId: string | null; enabled: boolean } | null>(null);
  const selectedRunAvailable = Boolean(selectedRunId && /^[a-f0-9]{32}$/.test(selectedRunId));
  const useSelectedRun = runScopePreference?.runId === (selectedRunId ?? null)
    ? runScopePreference.enabled
    : selectedRunAvailable && !currentSession;
  useEffect(() => {
    const id = window.sessionStorage.getItem("aegis-conversation-session");
    if (!id || !/^[a-f0-9]{32}$/.test(id)) return;
    const controller = new AbortController();
    fetch("/api/workbench/conversation/session?session_id=" + id, { credentials: "same-origin", cache: "no-store", signal: controller.signal })
      .then(async r => { if (!r.ok) throw Error("unavailable"); return r.json(); })
      .then((v: { run_id?: string; symbol?: string }) => {
        if (v.run_id && /^[a-f0-9]{32}$/.test(v.run_id)) { setCurrentSession(id); setCurrentRunId(v.run_id); setRestored("已恢复研究 " + (v.symbol ?? "") + " · Run ID: " + v.run_id); }
      }).catch(() => { /* Missing or expired sessions are never fabricated. */ });
    return () => controller.abort();
  }, []);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const runContext = useSelectedRun && selectedRunId && /^[a-f0-9]{32}$/.test(selectedRunId) && !startSymbol.trim() && (!currentSession || selectedRunId !== currentRunId) ? selectedRunId : "";
    const question = runContext ? `请只读查询已有 Run ID ${runContext}。${message.trim()} 不要启动新的研究。` : message.trim();
    if (!question || busy || question.length > 2048) return;
    const starting = startSymbol.trim().length > 0;
    if (starting && (!confirmStart || !/^(?:(?:600|601|603|605|688)[0-9]{3}\.SH|(?:000|001|002|003|300|301|302)[0-9]{3}\.SZ)$/.test(startSymbol.trim().toUpperCase()))) {
      setError("请填写有效的 A 股代码并明确确认启动研究"); return;
    }
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/api/workbench/conversation", {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(starting ? { message: question, start_symbol: startSymbol.trim().toUpperCase(), confirm_start: true, ...(currentSession ? { session_id: currentSession } : {}) } : runContext ? { message: question } : currentSession ? { message: question, session_id: currentSession } : { message: question }),
      });
      if (!response.ok) throw Error(response.status === 401 ? "登录已过期，请重新登录" : "对话服务暂不可用，请检查模型配置");
      const data: unknown = await response.json();
      if (!data || typeof data !== "object" || typeof (data as { answer?: unknown }).answer !== "string")
        throw Error("对话返回格式错误");
      const body = data as { answer: string; run_id?: string; session_id?: string; session_binding?: string };
      if (starting && (!body.run_id || !/^[a-f0-9]{32}$/.test(body.run_id))) throw Error("研究启动响应无有效 Run ID，请到研究列表核实，勿重复启动");
      if (starting && body.session_binding === "FAILED") {
        window.sessionStorage.removeItem("aegis-conversation-session");
        setCurrentSession("");
        setCurrentRunId(body.run_id ?? "");
        setRestored("");
        setExchanges(current => [...current, { question, answer: body.answer + " Run ID: " + body.run_id }]);
        setMessage("");
        setConfirmStart(false);
        setStartSymbol("");
        setError("研究已创建，但历史会话关联失败。请保留 Run ID，勿重复启动。");
        return;
      }
      // Never retain an earlier session when the newly-created Run has no binding.
      // The Run itself remains accessible by its explicit ID; no paid retry.
      if (starting && (!body.session_id || !/^[a-f0-9]{32}$/.test(body.session_id))) {
        window.sessionStorage.removeItem("aegis-conversation-session");
        setCurrentSession("");
        setRestored("");
        setError("研究已创建，但未返回有效会话关联。请保存 Run ID，勿重复启动。");
      }
      if (starting && body.run_id) setCurrentRunId(body.run_id);
      if (starting && body.session_id && /^[a-f0-9]{32}$/.test(body.session_id)) { window.sessionStorage.setItem("aegis-conversation-session", body.session_id); setCurrentSession(body.session_id); setCurrentRunId(body.run_id ?? ""); setRestored("当前研究 · Run ID: " + body.run_id); }
      setExchanges(current => [...current, { question, answer: starting ? `${body.answer} Run ID: ${body.run_id}` : body.answer }]);
      setMessage("");
      setRunScopePreference({ runId: selectedRunId ?? null, enabled: !starting && selectedRunAvailable && !currentSession });
      setConfirmStart(false);
      setStartSymbol("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "对话请求失败");
    } finally { setBusy(false); }
  }
  return <section aria-label="研究对话" className="surface" style={{ padding: 20, marginBlock: 16 }}>
    <h2 style={{ fontWeight: 700 }}>针对研究继续追问</h2>
    <p style={{ marginBlock: 8, opacity: .75 }}>默认只读，可追问本次研究的结论、依据和不确定性。只有在下方主动展开并确认后，才会通过对话启动新研究；模型调用可能产生费用。</p>
    {restored && <p aria-live="polite">{restored}</p>}
    {onOpenRun && /^[a-f0-9]{32}$/.test(currentRunId) && <button type="button" onClick={() => onOpenRun(currentRunId)}>在研究工作台打开此 Run</button>}
    <div aria-live="polite">
      {exchanges.map((entry, index) => <div key={index} style={{ paddingBlock: 12, borderBottom: "1px solid var(--line, #ddd)" }}>
        <p><strong>你：</strong>{entry.question}</p>
        <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}><strong>Aegis：</strong>{entry.answer}</p>
      </div>)}
    </div>
    <form onSubmit={submit} style={{ display: "grid", gap: 8, marginTop: 12 }}>
      {selectedRunId && /^[a-f0-9]{32}$/.test(selectedRunId) && <label><input type="checkbox" checked={useSelectedRun} disabled={busy} onChange={event => setRunScopePreference({ runId: selectedRunId ?? null, enabled: event.target.checked })} /> 针对当前研究 Run {selectedRunId.slice(0, 12)}… 追问（只读）</label>}
      <label htmlFor="aegis-conversation-message">询问已有研究</label>
      <textarea id="aegis-conversation-message" value={message} onChange={event => setMessage(event.target.value)}
        maxLength={2048} rows={3} placeholder="例如：请列出最近的研究 Run；600519.SH 最近一次研究的空头依据是什么？"
        disabled={busy} style={{ width: "100%", padding: 10, borderRadius: 8 }} />
      <details className="conversation-start-research">
        <summary>从对话启动新研究（可选）</summary>
        <label htmlFor="aegis-start-symbol">A 股代码</label>
        <input id="aegis-start-symbol" value={startSymbol} onChange={event => { setStartSymbol(event.target.value); setConfirmStart(false); }}
          placeholder="例如 600519.SH" disabled={busy} maxLength={9} />
        {startSymbol.trim() && <label><input type="checkbox" checked={confirmStart} disabled={busy}
          onChange={event => setConfirmStart(event.target.checked)} /> 我确认允许启动 {startSymbol.trim().toUpperCase()} 的新研究（可能产生费用）</label>}
      </details>
      <button type="submit" disabled={busy || !message.trim()}>{busy ? "正在分析…" : "发送问题"}</button>
      {error && <p role="alert" style={{ color: "var(--danger, #b42318)" }}>{error}</p>}
    </form>
  </section>;
}
