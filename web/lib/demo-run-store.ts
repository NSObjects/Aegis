/**
 * Mock-only task state. The Go API will own Run state in production.
 * This module never accesses market data or calls an Agent.
 */
import { demoHistory, demoStocks, stages, type StockId } from "./mock-research";
import type { ResearchStatus } from "./research-contract";

const STORAGE_KEY = "aegis.demo.research-runs.v1";
const MAX_RUNS = 50;
const symbolSet = new Set<string>(demoStocks.map((item) => item.symbol));
const statuses: ReadonlySet<string> = new Set(["PENDING", "RUNNING", "COMPLETED", "FAILED", "CANCELLED"]);

export type DemoScenario = "success" | "provider_failure";

export interface DemoRun {
  id: string;
  symbol: StockId;
  createdAt: string;
  status: ResearchStatus;
  completedStages: number;
  scenario: DemoScenario;
  failureStage?: string;
  failureReason?: string;
  headline?: string;
  duration?: string;
}

export const initialDemoRuns: DemoRun[] = demoHistory.map((item) => ({
  id: item.id,
  symbol: item.symbol,
  createdAt: `${item.when.replace(" ", "T")}:00+08:00`,
  status: item.state as ResearchStatus,
  completedStages: item.state === "COMPLETED" ? stages.length : 2,
  scenario: item.state === "FAILED" ? "provider_failure" : "success",
  failureStage: item.state === "FAILED" ? "event" : undefined,
  failureReason: item.state === "FAILED" ? "DEMO_PROVIDER_UNAVAILABLE" : undefined,
  headline: item.headline,
  duration: item.duration,
}));

export function createDemoRun(symbol: StockId, scenario: DemoScenario, at = new Date()): DemoRun {
  return {
    id: `DEMO-${at.getTime()}-${crypto.randomUUID().slice(0, 8)}`,
    symbol,
    createdAt: at.toISOString(),
    status: "RUNNING",
    completedStages: 0,
    scenario,
  };
}

/** One deterministic mock stage tick; no network calls, no wall-clock dependency. */
export function advanceDemoRun(run: DemoRun): DemoRun {
  if (run.status !== "RUNNING") return run;
  if (run.scenario === "provider_failure" && run.completedStages === 2) {
    return {
      ...run,
      status: "FAILED",
      failureStage: "event",
      failureReason: "DEMO_PROVIDER_UNAVAILABLE · Event Analyst 演示来源失败，后续阶段未执行",
    };
  }
  const completedStages = Math.min(stages.length, run.completedStages + 1);
  return {
    ...run,
    completedStages,
    status: completedStages === stages.length ? "COMPLETED" : "RUNNING",
    headline: completedStages === stages.length ? "本地演示研究完成 · 无真实行情与模型请求" : undefined,
  };
}

export function cancelDemoRun(run: DemoRun): DemoRun {
  return run.status === "RUNNING" || run.status === "PENDING"
    ? { ...run, status: "CANCELLED", failureReason: "USER_CANCELLED · 已终止本地演示" }
    : run;
}

/** Browser refresh cannot resume a simulated timer: fail closed and keep the trace. */
export function restoreDemoRuns(serialized: string | null): DemoRun[] {
  if (!serialized) return initialDemoRuns;
  try {
    const records: unknown = JSON.parse(serialized);
    if (!Array.isArray(records)) return initialDemoRuns;
    const valid = records.filter((value): value is DemoRun => {
      if (!value || typeof value !== "object") return false;
      const entry = value as Partial<DemoRun>;
      return typeof entry.id === "string" && entry.id.startsWith("DEMO-") &&
        typeof entry.symbol === "string" && symbolSet.has(entry.symbol) &&
        typeof entry.createdAt === "string" && !Number.isNaN(Date.parse(entry.createdAt)) &&
        typeof entry.status === "string" && statuses.has(entry.status) &&
        Number.isInteger(entry.completedStages) &&
        entry.completedStages! >= 0 && entry.completedStages! <= stages.length &&
        (entry.scenario === "success" || entry.scenario === "provider_failure");
    });
    if (valid.length === 0) return initialDemoRuns;
    const sanitized = valid.slice(0, MAX_RUNS).map((run) => run.status === "RUNNING" || run.status === "PENDING"
      ? {
          ...run,
          status: "FAILED" as const,
          failureStage: stages[Math.min(run.completedStages, stages.length - 1)].id,
          failureReason: "DEMO_SESSION_INTERRUPTED · 页面刷新中断了本地模拟，未执行真实研究",
        }
      : run);
    return sanitized;
  } catch {
    return initialDemoRuns;
  }
}

export function formatDemoRunTime(run: DemoRun): string {
  return run.createdAt.replace("T", " ").slice(0, 16);
}

export function describeDemoRun(run: DemoRun): string {
  if (run.status === "RUNNING") return "演示运行中";
  if (run.status === "FAILED") return "演示失败";
  if (run.status === "CANCELLED") return "已取消";
  if (run.status === "PENDING") return "等待执行";
  return "演示完成";
}

export function keepRecentDemoRuns(runs: DemoRun[]): DemoRun[] {
  return runs.slice(0, MAX_RUNS);
}

export const demoStorageKey = STORAGE_KEY;
