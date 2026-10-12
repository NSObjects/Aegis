"use client";

import { Activity, Clock3, Copy, Wifi, WifiOff } from "lucide-react";
import { useState } from "react";
import { formatFailureDiagnostic } from "@/lib/failure-diagnostic";
import type { LiveArtifact, LiveRun, LiveStageEvent } from "@/lib/live-contract";
import { readLiveReport } from "@/lib/live-report";
import { buildRunTrace, stageLabels, stageReportId, type StagePhase } from "@/lib/stage-trace";

type StreamStatus = "connected" | "disconnected" | "idle";

const failureDescriptions: Record<string, string> = {
  TECHNICAL_MODEL_EMPTY: "技术分析 Agent 没有返回可解析的内容",
  TECHNICAL_JSON_DECODE: "技术分析上下文 JSON 解析失败",
  TECHNICAL_CONTEXT_VALIDATION: "技术分析的标的、研究日期或结构校验失败",
  MARKET_MODEL_EMPTY: "市场分析 Agent 没有返回可解析的内容",
  MARKET_JSON_DECODE: "市场分析上下文 JSON 解析失败",
  MARKET_CONTEXT_VALIDATION: "市场分析结果未通过上下文校验",
  EVENT_JSON_DECODE: "事件分析模型输出无法解析为符合要求的 JSON",
  EVENT_MODEL_GENERATE: "事件分析模型生成调用失败",
  EVENT_MODEL_EMPTY: "事件分析模型返回空内容",
  EVENT_CONTEXT_VALIDATION: "事件研究结果未通过结构校验",
  EVENT_SYNTHESIS_VALIDATION: "事件研究结论未通过校验",
  PERSISTENCE: "研究记录持久化失败",
  INVALID_RESULT: "研究产物不符合结果契约",
  PROCESS_INTERRUPTED: "研究执行期间服务进程中断",
  UNKNOWN: "尚未定位具体原因；请查看已记录的阶段与工具事件",
};

const phaseLabels: Record<StagePhase, string> = {
  RUNNING: "执行中", DONE: "成功", FAIL: "失败",
  INCOMPLETE: "未记录完成", UNKNOWN: "未知状态",
};

const formatTime = (s: string | null) => s ? new Date(s).toLocaleString("zh-CN", { hour12: false }) : "—";

function duration(ms: number | null) {
  return ms === null ? "未提供" : ms >= 1000 ? (ms / 1000).toFixed(2) + "s" : ms + "ms";
}

/** Render only server-observed progress and links to matching completed artifacts. */
export default function LiveStageTrace({
  run, events, artifact, stream,
}: {
  run: LiveRun | null;
  events: LiveStageEvent[];
  artifact: LiveArtifact | null;
  stream: StreamStatus;
}) {
  const [copyStatus, setCopyStatus] = useState("");
  const trace = run ? buildRunTrace(run, events) : null;
  const terminal = run?.state === "COMPLETED" || run?.state === "FAILED" || run?.state === "CANCELLED";
  const reportIds = artifact && run?.state === "COMPLETED"
    ? new Set(readLiveReport(artifact).cards.map(c => c.id)) : new Set<string>();

  return <section className="surface live-events" aria-label="Agent 执行轨迹">
    <div className="panel-head">
      <div><h2>Agent 执行轨迹</h2><p>Go 持久化阶段事件 · 按实际发生顺序</p></div>
      <span title={terminal ? "任务已结束，查看持久化事件" : stream === "connected" ? "SSE 已连接" : stream === "disconnected" ? "SSE 已断开，轮询补读" : "等待阶段事件"}>
        {terminal ? <Clock3 size={17} color="#a4b5c8"/> : stream === "connected" ?
          <Wifi size={17} color="#65e1c4"/> : stream === "disconnected" ?
          <WifiOff size={17} color="#e9b16c"/> : <Clock3 size={17} color="#71849b"/>}
      </span>
    </div>
    {!run ? <p className="live-empty">选择任务后查看阶段事件</p> : <>
      <div className="live-trace-summary">
        <span>任务状态 <strong>{run.state}</strong></span>
        <span>观测阶段 <strong>{trace?.stages.length ?? 0}</strong></span>
      </div>
      {run.state === "FAILED" && <div className="live-trace-diagnostic" role="status" aria-label="任务失败诊断">
        <strong>失败诊断 · 来自本次 Run</strong>
        <button type="button" className="btn" onClick={async () => {
          try {
            if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
            await navigator.clipboard.writeText(formatFailureDiagnostic(run, events));
            setCopyStatus("诊断报告已复制");
          } catch { setCopyStatus("复制失败，请检查浏览器剪贴板权限"); }
        }}><Copy size={14}/>复制诊断报告</button>
        {copyStatus && <p role="status">{copyStatus}</p>}
        <dl>
          <div><dt>错误代码</dt><dd><code>{run.failure_code ?? "后端未提供"}</code></dd></div>
          <div><dt>错误类别</dt><dd><code>{run.failure_category ?? "历史记录未提供"}</code></dd></div>
          <div><dt>故障说明</dt><dd>{run.failure_category ? (failureDescriptions[run.failure_category] ?? "已记录错误类别，暂无细分说明") : "历史任务没有保存具体错误分类"}</dd></div>
          <div><dt>失败阶段</dt><dd>{(run.failure_stage ?? trace?.explicitFailure) ? (stageLabels[run.failure_stage ?? trace!.explicitFailure!] ?? (run.failure_stage ?? trace!.explicitFailure!)) + "（" + (run.failure_stage ?? trace!.explicitFailure!) + "）" : "阶段事件未定位"}</dd></div>
          <div><dt>任务 ID</dt><dd><code>{run.run_id}</code></dd></div>
        </dl>
        <p>可展开下方原始事件，按 Go seq 和时间核对实际执行过程。类别为后端固定诊断标识，不是原始异常。缺少 FAIL 事件时不推断出错的 Agent；工具 DONE 不等于阶段成功。</p>
      </div>}
      {trace?.explicitFailure && <p role="status" className="live-trace-error">Go 阶段事件明确记录失败：{trace.explicitFailure}（{stageLabels[trace.explicitFailure] ?? "未命名阶段"}）</p>}
      {trace?.unlocatedFailure && <p role="status" className="live-trace-error">任务失败：{run.failure_code ?? "未提供失败代码"}。阶段事件未指明具体失败位置，不能推断哪个 Agent 出错。</p>}
      {run.state === "CANCELLED" && <p className="live-trace-note">任务已取消；未收到 DONE 的阶段不代表成功。</p>}
      {trace?.stages.length === 0 ?
        <p className="live-empty">尚未收到 Agent 阶段事件。未出现的阶段不能推定已执行、跳过或失败。</p> :
        <ol className="live-trace-stages" aria-label="已观测 Agent 阶段">
          {trace?.stages.map(step => {
            const reportId = stageReportId[step.stage];
            const hasReport = !!reportId && reportIds.has(reportId);
            return <li key={step.stage} className={"live-trace-stage trace-" + step.phase.toLowerCase()}>
              <div className="live-trace-stage-heading"><div><strong>{stageLabels[step.stage] ?? step.stage}</strong><small>{step.stage} · #{step.firstSeq}–#{step.lastSeq}</small></div>
                <span className="live-trace-phase">{phaseLabels[step.phase]}</span>
              </div>
              <dl className="live-trace-timing">
                {step.startedAt && <div><dt>开始</dt><dd>{formatTime(step.startedAt)}</dd></div>}
                {step.finishedAt && <div><dt>结束</dt><dd>{formatTime(step.finishedAt)}</dd></div>}
                {(step.phase === "DONE" || step.phase === "FAIL") && <div><dt>Go 记录耗时</dt><dd>{duration(step.durationMS)}</dd></div>}
              </dl>
              {step.phase === "INCOMPLETE" && <p className="live-trace-small">最后只记录了 START；终态不表示该阶段已经完成。</p>}
              {step.phase === "UNKNOWN" && <p className="live-trace-small">Go 原始状态：{step.rawStatus}，不能解释成成功。</p>}
              {events.some(ev => ev.kind === "TOOL" && ev.stage === step.stage) && <div className="live-trace-tools" aria-label={`${step.stage} 实际观测工具事件`}>
                <strong>已观测工具调用</strong>
                <ul>{events.filter(ev => ev.kind === "TOOL" && ev.stage === step.stage).map(ev =>
                  <li key={ev.seq}><code>{ev.tool ?? "未提供"}</code> · {ev.status === "START" ? "开始" : ev.status === "DONE" ? "完成" : ev.status === "FAIL" ? "失败" : "状态未知"}{ev.outcome ? " · " + ({SUCCEEDED: "采集成功", PARTIAL: "部分成功", TIMEOUT: "采集超时", FAILED: "采集失败"}[ev.outcome] ?? "结果未知") : ""} · #{ev.seq}{typeof ev.duration_ms === "number" ? ` · ${ev.duration_ms}ms` : ""}</li>
                )}</ul>
              </div>}
              {hasReport && <a className="live-trace-report-link" href={"#report-" + reportId}>查看本任务对应报告片段 ↗</a>}
            </li>;
          })}
        </ol>}
      {trace && trace.lifecycle.length > 0 && <div className="live-trace-lifecycle">
        <strong>Run 生命周期</strong>
        <div>{trace.lifecycle.map(ev => <span key={ev.seq}>{ev.status} <small>#{ev.seq}</small></span>)}</div>
      </div>}
      {events.length > 0 && <details className="live-trace-raw" open>
        <summary>原始事件记录 · {events.length} 条（Go seq）</summary>
        <div className="live-event-list">{events.map(ev =>
          <div key={ev.seq} className="live-event-item">
            <span className={"live-event-dot " + (ev.status === "DONE" || ev.status === "COMPLETED" ? "green" : ev.status === "FAIL" || ev.status === "FAILED" ? "red" : "")}/>
            <div><strong>{ev.stage}</strong><small>{ev.status} · {formatTime(ev.at)}{typeof ev.duration_ms === "number" ? " · " + ev.duration_ms + "ms" : ""}</small></div>
            <code>#{ev.seq}</code>
          </div>)}</div>
      </details>}
      <p className="live-trace-boundary">阶段信息来自 Go 持久化事件；仅展示实际观测到的工具事件。未接入 observer 的工具不代表未执行。数据源、工具参数与返回值均不展示；不会伪造调用记录或模型内部推理。</p>
    </>}
    <div className="live-stream-foot"><Activity size={13}/>
      {terminal ? "任务已结束 · 事件从持久化接口补读" : stream === "disconnected" ?
        "SSE 中断 · 自动轮询补读 / 尝试重连" : "SSE 实时订阅 · 定期补读已持久化事件"}
    </div>
  </section>;
}
