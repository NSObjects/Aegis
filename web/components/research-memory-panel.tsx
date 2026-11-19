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
  feedback?: Feedback[];
};
type Feedback = {
  archive_sha256: string;
  strategy_version: string;
  test_through: string;
  direction: string;
  selection: string;
  data_quality: string;
  timing: string;
  candidate_outcome?: { source: string };
};

const feedbackLabels: Record<string, string> = {
  UNASSESSED: "待评估", SUPPORTED: "支持", REFUTED: "反驳",
  TRUE_POSITIVE: "命中", TRUE_NEGATIVE: "正确排除", FALSE_POSITIVE: "误选", FALSE_NEGATIVE: "漏选",
  EVALUABLE: "可评估", MISSING_CALENDAR_WINDOW: "交易日窗口不足",
  HALTED_SESSION_IN_WINDOW: "窗口内停牌", MISSING_PIT_ADJUSTMENT: "缺少可用复权因子",
  MISSING_TRADING_SESSION_DATA: "缺少交易日数据", ARCHIVE_CUTOFF_CHECKED: "已检查归档时点",
  LABEL_AFTER_TEST_WINDOW: "结果标签晚于测试窗口",
};

function FeedbackPanel({ item, onSaved }: { item: Memory; onSaved: () => void }) {
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const label = (value: string) => feedbackLabels[value] ?? value;
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const archive = new FormData(form).get("archive");
    if (!(archive instanceof File) || !archive.size || archive.size > 16 * 1024 * 1024) {
      setMessage("请选择不超过 16 MiB 的回测验证 JSON。");
      return;
    }
    setMessage("");
    setSaving(true);
    try {
      const response = await fetch(`${endpoint}/${item.memory_id}/feedback`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: archive,
      });
      if (response.status === 409) {
        setMessage("归档与此记忆的标的、原研究时点或后续观察不匹配。");
        return;
      }
      if (!response.ok) throw Error();
      form.reset();
      setMessage("已记录回测反馈。");
      onSaved();
    } catch {
      setMessage("归档验证失败。请确认上传的是回测验证输入。");
    } finally {
      setSaving(false);
    }
  }
  return <details>
    <summary>回测反馈</summary>
    {item.feedback?.map(record => <div key={record.archive_sha256}>
      <p>{record.strategy_version} · 样本截至 {record.test_through}</p>
      <p>方向 {label(record.direction)} · 筛选 {label(record.selection)} · 数据 {label(record.data_quality)} · 时点 {label(record.timing)}</p>
      {record.candidate_outcome && <p>结果标签来源 · {record.candidate_outcome.source}</p>}
      <p><code title={record.archive_sha256}>{record.archive_sha256.slice(0, 12)}…</code></p>
    </div>)}
    <form onSubmit={save}>
      <label>回测验证 JSON<input type="file" name="archive" accept=".json,application/json" required /></label>
      <button className="btn" type="submit" disabled={saving}>核验并附上反馈</button>
    </form>
    {message && <p role="status">{message}</p>}
  </details>;
}

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
        <FeedbackPanel item={item} onSaved={() => setRevision(value => value + 1)} />
        <button className="btn" type="button" disabled={revoking} onClick={() => void revoke(item)}>撤销记忆</button>
      </div>)}
      {(offset > 0 || results?.has_more) && <nav aria-label="研究记忆分页">
        <button className="btn" type="button" disabled={!offset} onClick={() => setOffset(value => value - 50)}>上一页</button>
        <button className="btn" type="button" disabled={!results?.has_more} onClick={() => setOffset(value => value + 50)}>下一页</button>
      </nav>}
    </div>
  </details>;
}
