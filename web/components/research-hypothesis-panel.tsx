"use client";

import { useEffect, useState, type FormEvent } from "react";
import HypothesisReviews from "@/components/hypothesis-reviews";

type Evidence = { reference: string; detail: string };
type ResearchHypothesis = {
  hypothesis_id: string;
  document_rule?: { document_id: string; on_match: "SUPPORTED" | "REFUTED" };
  close_rule?: { trade_date: string; operator: "GTE" | "LTE"; threshold: number };
  statement: string;
  expires_at: string;
  invalidation: string;
  verification_rule: string;
  supporting_evidence: Evidence[];
  contrary_evidence: Evidence[];
};
const marketDateTime = new Intl.DateTimeFormat("zh-CN", {
  timeZone: "Asia/Shanghai", dateStyle: "medium", timeStyle: "short",
});

type Observation = {
  run_id: string;
  as_of: string;
  status: "SUPPORTED" | "REFUTED" | "INCONCLUSIVE" | "EXPIRED";
  reason: string;
  close?: number;
  document_id?: string;
};

const statusLabels = {
  SUPPORTED: "支持", REFUTED: "反驳", INCONCLUSIVE: "证据不足", EXPIRED: "已过期",
};
const reasonLabels: Record<string, string> = {
  NO_STRUCTURED_RULE: "该假设只有文本规则",
  MARKET_CLOSE_NOT_REACHED: "后续研究时点早于目标日收盘",
  DOCUMENT_UNAVAILABLE: "报告缺少该时段可用的指定原始公告",
  CLOSE_UNAVAILABLE: "报告缺少目标日未复权收盘价",
};

function Observations({ runID, hypothesisID }: { runID: string; hypothesisID: string }) {
  const hypothesisURL = `/api/workbench/runs/${runID}/hypotheses/${hypothesisID}`;
  const endpoint = `${hypothesisURL}/observations`;
  const [records, setRecords] = useState<Observation[] | null>(null);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    fetch(endpoint, { cache: "no-store", signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw Error();
        const data = await response.json();
        if (!controller.signal.aborted) setRecords(data.observations);
      })
      .catch(() => {
        if (!controller.signal.aborted) setMessage("无法读取观察记录。");
      });
    return () => controller.abort();
  }, [endpoint]);

  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ run_id: String(new FormData(form).get("observation_run")).trim() }),
      });
      if (!response.ok) {
        setMessage("请选择同一标的、研究时点更晚且已完成的报告。");
        return;
      }
      const next: Observation = await response.json();
      setRecords(current => [...(current ?? []).filter(record => record.run_id !== next.run_id), next]);
      form.reset();
    } catch {
      setMessage("无法保存观察记录。");
    } finally {
      setBusy(false);
    }
  }

  return <>
    {records?.map(record => <p key={record.run_id}>
      <strong>{statusLabels[record.status]}</strong> · {marketDateTime.format(new Date(record.as_of))}
      {record.close !== undefined ? ` · 收盘价 ${record.close}` : record.document_id ? ` · 公告 ${record.document_id}` : ` · ${reasonLabels[record.reason] ?? "缺少可判定的证据"}`}
      {" · "}<a href={`/?run=${record.run_id}`}>来源报告</a>
    </p>)}
    <form onSubmit={add}>
      <label>后续报告任务 ID<input name="observation_run" pattern="[a-f0-9]{32}" required /></label>
      <button className="btn" type="submit" disabled={busy || records === null}>记录后验观察</button>
    </form>
    {message && <p role="alert">{message}</p>}
    {records && records.length > 0 && <HypothesisReviews endpoint={`${hypothesisURL}/reviews`} observations={records} runID={runID} hypothesisID={hypothesisID} />}
  </>;
}

export default function ResearchHypothesisPanel({ runID }: { runID: string }) {
  const endpoint = `/api/workbench/runs/${runID}/hypotheses`;
  const [expanded, setExpanded] = useState(false);
  const [items, setItems] = useState<ResearchHypothesis[] | null>(null);
  const [revision, setRevision] = useState(0);
  const [loadingError, setLoadingError] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!expanded) return;
    const controller = new AbortController();
    async function load() {
      try {
        const response = await fetch(endpoint, { cache: "no-store", signal: controller.signal });
        if (!response.ok) throw Error("hypothesis list unavailable");
        const data: { hypotheses: ResearchHypothesis[] } = await response.json();
        if (!controller.signal.aborted) setItems(data.hypotheses);
      } catch {
        if (!controller.signal.aborted) setLoadingError(true);
      }
    }
    void load();
    return () => controller.abort();
  }, [endpoint, expanded, revision]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const value = (name: string) => String(data.get(name) ?? "").trim();
    function evidence(side: string) {
      const reference = value(`${side}_reference`);
      const detail = value(`${side}_detail`);
      return reference || detail ? [{ reference, detail }] : [];
    }
    const documentID = value("verify_document");
    const tradeDate = value("close_date").replaceAll("-", "");
    const operator = value("close_operator");
    const threshold = value("close_threshold");
    if ((tradeDate || threshold) && (!tradeDate || !threshold)) {
      setSaveError("请同时填写目标交易日和收盘价阈值。");
      return;
    }
    if (documentID && tradeDate) {
      setSaveError("请选择一种结构化规则。");
      return;
    }
    setSaving(true);
    setSaveError("");
    try {
      const expiresAt = new Date(`${value("expires_at")}+08:00`).toISOString();
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          statement: value("statement"),
          expires_at: expiresAt,
          invalidation: value("invalidation"),
          verification_rule: value("verification_rule"),
          supporting_evidence: evidence("supporting"),
          contrary_evidence: evidence("contrary"),
          ...(documentID && { document_rule: { document_id: documentID, on_match: value("document_outcome") } }),
          ...(tradeDate && { close_rule: { trade_date: tradeDate, operator, threshold: Number(threshold) } }),
        }),
      });
      if (response.status === 400) {
        setSaveError("请填写假设、失效条件及验证规则。有效期须晚于研究时点，填写证据时须同时提供来源和内容。");
        return;
      }
      if (!response.ok) throw Error("hypothesis write unavailable");
      const item: ResearchHypothesis = await response.json();
      setItems(current => [...(current ?? []).filter(previous => previous.hypothesis_id !== item.hypothesis_id), item]);
      form.reset();
    } catch {
      setSaveError("保存假设失败。请确认报告仍可读取后重试。");
    } finally {
      setSaving(false);
    }
  }

  return <details className="surface live-hypotheses" onToggle={event => setExpanded(event.currentTarget.open)}>
    <summary>研究假设</summary>
    <div className="live-create-body">
      {loadingError ? <p role="alert">无法读取研究假设。 <button type="button" className="btn" onClick={() => {
        setLoadingError(false);
        setRevision(value => value + 1);
      }}>重试</button></p>
        : items === null ? <p role="status">正在读取研究假设…</p>
        : items.length === 0 ? <p>这份报告尚无记录的假设。</p>
        : <ul>{items.map(item => <li key={item.hypothesis_id}>
          <strong>{item.statement}</strong>
          <p>有效至 {marketDateTime.format(new Date(item.expires_at))}（上海时间）</p>
          <p>失效条件 · {item.invalidation}</p>
          <p>验证规则 · {item.verification_rule}</p>
          {item.supporting_evidence.map((evidence, index) => <p key={index}>支持 · {evidence.reference} · {evidence.detail}</p>)}
          {item.contrary_evidence.map((evidence, index) => <p key={index}>反证 · {evidence.reference} · {evidence.detail}</p>)}
          {item.close_rule && <p>目标日收盘价 · {item.close_rule.trade_date} · {item.close_rule.operator === "GTE" ? "≥" : "≤"} {item.close_rule.threshold}</p>}
          {item.document_rule && <p>公告 {item.document_rule.document_id} · 出现时{statusLabels[item.document_rule.on_match]}</p>}
          <Observations runID={runID} hypothesisID={item.hypothesis_id} />
        </li>)}</ul>}
      <form onSubmit={save}>
        <label>假设<input name="statement" maxLength={300} required /></label>
        <label>有效期（上海时间）<input type="datetime-local" name="expires_at" required /></label>
        <label>失效条件<input name="invalidation" maxLength={300} required /></label>
        <label>验证规则<input name="verification_rule" maxLength={300} required /></label>
        <label>收盘价验证日期（选填）<input type="date" name="close_date" /></label>
        <label>收盘价比较<select name="close_operator"><option value="GTE">大于或等于</option><option value="LTE">小于或等于</option></select></label>
        <label>收盘价阈值（选填）<input type="number" step="any" min="0" name="close_threshold" /></label>
        <label>验证用原始公告 ID（选填）<input name="verify_document" maxLength={128} /></label>
        <label>公告出现时<select name="document_outcome"><option value="SUPPORTED">支持假设</option><option value="REFUTED">反驳假设</option></select></label>
        <label>支持证据来源（选填）<input name="supporting_reference" maxLength={150} /></label>
        <label>支持证据内容（选填）<input name="supporting_detail" maxLength={150} /></label>
        <label>反证来源（选填）<input name="contrary_reference" maxLength={150} /></label>
        <label>反证内容（选填）<input name="contrary_detail" maxLength={150} /></label>
        {saveError && <p role="alert">{saveError}</p>}
        <button className="btn primary" type="submit" disabled={saving || items === null}>
          {saving ? "保存中…" : "保存假设"}
        </button>
      </form>
    </div>
  </details>;
}
