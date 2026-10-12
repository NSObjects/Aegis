"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowLeftRight, ArrowUpRight, ClipboardList, RefreshCcw } from "lucide-react";
import { isLiveRun, type LiveArtifact, type LiveRun } from "@/lib/live-contract";
import { compareResearchRuns, type ResearchComparison } from "@/lib/run-comparison";
import { workbenchHref } from "@/lib/workbench-location";

type Pair = { leftId: string | null; rightId: string | null };
type Loaded = { key: string; left: LiveRun; right: LiveRun; comparison: ResearchComparison };
const statuses = {
  same: "相同记录", different: "记录不同", only_left: "仅基准有数据",
  only_right: "仅对比有数据", unavailable: "双方均未提供",
};
const time = (s: string) => new Date(s).toLocaleString("zh-CN", { hour12: false });

async function fetchJSON(url: string): Promise<unknown> {
  const res = await fetch(url, { cache: "no-store", credentials: "same-origin" });
  if (!res.ok) throw Error("读取服务端历史记录失败（HTTP " + res.status + "）");
  return res.json();
}
async function loadRun(id: string): Promise<{ run: LiveRun; artifact: LiveArtifact }> {
  // List metadata can omit older runs. Always use exact GET by ID so the
  // requested comparison is independent of the Go list's most-recent cap.
  const raw = await fetchJSON("/api/workbench/runs/" + id);
  if (!isLiveRun(raw) || raw.run_id !== id) throw Error("服务端返回了不匹配的 Run 元数据");
  if (raw.state !== "COMPLETED") throw Error("对比只支持已完成 Run；失败、取消或运行中的任务没有可信研究报告");
  const artifact = await fetchJSON("/api/workbench/runs/" + id + "/artifact");
  if (!artifact || typeof artifact !== "object" || Array.isArray(artifact)) throw Error("服务端未返回有效的研究工件");
  return { run: raw, artifact: artifact as LiveArtifact };
}

/** Read-only comparison: never submits a new research task or executes a model. */
export default function LiveRunComparison({
  runs, pair, onPairChange,
}: {
  runs: LiveRun[];
  pair: Pair;
  onPairChange: (leftId: string | null, rightId: string | null) => void;
}) {
  const { leftId, rightId } = pair;
  const key = (leftId ?? "") + ":" + (rightId ?? "");
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [retry, setRetry] = useState(0);
  const [state, setState] = useState<{ key: string; loading: boolean; error: string }>({ key: "", loading: false, error: "" });
  const completed = useMemo(() => runs.filter(run => run.state === "COMPLETED"), [runs]);
  const baseline = completed.find(run => run.run_id === leftId);
  const compatible = completed.filter(run => run.run_id !== leftId &&
    (!baseline || (run.symbol === baseline.symbol && run.mode === baseline.mode)));

  useEffect(() => {
    if (!leftId || !rightId) return;
    let cancelled = false;
    const activeKey = leftId + ":" + rightId;
    if (leftId === rightId) return;
    Promise.resolve().then(() => {
      if (!cancelled) setState({ key: activeKey, loading: true, error: "" });
      return Promise.all([loadRun(leftId), loadRun(rightId)]);
    }).then(([left, right]) => {
      if (cancelled) return;
      const comparison = compareResearchRuns(left.run, left.artifact, right.run, right.artifact);
      if (!comparison) throw Error("对比来源不合法：要求不同的已完成 Run、同一标的与研究模式，且工件身份必须匹配");
      setLoaded({ key: activeKey, left: left.run, right: right.run, comparison });
      setState({ key: activeKey, loading: false, error: "" });
    }).catch(err => {
      if (!cancelled) setState({ key: activeKey, loading: false, error: (err as Error).message });
    });
    return () => { cancelled = true; };
  }, [leftId, rightId, retry]);

  const shown = loaded?.key === key && state.key === key && !state.loading && !state.error ? loaded : null;
  const loading = leftId !== null && rightId !== null && leftId !== rightId &&
    (state.key !== key || state.loading);
  const invalidSame = leftId !== null && rightId !== null && leftId === rightId;

  const groups = shown ? Array.from(new Set(shown.comparison.rows.map(r => r.section))) : [];
  const options = (items: LiveRun[], selected: string | null) => {
    const values = [...items];
    // Keep the exact deep-linked ID selectable, but do not hide the rest
    // of the list when the server's top-100 response omits an older run.
    return <>
      {selected && !values.some(r => r.run_id === selected) && <option value={selected}>{selected.slice(0, 12)}… · 历史 ID（直接查询）</option>}
      {values.map(r => <option key={r.run_id} value={r.run_id}>
        {r.symbol} · {time(r.requested_as_of)} · {r.run_id.slice(0, 8)}…
      </option>)}
    </>;
  };
  return <section className="surface live-compare" aria-label="历史研究对比">
    <div className="panel-head">
      <div><div className="eyebrow">Historical research / side by side</div><h2>两次研究记录对比</h2>
        <p>仅查看服务端已存储的研究工件；不会重新运行 Agent，也不会产生模型调用。</p></div>
      <ArrowLeftRight size={21}/>
    </div>
    <div className="live-compare-pickers">
      <label>基准研究
        <select aria-label="基准研究" value={leftId ?? ""} onChange={e => {
          const next = e.target.value || null;
          const chosen = completed.find(r => r.run_id === next);
          const oldRight = completed.find(r => r.run_id === rightId);
          onPairChange(next, chosen && oldRight && (chosen.symbol !== oldRight.symbol || chosen.mode !== oldRight.mode || next === rightId) ? null : rightId);
        }}>
          <option value="">选择已完成任务</option>
          {options(completed, leftId)}
        </select>
      </label>
      <label>对比研究
        <select aria-label="对比研究" value={rightId ?? ""} onChange={e => onPairChange(leftId, e.target.value || null)}>
          <option value="">选择另一条已完成任务</option>
          {options(compatible, rightId)}
        </select>
      </label>
    </div>
    {(!leftId || !rightId) && <div className="live-compare-empty"><ClipboardList size={23}/><p>请选择两次已完成研究。相同标的、相同研究模式才能比较；仅有一条历史记录时请先完成另一轮研究。</p></div>}
    {invalidSame && <p className="live-compare-error" role="alert">不能将同一 Run 与自己比较，请选择两条不同的研究记录。</p>}
    {loading && <p className="live-compare-empty" role="status">正在读取两次历史 Run 与各自的原始工件…</p>}
    {leftId && rightId && state.key === key && state.error && <p className="live-compare-error" role="alert">{state.error} <button type="button" className="btn" onClick={() => setRetry(value => value + 1)}><RefreshCcw size={14}/>重新读取</button></p>}
    {shown && <>
      <div className="live-compare-heads">
        <div><small>基准研究 · Source A</small><strong>{shown.left.symbol}</strong><span>PIT {time(shown.left.requested_as_of)}</span>
          <a href={workbenchHref("/", "workspace", shown.left.run_id)}>查看原任务与报告 <ArrowUpRight size={13}/></a><code>{shown.left.run_id}</code></div>
        <div><small>对比研究 · Source B</small><strong>{shown.right.symbol}</strong><span>PIT {time(shown.right.requested_as_of)}</span>
          <a href={workbenchHref("/", "workspace", shown.right.run_id)}>查看原任务与报告 <ArrowUpRight size={13}/></a><code>{shown.right.run_id}</code></div>
      </div>
      <p className="live-compare-notice">标的与模式：{shown.comparison.symbol} · {shown.comparison.mode}。
        两次研究记录的差异不代表市场变化原因，也不保证模型/源数据版本一致。仅对实际同时存在的字段判断“记录不同”；缺失不等于零。
      </p>
      <div className="live-compare-stats">
        <span><strong>{shown.comparison.changed}</strong> 个共同存在字段的值不同</span>
        <span><strong>{shown.comparison.missing}</strong> 个字段含未提供数据</span>
      </div>
      {groups.map(section => <div className="live-compare-section" key={section}>
        <h3>{section}</h3>
        <div className="live-compare-table" role="table" aria-label={section + "对比"}>
          <div role="row" className="live-compare-row live-compare-table-head"><span role="columnheader">数据字段</span><span role="columnheader">基准研究</span><span role="columnheader">对比研究</span></div>
          {shown.comparison.rows.filter(r => r.section === section).map(row =>
            <div role="row" className={"live-compare-row diff-" + row.kind} key={row.key}>
              <span role="cell" className="live-compare-label"><strong>{row.label}</strong><small>{statuses[row.kind]}</small></span>
              <span role="cell">{row.left ?? "未提供"}</span>
              <span role="cell">{row.right ?? "未提供"}</span>
            </div>)}
        </div>
      </div>)}
      <div className="live-compare-section">
        <h3>事件 ID 证据差异</h3>
        {shown.comparison.evidence.added === null ? <p className="live-compare-note">
          至少一侧没有可用、无歧义的完整事件 ID 列表；不能推导新增或消失事件。
        </p> : <div className="live-compare-evidence">
          <div><strong>仅对比研究出现的事件 ID</strong><p>{shown.comparison.evidence.added.join("、") || "无（双方事件列表均已提供）"}</p></div>
          <div><strong>仅基准研究出现的事件 ID</strong><p>{shown.comparison.evidence.removed?.join("、") || "无（双方事件列表均已提供）"}</p></div>
        </div>}
        <p className="live-compare-note">这里只比对后端显式事件 ID。事件在某次研究中缺席，并不等于事件未发生或已失效。</p>
      </div>
    </>}
  </section>;
}
