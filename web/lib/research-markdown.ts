import { readLiveReport } from "./live-report";

type RunForExport = { run_id: string; symbol: string; requested_as_of: string; mode: string };
const escapeCell = (value: string) => value.replace(/\|/g, "\\|").replace(/\r?\n/g, " ");
const clean = (value: string) => value.replace(/[\u0000-\u001f\u007f]/g, " ").trim();
const safeId = (value: string) => /^[a-f0-9]{32}$/.test(value) ? value : "未提供";

export function renderResearchMarkdown(artifact: unknown, run: RunForExport): string {
  const report = readLiveReport(artifact);
  const lines = [
    "# Aegis 研究报告",
    "",
    "> 研究辅助材料，不构成交易指令或投资建议；不包含实时交易能力。",
    "",
    "| 字段 | 内容 |",
    "| --- | --- |",
    `| 标的 | ${escapeCell(clean(run.symbol))} |`,
    `| Run ID | ${safeId(run.run_id)} |`,
    `| 研究截止时间 (PIT) | ${escapeCell(clean(run.requested_as_of))} |`,
    `| 研究模式 | ${escapeCell(report.mode)} |`,
    "",
    "## 研究结论",
    "",
    report.headline ? clean(report.headline) : "未提供",
    "",
    "## 主要结论与状态",
    "",
  ];
  if (report.overview.length === 0) lines.push("未提供", "");
  else {
    lines.push("| 字段 | 值 |", "| --- | --- |");
    for (const field of report.overview) lines.push(`| ${escapeCell(clean(field.label))} | ${escapeCell(clean(field.value))} |`);
    lines.push("");
  }
  for (const card of report.cards) {
    lines.push(`## ${clean(card.title)}`, "", clean(card.description), "");
    if (card.fields.length) {
      lines.push("| 字段 | 值 |", "| --- | --- |");
      for (const field of card.fields) lines.push(`| ${escapeCell(clean(field.label))} | ${escapeCell(clean(field.value))} |`);
      lines.push("");
    }
    for (const entry of card.entries) {
      lines.push(`### ${clean(entry.label)}`, "", clean(entry.body), "");
      if (entry.details.length) lines.push("后端声明的引用/元数据：", ...entry.details.map(item => `- ${clean(item)}`), "");
    }
  }
  lines.push("## 数据与引用限制", "", "仅导出同一 Run 的可读投影。ID 不是可独立验证的来源链接；缺失数据未补为零，模型观点并非已核实事实。", "");
  return lines.join("\n");
}
