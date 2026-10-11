import type { LiveReport, ReportCard, ReportEntry, ReportField } from "./live-report";

/** Presentation-only labels: retain the original server enum for auditing. */
const meanings: Record<string, string> = {
  INSUFFICIENT_EVIDENCE: "证据不足",
  NOT_PROVIDED: "未提供（未知）",
  NO_ACTION: "未形成执行动作",
  REVIEW_REQUIRED: "需要复核",
  APPROVED: "已通过门禁",
  REJECTED: "未通过门禁",
  BLOCKED: "门禁阻止",
  WATCH: "持续观察",
  MONITOR: "继续跟踪",
  INSUFFICIENT_CONTEXT: "研究上下文不足",
  AVOID: "暂不参与",
  CANDIDATE: "候选研究对象",
  RISK_OFF: "防御环境",
  RISK_ON: "积极环境",
  MIXED: "信号分化",
  NORMAL: "正常",
  NO_DATA: "未取得数据",
  NO_EVENT: "未发现已入档事件",
};
export const readStatus = (raw: string): string => meanings[raw] ? meanings[raw] + " · " + raw : raw;

export interface ReadingPoint {
  label: string;
  text: string;
  /** Card section from the exact same stored artifact, never a citation URL. */
  sectionId: string;
  meta: string[];
}
export interface ReadingDigest {
  headline: string | null;
  status: Array<{ label: string; raw: string; display: string }>;
  supporting: ReadingPoint[];
  conflicting: ReadingPoint[];
  risks: ReadingPoint[];
  eventCount: number | null;
  omittedSupporting: number;
  omittedConflicting: number;
  omittedRisks: number;
}
const pick = (report: LiveReport, id: string): ReportCard | undefined => report.cards.find(c => c.id === id);
const getField = (fields: ReportField[], label: string): string | null => fields.find(f => f.label === label)?.value ?? null;
const entries = (report: LiveReport, id: string, starts: string[]): ReadingPoint[] => {
  const card = pick(report, id);
  if (!card) return [];
  return card.entries.filter(entry => starts.some(prefix => entry.label.startsWith(prefix))).map(entry => ({
    label: entry.label, text: entry.body, sectionId: id, meta: entry.details,
  }));
};
const distinct = (points: ReadingPoint[]): ReadingPoint[] => {
  const seen = new Set<string>();
  return points.filter(p => {
    const key = p.sectionId + ":" + p.label + ":" + p.text;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
};

export function buildReadingDigest(report: LiveReport, maxPoints = 3): ReadingDigest {
  const status = report.overview
    .filter(item => ["研究动作（非委托）", "研究结论", "TradeIntent（非指令）", "最终风险门禁", "账户状态"].includes(item.label))
    .map(item => ({ label: item.label, raw: item.value, display: readStatus(item.value) }));
  const supportingFull = distinct([
    ...entries(report, "manager", ["机会"]),
    ...entries(report, "synthesis", ["一致观点"]),
    ...entries(report, "decision", ["支持信息"]),
  ]);
  const conflictingFull = distinct([
    ...entries(report, "synthesis", ["冲突观点"]),
    ...entries(report, "manager", ["尚未解决的论点 ID"]),
  ]);
  const riskFull = distinct([
    ...entries(report, "manager", ["风险 ", "研究约束"]),
    ...entries(report, "risk", ["风险总结", "风险发现", "风险约束", "未解决问题"]),
    ...entries(report, "synthesis", ["研究风险"]),
    ...entries(report, "decision", ["风险"]),
  ]);
  // An absent event card/list is not "zero events"; an explicit NO_EVENT status
  // is also not proof that the real world had no news.
  const eventCard = pick(report, "event");
  const eventCount = eventCard?.entries.filter(entry => !["事件证据", "事件风险"].some(p => entry.label.startsWith(p))).length ?? null;
  const take = (points: ReadingPoint[]) => points.slice(0, Math.max(0, maxPoints));
  return {
    headline: report.headline,
    status, supporting: take(supportingFull), conflicting: take(conflictingFull), risks: take(riskFull),
    omittedSupporting: supportingFull.length - take(supportingFull).length,
    omittedConflicting: conflictingFull.length - take(conflictingFull).length,
    omittedRisks: riskFull.length - take(riskFull).length,
    eventCount: eventCard ? eventCount : null,
  };
}

export type ReadingGroup = "overview" | "context" | "analysis" | "audit";
export const groups: Array<{id: ReadingGroup; title: string; intro: string; cardIds: string[]}> = [
  { id: "overview", title: "结论与风险说明", intro: "研究经理、意向及风控的原始解释，不是下单建议。", cardIds: ["synthesis", "manager", "intent", "risk", "portfolio", "decision"] },
  { id: "context", title: "研究所依据的数据", intro: "按本次运行的数据时点检查市场、技术、事件及基本面。", cardIds: ["market", "technical", "event", "fundamental"] },
  { id: "analysis", title: "Agent 观点和辩论", intro: "展开查看多空论点、反驳及争议，不把观点当作客观事实。", cardIds: ["debate"] },
  { id: "audit", title: "证据与详细审计", intro: "定位 EvidenceLedger 结构化引用和完整原始记录。", cardIds: ["ledger"] },
];

/** Detail chapter counts are based on available typed fields/entries only. */
export function chapterCaption(card: ReportCard): string {
  const count = card.entries.length;
  if (count > 0) return count + " 条原始内容";
  return card.fields.length + " 项已记录字段";
}
