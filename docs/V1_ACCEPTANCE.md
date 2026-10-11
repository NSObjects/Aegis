# V1 Multi-Agent 研究内核 — R0 验收记录

> **验收日期：2026-10-11。** V1 指当前可运行的多 Agent 股票研究闭环，并非所有规划角色或 Web 工作台均已完成。
>
> **代码合并基线：** `main` merge commit `b6ab9b688450c8f62f40483e0b5ccd870a3614ca`，来自已合并 [PR #142](https://github.com/NSObjects/Aegis/pull/142)。真实八项 Hosted 验收运行于该 PR 的固定 head `61de58d0c0812de5d98c35e481491355debdff4f`，不是 merge commit 本身。合并前后是否完全相同、后续新提交是否回归，均须以 SHA 区分。

## 真实验收证据

- **[全量 Integration #38098687921](https://github.com/NSObjects/Aegis/actions/runs/38098687921)：SUCCESS，8/8 Job 实际执行，固定 `61de58d0`。** Trading Agent、Technical Agent、Full Stateless Hosted E2E、Full Multi-Agent Hosted E2E、Full Risk / Portfolio Hosted E2E、Fundamental Production CLI E2E、Production CLI Portfolio Review E2E、Production CLI Invalid Account Fail-Closed 全部成功。
- **[PR Head CI #38098690842](https://github.com/NSObjects/Aegis/actions/runs/38098690842)：SUCCESS，固定 `61de58d0`。** Go vet、unit tests、输入校验与本地 HTTP smoke。
- **合并后 main CI：** [#38098968618](https://github.com/NSObjects/Aegis/actions/runs/38098968618)，固定 `b6ab9b68`。最终状态应按 Actions 页面核对，不能用合并前的运行冒充。
- 历史失败 [#38060141143](https://github.com/NSObjects/Aegis/actions/runs/38060141143)（Event 重复原文引用）和 [#38055096507](https://github.com/NSObjects/Aegis/actions/runs/38055096507)（Research Manager 引用 Debate 不存在 ID、Risk JSON Mode 元数据）均发生于旧提交，已通过 PR #142 的代码变更和后续全量复验。
- 2026-10-11 同分支的 #38098333390 / #38098353853 是 `push` 自动触发的 **8/8 SKIPPED**，**不构成任何 Hosted 通过证据**。仅以完整实际执行的 #38098687921 计算验收。

## 运行入口和可复现命令

环境需要配置 `configs/config.yaml`、`skills/` 与真实外部 DeepSeek / TuShare / Web Search 所需凭据。凭据由环境变量或私有配置提供，**不可写入仓库、工作流日志或对浏览器暴露**。以当前仓库 CLI 参数为准：

```bash
# 研究型账户状态：未知；它不等于已观察到空仓。
printf '{"state":"NOT_PROVIDED"}\n' > /tmp/aegis-portfolio.json

go vet ./...
go test ./... -count=1

go run ./cmd/aegis \
  -config configs/config.yaml \
  -skills skills \
  -symbol 600519.SH \
  -portfolio-review \
  -portfolio-file /tmp/aegis-portfolio.json \
  -evidence-store .aegis/evidence \
  > /tmp/aegis-research.json

# 需要可用的 TuShare income 权限；可选启用 Fundamental。
# 在上述 go run 参数追加 -fundamental。
jq -e '
  .research_artifact.symbol == "600519.SH" and
  .portfolio_review.trade_intent.symbol == "600519.SH" and
  .portfolio_review.risk_context.symbol == "600519.SH" and
  .portfolio_review.portfolio_decision.symbol == "600519.SH" and
  .final_risk_gate.disposition == .portfolio_review.portfolio_decision.disposition and
  .portfolio_review.risk_context.assessment == "INSUFFICIENT_CONTEXT" and
  .portfolio_review.portfolio_decision.portfolio_state == "NOT_PROVIDED"
' /tmp/aegis-research.json
```

正式云端运行在 [Integration workflow](https://github.com/NSObjects/Aegis/actions/workflows/integration.yml) 选择目标分支，并按所需勾选 `run_trading`、`run_technical`、`run_research`、`run_multi_agent`、`run_risk_portfolio`、`run_fundamental`、`run_production_cli`。分支 `test/v1-integrated-acceptance` 上只有 **commit message 以 `[v1-gate]` 开头的 push** 会自动执行全部付费 Jobs；普通 push 全跳过，不能代替验收。避免重复并发触发同分支工作流。

## 可公开核验的真实结果摘录

`#38098687921` 的两个生产 CLI Job **实际输出的脱敏最终状态摘要**均为：

```json
{"symbol":"600519.SH","intent":"MONITOR","risk":"INSUFFICIENT_CONTEXT","disposition":"NO_ACTION","portfolio_state":"NOT_PROVIDED"}
```

这 5 个字段是 Actions 日志中真实展示的结果；**不是完整研究 JSON**，完整输出被保存于 runner 临时文件，当前工作流并未公开上传完整 Artifact。不得用手写示例冒充真实完整报告。

该 Hosted Workflow 的 `jq` 契约检查进一步确认：`research_artifact` 与 `portfolio_review.trade_intent/risk_context/portfolio_decision` 标的一致；Bull/Bear 交替四轮，后续回合包含对对立论点的实际回应；`final_risk_gate` 与 PortfolioDecision 一致；不含下单 `order` 对象；Fundamental Job 有 `fundamental.income.source=TUSHARE_INCOME` 且解释非空。

## 失败与不足场景

- `NOT_PROVIDED`：调用者未提供账户事实，风险评估保持 `INSUFFICIENT_CONTEXT`；不得从未知账户推断 0% 仓位、现金、敞口或可买额度。
- 非法账户文件必须在外部模型和数据调用前失败，不能输出伪成功结果；该路径独立 Hosted Job 已通过。
- **Event**：证据引文必须精确落于 CanonicalDocument；重复引文可能触发基于唯一原文上下文的纠错。有限重试后仍存在歧义时，可以保守移除无法唯一定位的引文；保留的来源仍严格验证。如果所有事实失去有效 grounding 则硬失败，不能伪装为 `NO_EVENT`。
- **Synthesis**：无效的单领域「跨领域冲突」在纠错耗尽后只能保守省略并披露局限；不能补造其他研究领域证据。
- **Research Manager**：只有实际存在于 DebateState 的 Argument ID 可以做关联；纠错失败后只能移除无效的可选关联并标记溯源缺口。实质证据引用和结构校验仍必须通过。
- **Fundamental**：仅在首次模型解释为空且无 Provider 错误时重试一次；两次为空明确失败。输入的财报数值仍由 TuShare 来源持有。
- **Risk**：仅允许模型 JSON Mode 的精确 `type=json_object` 包装元数据，其余多余字段、非法账户状态或未经观察的资金事实继续拒绝。
- 上述受限回退保留运行完成的可能，但会造成信息删减或信心降低，**不能等同于模型事实的语义正确性已经通过全量独立人工核查**。

## 能力边界与下一阶段

当前 V1 已具有 Market/Technical/Event（可选 Fundamental）、Research Synthesis、Bull/Bear Debate、Research Manager、TradeIntent、Risk Analyst、Portfolio Manager、FinalRiskGate 的**非交易执行**研究闭环。该验收证明给定合法外部配置时可运行及 fail-closed，不保证每个标的、每次模型调用或每一种数据源异常永不失败。不包括完整自动仓位管理、自动交易、独立 Sentiment、长期 Memory/RAG 或 Web 工作台全功能。

详细的 F0～F4 基线在 [MULTI_AGENT_ROADMAP.md](MULTI_AGENT_ROADMAP.md)，统一下一任务从 [DEVELOPMENT_ROADMAP.md](DEVELOPMENT_ROADMAP.md) 顶部读取。完成 R0 后即进入 R1 研究工作台，首先复核已有 `internal/application` 服务代码，避免重复开发。
