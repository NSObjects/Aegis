/**
 * Read-only projection of Aegis Go JSON contracts. Never promotes an absent
 * value to zero, creates a citation, or infers a trading instruction.
 *
 * Go sources: synthesis.Artifact, FullPortfolioResult, PortfolioReviewResult,
 * DebateState, ResearchPlan, EvidenceLedger and decision.Decision.
 */
export interface ReportField { label: string; value: string }
export interface ReportEntry {
  label: string;
  body: string;
  details: string[];
}
export interface ReportCard {
  id: string;
  title: string;
  description: string;
  fields: ReportField[];
  entries: ReportEntry[];
}
export interface LiveReport {
  mode: "DECISION" | "PORTFOLIO_REVIEW" | "UNKNOWN";
  headline: string | null;
  overview: ReportField[];
  cards: ReportCard[];
}

type RecordValue = Record<string, unknown>;
const object = (v: unknown): RecordValue | null =>
  v !== null && typeof v === "object" && !Array.isArray(v) ? v as RecordValue : null;
const value = (v: unknown): string | null =>
  typeof v === "string" && v.trim() ? v : typeof v === "number" && Number.isFinite(v) ? String(v) : null;
const array = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const textItems = (v: unknown): string[] => array(v).map(value).filter((v): v is string => v !== null);

function field(fields: ReportField[], label: string, raw: unknown, unit = "") {
  const v = value(raw);
  if (v !== null) fields.push({ label, value: v + unit });
}
function entry(entries: ReportEntry[], label: string, body: unknown, details: string[] = []) {
  const v = value(body);
  if (v !== null) entries.push({ label, body: v, details });
}
function strings(entries: ReportEntry[], label: string, raw: unknown) {
  textItems(raw).forEach((v, i) => entry(entries, label + " " + (i + 1), v));
}
function references(raw: unknown): string[] {
  return array(raw).flatMap(ref => {
    const r = object(ref);
    if (!r) return [];
    const domain = value(r.domain);
    if (!domain) return [];
    const eventID = value(r.event_id);
    return [eventID ? domain + " · 事件 " + eventID : domain + " · 领域引用（无具体事件 ID）"];
  });
}
function card(cards: ReportCard[], id: string, title: string, description: string, source: RecordValue | null, fill: (fields: ReportField[], entries: ReportEntry[], s: RecordValue) => void) {
  if (!source) return;
  const fields: ReportField[] = [];
  const entries: ReportEntry[] = [];
  fill(fields, entries, source);
  if (fields.length || entries.length) cards.push({ id, title, description, fields, entries });
}
function claims(entries: ReportEntry[], prefix: string, raw: unknown, key: string) {
  array(raw).forEach((v, i) => {
    const item = object(v);
    if (item) entry(entries, prefix + " " + (i + 1), item[key], references(item.references));
  });
}

export function readLiveReport(input: unknown): LiveReport {
  const root = object(input);
  if (!root) return { mode: "UNKNOWN", headline: null, overview: [], cards: [] };
  const research = object(root.research_artifact);
  const review = object(root.portfolio_review);
  const synthesis = research && object(research.synthesis);
  const plan = review && object(review.research_plan);
  const intent = review && object(review.trade_intent);
  const risk = review && object(review.risk_context);
  const portfolio = review && object(review.portfolio_decision);
  const gate = object(root.final_risk_gate);
  const overview: ReportField[] = [];
  const cards: ReportCard[] = [];
  const mode = research ? "PORTFOLIO_REVIEW" : value(root.action) ? "DECISION" : "UNKNOWN";
  const headline = value(plan?.summary) ?? value(synthesis?.summary) ?? value(root.market_view);

  if (mode === "DECISION") {
    field(overview, "研究动作（非委托）", root.action);
    card(cards, "decision", "研究决策", "Decision 模式：仅供研究，不构成委托。", root, (f, e, s) => {
      field(f, "标的", s.symbol);
      field(f, "研究动作", s.action);
      entry(e, "市场观点", s.market_view);
      strings(e, "支持信息", s.evidence);
      strings(e, "风险", s.risk);
    });
    return { mode, headline, overview, cards };
  }
  if (!research) return { mode, headline, overview, cards };

  field(overview, "标的", research.symbol);
  field(overview, "研究结论", plan?.conclusion);
  field(overview, "TradeIntent（非指令）", intent?.status);
  field(overview, "最终风险门禁", gate?.disposition);
  const portfolioState = value(portfolio?.portfolio_state);
  if (portfolioState) field(overview, "账户状态", portfolioState === "NOT_PROVIDED" ? "NOT_PROVIDED · 未知" : portfolioState);

  card(cards, "synthesis", "研究综合", "多角色综合分析与已声明分歧；不会重写模型输出。", synthesis, (f, e, s) => {
    entry(e, "核心摘要", s.summary);
    claims(e, "一致观点", s.agreements, "claim");
    claims(e, "冲突观点", s.conflicts, "description");
    strings(e, "研究风险", s.risks);
  });
  card(cards, "market", "市场环境 · Market", "市场宽度与情绪等来自后端研究时点的上下文。", object(research.market), (f, e, s) => {
    field(f, "市场数据日", s.as_of);
    field(f, "市场状态", s.regime);
    field(f, "上涨家数", s.advance_count);
    field(f, "下跌家数", s.decline_count);
    field(f, "涨停家数", s.limit_up_count);
    field(f, "跌停家数", s.limit_down_count);
    field(f, "总成交额（元）", s.turnover);
    strings(e, "市场证据", s.evidence);
    strings(e, "风险提示", s.risk);
  });
  card(cards, "technical", "技术研究 · Technical", "价格和指标是该次研究的历史快照，并非当前实时行情。", object(research.technical), (f, e, s) => {
    field(f, "研究交易日", s.requested_as_of);
    field(f, "有效交易日", s.effective_as_of);
    field(f, "最后交易日", s.last_trade_date);
    field(f, "数据状态", s.status);
    const snapshot = object(s.snapshot);
    if (snapshot) {
      field(f, "开盘价", snapshot.open);
      field(f, "最高价", snapshot.high);
      field(f, "最低价", snapshot.low);
      field(f, "收盘价", snapshot.close);
      field(f, "前收盘价", snapshot.pre_close);
      field(f, "涨跌幅（%）", snapshot.change_percent);
      field(f, "成交量（源单位）", snapshot.volume);
      field(f, "成交额（源单位）", snapshot.amount);
    }
    const averages = object(s.moving_averages);
    if (averages) ["ma5", "ma10", "ma20", "ma60"].forEach(k => field(f, k.toUpperCase(), averages[k]));
    const momentum = object(s.momentum);
    if (momentum) {
      field(f, "RSI14", momentum.rsi14);
      const macd = object(momentum.macd);
      if (macd) ["dif", "dea", "hist"].forEach(k => field(f, "MACD " + k.toUpperCase(), macd[k]));
    }
    strings(e, "技术证据", s.evidence);
    strings(e, "技术风险", s.risk);
  });
  card(cards, "event", "事件研究 · Event", "事件事实与原有证据文档 ID；ID 本身不等于可点击来源。", object(research.event), (f, e, s) => {
    field(f, "事件状态", s.status);
    field(f, "研究截止", s.requested_as_of);
    field(f, "证据观察截止", s.evidence_as_of);
    array(s.events).forEach((raw, i) => {
      const event = object(raw);
      if (!event) return;
      const details = [
        value(event.id) ? "事件 ID · " + value(event.id) : null,
        value(event.type) ? "类型 · " + value(event.type) : null,
        value(event.occurred_at) ? "发生时间 · " + value(event.occurred_at) : null,
        value(event.source_count) !== null ? "来源数量 · " + value(event.source_count) : null,
        ...textItems(event.evidence_document_ids).map(id => "文档 ID · " + id),
      ].filter((x): x is string => x !== null);
      entry(e, value(event.headline) ?? "事件 " + (i + 1), event.fact_summary, details);
    });
    strings(e, "事件证据", s.evidence);
    strings(e, "事件风险", s.risk);
  });
  card(cards, "fundamental", "基本面 · Fundamental", "仅展示已公告且符合本次研究时点的源数据；不填补缺失财务数字。", object(research.fundamental), (f, e, s) => {
    field(f, "研究交易日", s.requested_as_of);
    const income = object(s.income);
    if (income) {
      field(f, "报告期末", income.period_end);
      field(f, "实际公告日", income.announcement_date);
      field(f, "数据来源", income.source);
      field(f, "营业收入（人民币元）", income.revenue);
      field(f, "净利润（人民币元）", income.net_income);
    }
    entry(e, "模型解释（非财务原始数据）", s.interpretation);
  });
  card(cards, "debate", "多空辩论 · Bull / Bear", "展示真实辩论回合与反驳目标 ID，不推断立场是否被接受。", review && object(review.debate_state), (f, e, s) => {
    array(s.turns).forEach((v, i) => {
      const turn = object(v);
      if (!turn) return;
      const label = "第 " + (value(turn.round) ?? String(i + 1)) + " 轮 · " + (value(turn.side) ?? "未知方");
      entry(e, label + " 观点", turn.thesis);
      array(turn.arguments).forEach(raw => {
        const argument = object(raw);
        if (argument) entry(e, label + " · 论点 " + (value(argument.id) ?? "（无 ID）"), argument.claim, references(argument.references));
      });
      array(turn.rebuttals).forEach(raw => {
        const rebuttal = object(raw);
        if (rebuttal) entry(e, label + " · 反驳 " + (value(rebuttal.target_argument_id) ?? "（目标未知）"), rebuttal.claim, references(rebuttal.references));
      });
    });
  });
  card(cards, "manager", "研究经理 · Research Manager", "结论与机会、风险均沿用 typed ResearchPlan。", plan, (f, e, s) => {
    field(f, "研究结论", s.conclusion);
    entry(e, "研究经理摘要", s.summary);
    claims(e, "机会", s.opportunities, "claim");
    claims(e, "风险", s.risks, "claim");
    strings(e, "研究约束", s.trading_constraints);
    const accepted = textItems(s.accepted_argument_ids);
    const unresolved = textItems(s.unresolved_argument_ids);
    if (accepted.length) entry(e, "已接受的论点 ID", accepted.join("、"));
    if (unresolved.length) entry(e, "尚未解决的论点 ID", unresolved.join("、"));
  });
  card(cards, "intent", "研究意向 · TradeIntent", "研究性候选/观察/排除；绝非交易指令。", intent, (f, e, s) => {
    field(f, "研究状态", s.status);
    entry(e, "研究依据", s.thesis);
    strings(e, "支持信息", s.evidence);
    strings(e, "风险", s.risk);
  });
  card(cards, "risk", "风险分析 · Risk Analyst", "独立风险评估；未解问题不能自动视为通过。", risk, (f, e, s) => {
    field(f, "风险判断", s.assessment);
    entry(e, "风险总结", s.summary);
    ["findings", "constraints", "unresolved"].forEach((key, index) => {
      const prefix = ["风险发现", "风险约束", "未解决问题"][index];
      array(s[key]).forEach((raw, i) => {
        const item = object(raw);
        if (item) entry(e, prefix + " " + (i + 1), item.claim);
      });
    });
  });
  card(cards, "portfolio", "组合审查与最终风险门禁", "已有确定性审查，不是动态仓位系统，也没有委托权限。", portfolio ?? gate, (f, e) => {
    if (portfolio) {
      field(f, "组合决策", portfolio.disposition);
      field(f, "原因代码", portfolio.reason_code);
      field(f, "账户状态", portfolioState === "NOT_PROVIDED" ? "NOT_PROVIDED · 未知（不能解释为零持仓）" : portfolioState);
      field(f, "风险等级", portfolio.risk_assessment);
      entry(e, "组合审查摘要", portfolio.summary);
    }
    if (gate) {
      field(f, "最终门禁", gate.disposition);
      field(f, "门禁原因", gate.reason);
      field(f, "当前风险规则状态", gate.current_limits_status);
    }
    if (!object(review?.observed_exposure)) entry(e, "已观测敞口", "未提供可验证账户估值数据；不能推导现金、仓位或零敞口。");
  });
  card(cards, "ledger", "论点与证据索引 · EvidenceLedger", "仅显示后端明确提供的论点、引用域、事件 ID 与反驳关系。", review && object(review.evidence_ledger), (f, e, s) => {
    field(f, "归档结论", s.conclusion);
    array(s.arguments).forEach(raw => {
      const argument = object(raw);
      if (!argument) return;
      const details = [
        value(argument.side) ? "阵营 · " + value(argument.side) : null,
        value(argument.round) !== null ? "回合 · " + value(argument.round) : null,
        value(argument.disposition) ? "经理分类 · " + value(argument.disposition) : null,
        ...references(argument.references),
      ].filter((x): x is string => x !== null);
      entry(e, "论点 " + (value(argument.id) ?? "（无 ID）"), argument.claim, details);
      array(argument.challenges).forEach(rawChallenge => {
        const challenge = object(rawChallenge);
        if (challenge) entry(e, "针对 " + (value(argument.id) ?? "（无 ID）") + " 的反驳", challenge.claim, references(challenge.references));
      });
    });
    array(s.insights).forEach((raw, i) => {
      const insight = object(raw);
      if (!insight) return;
      entry(e, "研究经理 " + (value(insight.kind) ?? "INSIGHT") + " " + (i + 1), insight.claim, [
        ...textItems(insight.argument_ids).map(id => "关联论点 ID · " + id),
        ...references(insight.references),
      ]);
    });
  });

  return { mode, headline, overview, cards };
}
