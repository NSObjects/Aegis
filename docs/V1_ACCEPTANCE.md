# Aegis V1 Multi-Agent 研究内核验收记录

> 记录日期：2026-10-10。验收代码基线：`main` 合并提交 `54441b543e91294f765ae278a84cb13d9ef9cdcb`（PR #129）。
> **当前状态：IN_REVIEW**。合并前同一实现的 Hosted Integration 已成功，尚不能以此替代合并后 main 的独立 Integration 运行及最终 CI 结果。

## 已有直接证据

| 证据 | 固定提交 | 结论与范围 |
| --- | --- | --- |
| [PR #129 Integration #38045258418](https://github.com/NSObjects/Aegis/actions/runs/38045258418) | `868f5de6d8ba850ff799582adf59f987b846e100` | SUCCESS：Production CLI Portfolio Review、Fundamental Production CLI 和 Invalid Account Fail-Closed 三个 Job |
| [PR #129 CI #38039296293](https://github.com/NSObjects/Aegis/actions/runs/38039296293) | `868f5de6d8ba850ff799582adf59f987b846e100` | SUCCESS：Go test/vet 与 CLI 输入门禁 |
| [main CI #38045567317](https://github.com/NSObjects/Aegis/actions/runs/38045567317) | `54441b543e91294f765ae278a84cb13d9ef9cdcb` | 检查时 **IN_PROGRESS**；必须更新最终结论 |
| [main Integration #38038984852](https://github.com/NSObjects/Aegis/actions/runs/38038984852) | `63db14aface62d4ab9d4dc11f3c24d1c26c8c535` | FAILURE：Fundamental Job Event 引用原文校验未通过；该历史失败促成 PR #129，不能当作当前通过证据 |

**未证明的范围**：上述最新成功 Integration 中，Trading、Technical、Full Stateless、Full Multi-Agent 和 Full Risk/Portfolio *独立 Hosted Job* 均 SKIPPED；三个成功生产 Job 不能冒充这些专项已独立验证。没有证据表明已接入真实券商账户、自动交易、动态仓位或所有外部服务故障模式。

## V1 支持范围

一次正式 CLI 调用执行 Market、Technical、Event、可选 Fundamental → Research Synthesis → 四轮 Bull/Bear → Research Manager → TradeIntent → RiskContext → PortfolioDecision → FinalRiskGate。研究结果输出为可机器解析 JSON，Agent 阶段开始/结束、失败与耗时输出至 stderr。

外部来源包括已配置的 DeepSeek 模型、TuShare HTTP 数据及可用的公告/新闻检索来源。来源成功、文档证据和研究截止时点均依当次真实返回判断，不使用伪造的行情或持仓填充缺失值。

## 复现入口

在仓库根目录配置 `configs/config.yaml`、有效 DeepSeek/TuShare 环境凭据和可用的 `skills/` 目录；Event Web Search 来源可按实际配置使用对应凭据。**不要将密钥写入仓库或日志**。

```bash
printf '{"state":"NOT_PROVIDED"}\n' > /tmp/aegis-portfolio.json

go test ./... -count=1
go vet ./...

go run ./cmd/aegis \
  -config configs/config.yaml \
  -skills skills \
  -symbol 600519.SH \
  -portfolio-review \
  -portfolio-file /tmp/aegis-portfolio.json \
  -evidence-store .aegis/evidence \
  > /tmp/aegis-research.json

jq -e '.research_artifact.symbol == "600519.SH" and
  .portfolio_review.portfolio_decision.symbol == "600519.SH" and
  .final_risk_gate.disposition == .portfolio_review.portfolio_decision.disposition' \
  /tmp/aegis-research.json
```

启用真实 Fundamental Analyst 时在同一命令追加 `-fundamental`；要求 TuShare income 数据可用。CI 托管执行入口：[Integration workflow](https://github.com/NSObjects/Aegis/actions/workflows/integration.yml)，选中 `main`，勾选 `run_production_cli` 和 `run_fundamental`。若需单独验收其他 Hosted Job，必须额外开启各自 workflow_dispatch 选项并逐一记录执行结果。

最终 JSON 应包含 `research_artifact`、`portfolio_review.debate_state`、`research_plan`、`trade_intent`、`risk_context`、`portfolio_decision` 和 `final_risk_gate`；**字段的最终约束以代码和 workflow jq 检查为准**。以上是命令和结构示例，并非捏造的一份真实投资报告。

## 失败与不足语义

- `NOT_PROVIDED` 表示调用方未提供账户快照，不等于 EMPTY 或 0% 仓位；因此不能臆造持仓、NAV 和风险预算。
- 非法账户文件由 CLI 在外部研究启动前拒绝；真实源/LLM 失败不能转换成看似成功的研究结果。
- Event 事实引用必须指向规范化文档中**唯一的原文片段**；无效引用可以请求模型基于已知原文纠错，但不能直接放宽引用校验。
- Market JSON / Debate 生成异常有受约束的纠错路径；重试耗尽必须失败，不能静默补造数据。
- FinalRiskGate 和 PortfolioDecision 是**非执行**的研究性判定，非投资订单或审批。未经提供的限制、交易情景和账户事实不会自动生成。

## V1 Exit Gate / 待收口事项

- [x] 主要功能路径具备可执行正式 CLI；PR #129 相同实现分支完成普通、Fundamental 与非法账户真实 Hosted 验证。
- [x] 历史 Event Grounding 引用失败已定位并修复，且分支真实 Hosted 复验成功。
- [ ] 核实合并提交 `54441b54` 的 main CI 最终为 SUCCESS，并更新本文件证据。
- [ ] 在**该 main 提交**运行真实 Integration（至少普通生产 CLI + Fundamental + 非法账户），记录运行 URL、Job 结论。
- [ ] 若全绿，将 `docs/DEVELOPMENT_ROADMAP.md` 的 R0-01～R0-03 和 R0 状态标为 DONE，并更新唯一「下一项」为 R1-01；此前不得宣称最新 main 完成整体验收。

R0 以外的 Web 工作台、Memory、策略研究、动态仓位属于后续路线，不作为此验收已有能力。 
