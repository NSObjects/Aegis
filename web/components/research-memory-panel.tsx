"use client";

import { useEffect, useState, type FormEvent } from "react";
import { memoryOutcomes, reviewCategories } from "@/lib/research-memory";

const endpoint = "/api/workbench/runs/memory";

type Memory = {
  memory_id: string;
  symbol: string;
  thesis: string;
  outcome: keyof typeof memoryOutcomes;
  market_phase?: string;
  category?: keyof typeof reviewCategories;
  source_run_id: string;
  observation_run_id: string;
};
type Results = { memories: Memory[]; total: number; has_more: boolean };

export default function ResearchMemoryPanel() {
  const [expanded, setExpanded] = useState(false);
  const [query, setQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [results, setResults] = useState<Results | null>(null);
  const [error, setError] = useState("");
  const [revoking, setRevoking] = useState(false);

  useEffect(() => {
    if (!expanded) return;
    const controller = new AbortController();
    const params = new URLSearchParams(query);
    params.set("offset", String(offset));
    fetch(`${endpoint}?${params}`, { cache: "no-store", signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw Error();
        const data: Results = await response.json();
        if (!controller.signal.aborted) setResults(data);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("研究记忆查询失败。请检查筛选条件后重试。");
      });
    return () => controller.abort();
  }, [expanded, query, offset, revision]);

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const params = new URLSearchParams();
    for (const [key, value] of new FormData(event.currentTarget)) {
      const term = String(value).trim();
      if (term) params.set(key, term);
    }
    setError("");
    setResults(null);
    setOffset(0);
    setQuery(params.toString());
    setRevision(value => value + 1);
  }

  async function revoke(item: Memory) {
    if (!window.confirm("撤销这条研究记忆？撤销后它将从检索中移除。")) return;
    setRevoking(true);
    setError("");
    try {
      const response = await fetch(`${endpoint}/${item.memory_id}/revoke`, { method: "POST" });
      if (!response.ok) throw Error();
      setOffset(0);
      setRevision(value => value + 1);
    } catch {
      setError("撤销失败，请重试。");
    } finally {
      setRevoking(false);
    }
  }

  return <details className="surface live-hypotheses" onToggle={event => setExpanded(event.currentTarget.open)}>
    <summary>研究记忆</summary>
    <div className="live-create-body">
      <form onSubmit={search}>
        <label>记忆标的<input name="symbol" pattern="[0-9]{6}\.[a-zA-Z]{2}" placeholder="600519.SH" /></label>
        <label>研究论点<input name="q" maxLength={150} /></label>
        <label>市场阶段<input name="market_phase" maxLength={40} /></label>
        <label>失败类型<select name="category">
          <option value="">全部</option>
          {Object.entries(reviewCategories).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select></label>
        <label>验证结果<select name="outcome">
          <option value="">全部</option>
          {Object.entries(memoryOutcomes).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select></label>
        <button className="btn" type="submit">检索研究记忆</button>
      </form>
      {error && <p role="alert">{error}</p>}
      {!results && !error && expanded && <p role="status">正在读取记忆…</p>}
      {results && <p role="status">{results.total ? `找到 ${results.total} 条记忆` : "没有符合条件的记忆。"}</p>}
      {results?.memories.map(item => <div key={item.memory_id}>
        <strong>{item.symbol} · {item.thesis}</strong>
        <p>{memoryOutcomes[item.outcome]}
          {item.market_phase && ` · ${item.market_phase}`}{item.category && ` · ${reviewCategories[item.category]}`}
          {" · "}<a href={`/?run=${item.source_run_id}`}>原报告</a>
          {" · "}<a href={`/?run=${item.observation_run_id}`}>后续报告</a>
        </p>
        <button className="btn" type="button" disabled={revoking} onClick={() => void revoke(item)}>撤销记忆</button>
      </div>)}
      {(offset > 0 || results?.has_more) && <nav aria-label="研究记忆分页">
        <button className="btn" type="button" disabled={!offset} onClick={() => setOffset(value => value - 50)}>上一页</button>
        <button className="btn" type="button" disabled={!results?.has_more} onClick={() => setOffset(value => value + 50)}>下一页</button>
      </nav>}
    </div>
  </details>;
}
