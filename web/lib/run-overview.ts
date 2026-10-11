import { isLiveRun, type LiveRun, type LiveState } from "./live-contract";

/** The Go ListRuns endpoint currently returns up to 100 recent records.
 * These counts are always sample counts, never global historical totals.
 */
export type OverviewFilter = "all" | "active" | "completed" | "failed" | "cancelled";
export type OverviewDay = { utcDate: string; count: number };
export interface RunOverview {
  runs: LiveRun[];
  total: number;
  active: number;
  completed: number;
  failed: number;
  cancelled: number;
  distinctSymbols: number;
  latestCompleted: LiveRun | null;
  recentFailures: LiveRun[];
  daily: OverviewDay[];
  capped: boolean;
}

export function summarizeRunOverview(input: unknown[], now: Date): RunOverview {
  const valid = input.filter(isLiveRun);
  // Dedupe direct-lookup records that the periodic list can reintroduce.
  const byID = new Map<string, LiveRun>();
  for (const r of valid) if (!byID.has(r.run_id)) byID.set(r.run_id, r);
  const runs = [...byID.values()].sort((a, b) =>
    Date.parse(b.created_at) - Date.parse(a.created_at) || a.run_id.localeCompare(b.run_id));
  const active = runs.filter(r => r.state === "PENDING" || r.state === "RUNNING").length;
  const completed = runs.filter(r => r.state === "COMPLETED").length;
  const failedRuns = runs.filter(r => r.state === "FAILED");
  const cancelled = runs.filter(r => r.state === "CANCELLED").length;
  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const days = Array.from({ length: 7 }, (_, index) => {
    const day = new Date(today.getTime() - (6 - index) * 86400000);
    return day.toISOString().slice(0, 10);
  });
  const counts = new Map(days.map(d => [d, 0]));
  for (const run of runs) {
    const day = new Date(run.created_at).toISOString().slice(0, 10);
    if (counts.has(day)) counts.set(day, (counts.get(day) ?? 0) + 1);
  }
  return {
    runs,
    total: runs.length, active, completed, failed: failedRuns.length, cancelled,
    distinctSymbols: new Set(runs.map(r => r.symbol)).size,
    latestCompleted: runs.find(r => r.state === "COMPLETED") ?? null,
    recentFailures: failedRuns.slice(0, 3),
    daily: days.map(day => ({ utcDate: day, count: counts.get(day) ?? 0 })),
    capped: input.length >= 100,
  };
}

export function filterOverviewRuns(runs: LiveRun[], filter: OverviewFilter, term: string): LiveRun[] {
  const q = term.trim().toUpperCase();
  return runs.filter(run =>
    (filter === "all" ||
     (filter === "active" && (run.state === "PENDING" || run.state === "RUNNING")) ||
     (filter === "completed" && run.state === "COMPLETED") ||
     (filter === "failed" && run.state === "FAILED") ||
     (filter === "cancelled" && run.state === "CANCELLED")) &&
    (!q || run.symbol.includes(q) || run.run_id.toUpperCase().includes(q))
  );
}

export function runStateLabel(state: LiveState): string {
  switch (state) {
    case "PENDING": return "等待执行";
    case "RUNNING": return "研究中";
    case "COMPLETED": return "已完成";
    case "FAILED": return "失败";
    case "CANCELLED": return "已取消";
  }
}
