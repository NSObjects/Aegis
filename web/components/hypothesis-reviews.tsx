"use client";

import { useEffect, useState, type FormEvent } from "react";

import { reviewCategories as categories } from "@/lib/research-memory";

type Evidence = { reference: string; detail: string };
type Review = {
  review_id: string;
  observation_run_id: string;
  category: keyof typeof categories;
  reason: string;
  supporting_evidence: Evidence[];
  counterexamples: Evidence[];
};

export default function HypothesisReviews({ endpoint, observations, runID, hypothesisID }: {
  endpoint: string;
  runID: string;
  hypothesisID: string;
  observations: { run_id: string; as_of: string }[];
}) {
  const [reviews, setReviews] = useState<Review[] | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [memoryObservation, setMemoryObservation] = useState("");
  const [memoryMessage, setMemoryMessage] = useState("");
  const [savingMemory, setSavingMemory] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    fetch(endpoint, { cache: "no-store", signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw Error();
        const data: { reviews: Review[] } = await response.json();
        if (!controller.signal.aborted) setReviews(data.reviews);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("无法读取复盘记录。");
      });
    return () => controller.abort();
  }, [endpoint]);

  async function archive(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const fields = new FormData(form);
    const value = (name: string) => String(fields.get(name) ?? "").trim();
    const counterexample = { reference: value("counter_ref"), detail: value("counter_detail") };
    if (Boolean(counterexample.reference) !== Boolean(counterexample.detail)) {
      setError("填写反例时，请同时提供来源和内容。");
      return;
    }
    setError("");
    setSaving(true);
    try {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          observation_run_id: value("observation"),
          category: value("category"),
          reason: value("reason"),
          supporting_evidence: [{ reference: value("support_ref"), detail: value("support_detail") }],
          counterexamples: counterexample.reference ? [counterexample] : [],
        }),
      });
      if (!response.ok) {
        setError(response.status === 400 ? "请填写错误类型、原因及支撑证据。"
          : "无法保存复盘。请确认关联的后验观察仍可读取。");
        return;
      }
      const item: Review = await response.json();
      setReviews(current => current?.some(record => record.review_id === item.review_id)
        ? current : [...(current ?? []), item]);
      form.reset();
    } catch {
      setError("保存复盘失败，请重试。");
    } finally {
      setSaving(false);
    }
  }

  async function saveMemory(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const fields = new FormData(form);
    setSavingMemory(true);
    setMemoryMessage("");
    try {
      const response = await fetch("/api/workbench/runs/memory", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          source_run_id: runID, hypothesis_id: hypothesisID, observation_run_id: memoryObservation,
          review_id: String(fields.get("review_id") ?? ""), market_phase: String(fields.get("market_phase") ?? ""),
        }),
      });
      if (!response.ok) throw Error();
      setMemoryMessage("已收入研究记忆。");
    } catch {
      setMemoryMessage("无法收入研究记忆。");
    } finally {
      setSavingMemory(false);
    }
  }

  return <>
  return <details>
    <summary>错误复盘</summary>
    {reviews?.map(review => <div key={review.review_id}>
      <p><strong>{categories[review.category]}</strong> · {review.reason}
        {" · "}<a href={`/?run=${review.observation_run_id}`}>后续报告</a>
      </p>
      {review.supporting_evidence.map((evidence, index) => <p key={index}>支撑 · {evidence.reference} · {evidence.detail}</p>)}
      {review.counterexamples.map((evidence, index) => <p key={index}>反例 · {evidence.reference} · {evidence.detail}</p>)}
    </div>)}
    <form onSubmit={archive}>
      <label>关联观察<select name="observation" required>
        <option value="">选择后续报告</option>
        {observations.map(observation => <option key={observation.run_id} value={observation.run_id}>
          {observation.run_id.slice(0, 12)} · {new Date(observation.as_of).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })}
        </option>)}
      </select></label>
      <label>错误类型<select name="category" required>
        <option value="">选择类型</option>
        {Object.entries(categories).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
      </select></label>
      <label>复盘原因<input name="reason" maxLength={600} required /></label>
      <label>支撑证据来源<input name="support_ref" maxLength={150} required /></label>
      <label>支撑证据内容<input name="support_detail" maxLength={150} required /></label>
      <label>反例来源（选填）<input name="counter_ref" maxLength={150} /></label>
      <label>反例内容（选填）<input name="counter_detail" maxLength={150} /></label>
      <button className="btn" type="submit" disabled={saving || reviews === null}>归档复盘</button>
    </form>
    {error && <p role="alert">{error}</p>}
  </details>
  <details>
    <summary>收入研究记忆</summary>
    <form onSubmit={saveMemory}>
      <label>记忆来源观察<select required value={memoryObservation} onChange={event => {
        setMemoryObservation(event.target.value);
        setMemoryMessage("");
      }}>
        <option value="">选择后续报告</option>
        {observations.map(observation => <option key={observation.run_id} value={observation.run_id}>
          {observation.run_id.slice(0, 12)} · {new Date(observation.as_of).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })}
        </option>)}
      </select></label>
      <label>关联复盘（选填）<select name="review_id" key={memoryObservation}>
        <option value="">无关联复盘</option>
        {reviews?.filter(review => review.observation_run_id === memoryObservation).map(review =>
          <option key={review.review_id} value={review.review_id}>{categories[review.category]} · {review.reason}</option>)}
      </select></label>
      <label>市场阶段（选填）<input name="market_phase" maxLength={40} /></label>
      <button className="btn" type="submit" disabled={savingMemory}>保存到记忆</button>
    </form>
    {memoryMessage && <p role="status">{memoryMessage}</p>}
  </details>
  </>;
}
