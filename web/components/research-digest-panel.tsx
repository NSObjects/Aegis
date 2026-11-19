"use client";

import { useEffect, useState } from "react";

type Period = "daily" | "weekly";
type DigestAlert = {
  rule_id: string;
  symbol: string;
  trade_date: string;
  outcome: "FIRED" | "SOURCE_UNAVAILABLE";
  detail: string;
  trigger_state?: "PENDING" | "RUNNING" | "COMPLETED" | "FAILED";
  run_id?: string;
  previous_run_id?: string;
  failure_code?: string;
};
type Digest = {
  from: string;
  through: string;
  alerts: DigestAlert[];
  source_outages: number;
};
const triggerLabels = {
  PENDING: "待处理",
  RUNNING: "处理中",
  COMPLETED: "已处理",
  FAILED: "处理失败",
};

function showDate(value: string) {
  return `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6)}`;
}

export default function ResearchDigestPanel({ onOpenRun }: { onOpenRun: (id: string) => void }) {
  const [period, setPeriod] = useState<Period>("daily");
  const [revision, setRevision] = useState(0);
  const [digest, setDigest] = useState<Digest | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const response = await fetch(`/api/workbench/research-digest?period=${period}`, {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) {
          throw Error(response.status === 503 ? "监控摘要服务未启用或暂不可用。" : "读取研究摘要失败。");
        }
        const data: Digest = await response.json();
        if (!controller.signal.aborted) setDigest(data);
      } catch (failure) {
        if (!controller.signal.aborted) setError(failure instanceof Error ? failure.message : "读取研究摘要失败。");
      }
    }
    void load();
    return () => controller.abort();
  }, [period, revision]);

  function reset() {
    setDigest(null);
    setError(null);
  }

  return <section className="surface live-history" aria-label="监控与研究摘要">
    <div className="panel-head">
      <h2>监控与研究摘要</h2>
      <div className="live-history-actions">
        <label>时段 <select value={period} onChange={event => {
          reset();
          setPeriod(event.target.value === "weekly" ? "weekly" : "daily");
        }}>
          <option value="daily">当日</option>
          <option value="weekly">最近 7 天</option>
        </select></label>
        <button type="button" className="btn" onClick={() => {
          reset();
          setRevision(value => value + 1);
        }}>刷新摘要</button>
      </div>
    </div>
    {error ? <p className="live-empty" role="alert">{error}</p>
      : !digest ? <p className="live-empty" role="status">正在读取摘要…</p>
      : <>
        <p className="live-empty">{showDate(digest.from)} 至 {showDate(digest.through)} · {digest.alerts.length} 条提醒</p>
        {digest.source_outages > 0 && <p className="live-alert" role="alert">
          {digest.source_outages} 条监控因数据源不可用而无法判定。
        </p>}
        {digest.alerts.length === 0 && <p className="live-empty">此时段无已记录的提醒。</p>}
        <ul>
          {digest.alerts.map(alert => {
            const { run_id: runID, previous_run_id: previousRunID } = alert;
            return <li key={`${alert.rule_id}/${alert.trade_date}`}>
              <strong>{alert.symbol}</strong> · {showDate(alert.trade_date)} · {alert.outcome === "SOURCE_UNAVAILABLE"
                ? "数据源不可用"
                : alert.detail}
              {alert.trigger_state && <> · {triggerLabels[alert.trigger_state]}</>}
              {alert.failure_code && <> · <code>{alert.failure_code}</code></>}
              {runID && <button type="button" className="btn" onClick={() => onOpenRun(runID)}>查看触发研究</button>}
              {previousRunID && <button type="button" className="btn" onClick={() => onOpenRun(previousRunID)}>查看此前报告</button>}
            </li>;
          })}
        </ul>
      </>}
  </section>;
}
