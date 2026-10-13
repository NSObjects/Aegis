import type { LiveReport, ReportCard, ReportField } from "./live-report";

/** Presentation-only projection. All text remains directly traceable to a
 * field or entry in the same Go Run. No LLM rewriting or inferred advice. */
export type BriefBlock = { title: string; lines: string[]; source: string };
export type ResearchBrief = {
  summary: string | null;
  conclusion: string | null;
  marketView: string | null;
  action: string | null;
  confidence: string | null;
  posture: string | null;
  market: ReportField[];
  technical: ReportField[];
  evidence: BriefBlock[];
  risks: BriefBlock[];
  unknowns: string[];
};

const card = (report: LiveReport, id: string): ReportCard | undefined => report.cards.find(c => c.id === id);
const match = (fields: ReportField[], name: string): string | null => fields.find(f => f.label === name)?.value ?? null;
function gather(c: ReportCard | undefined, names: string[], title: string): BriefBlock | null {
  if (!c) return null;
  const lines = c.entries.filter(e => names.some(name => e.label.includes(name)))
    .map(e => e.body).filter(Boolean).slice(0, 5);
  return lines.length ? { title, lines, source: c.title } : null;
}
export function readResearchBrief(report: LiveReport): ResearchBrief {
  const overview = report.overview;
  const manager = card(report, "manager");
  const synthesis = card(report, "synthesis");
  const decision = card(report, "decision");
  const risk = card(report, "risk");
  const intent = card(report, "intent");
  const events = card(report, "event");
  const market = card(report, "market");
  const technical = card(report, "technical");
  const portfolio = card(report, "portfolio");
  const conclusion = match(overview, "研究结论") ?? match(overview, "研究动作（非委托）");
  const marketView = match(overview, "市场观点") ?? match(market?.fields ?? [], "市场状态");
  const action = match(overview, "研究动作（非委托）") ?? match(overview, "研究意向（非指令）");
  const confidence = match(overview, "置信度");
  const posture = match(overview, "最终风险门禁") ?? match(portfolio?.fields ?? [], "组合决策");
  const evidence = [
    gather(manager, ["机会"], "潜在机会（原始研究观点）"),
    gather(synthesis, ["一致观点"], "已声明的一致观点"),
    gather(events, ["事件证据"], "事件研究证据"),
    gather(decision, ["支持信息"], "研究依据"),
  ].filter((v): v is BriefBlock => v !== null);
  const risks = [
    gather(manager, ["风险"], "研究经理风险"),
    gather(synthesis, ["研究风险", "冲突观点"], "分歧与综合风险"),
    gather(risk, ["风险发现", "未解决问题"], "独立风险分析"),
    gather(intent, ["风险"], "研究意向风险"),
    gather(decision, ["风险"], "研究风险"),
  ].filter((v): v is BriefBlock => v !== null);
  const unknowns: string[] = [];
  if (match(overview, "账户状态")?.startsWith("NOT_PROVIDED"))
    unknowns.push("账户状态未提供，不能推断现金、持仓或实际风险敞口。");
  if (!conclusion) unknowns.push("本次工件没有可提取的研究结论。");
  if (!posture && report.mode === "PORTFOLIO_REVIEW") unknowns.push("未提供最终风险门禁结果。");
  if (!market) unknowns.push("未提供可阅读的市场背景。");
  if (!technical || match(technical.fields, "数据状态") === "NO_DATA")
    unknowns.push("没有可用的技术行情快照，不能由报告推断当前价格走势。");
  if (!evidence.length) unknowns.push("未在摘要层发现明确的支持论点；请查看下方完整证据和原始数据。");
  return {
    summary: report.headline,
    conclusion, marketView, action, confidence, posture,
    market: market?.fields.filter(f => ["市场数据日","市场状态","上涨家数","下跌家数"].includes(f.label)) ?? [],
    technical: technical?.fields.filter(f => ["有效交易日","数据状态","收盘价","涨跌幅（%）","RSI14"].includes(f.label)) ?? [],
    evidence, risks, unknowns,
  };
}

/** UI-only gloss; always keep the untouched backend enum visible in brackets. */
const enumGloss: Record<string,string> = {
  BALANCED:"多空观点相对均衡", INSUFFICIENT_EVIDENCE:"研究证据不足",
  BULL_LEAN:"研究偏多", BEAR_LEAN:"研究偏空", CANDIDATE:"进入候选观察名单",
  REJECTED:"研究意向已否决", BLOCKED:"风险门禁阻止",
  CAUTION:"需要谨慎", HIGH_RISK:"高风险", ACCEPTABLE:"风险审查未识别重大问题",
  BULLISH:"偏多", BEARISH:"偏空", NEUTRAL:"中性",
  WATCH:"继续观察", MONITOR:"持续监测", AVOID:"暂不参与",
  REVIEW_REQUIRED:"需要人工复核", NO_ACTION:"不采取行动",
  NOT_PROVIDED:"未提供", INSUFFICIENT_CONTEXT:"上下文不足",
  NORMAL:"数据可用", NO_DATA:"没有有效数据", NO_EVENT:"未记录事件",
  RISK_OFF:"风险偏好较低", RISK_ON:"风险偏好较高", MIXED:"市场分化",
};
/** Never silently translate unknown enums into a purported investment call. */
export function readableResearchCode(value: string | null): string {
  if (value === null) return "未提供";
  return enumGloss[value] ? enumGloss[value] + "（" + value + "）" : value;
}
