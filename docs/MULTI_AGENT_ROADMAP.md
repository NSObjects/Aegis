# Aegis Multi-Agent 功能优先路线图

> 2026-10-10 基线：main commit `55967260b5bc0c5853256b496ee56a922c8941a9`（PR #111 / V2.12.9）。本文件是后续功能开发的任务顺序，不宣称已在本次文档 PR 中运行 Hosted E2E。

## 本阶段唯一目标

输入标的、研究时间和**明确的** PortfolioContext，经正式 CLI 一次性执行多角色研究与组合审查，产出可解释的最终研究结论。先补齐职责和真实工作流，后做更深入的归档、语义质量、RAG、Memory、性能优化。保持 [既有架构边界](10-多Agent架构与职责边界.md)：本项目不进行券商下单、资金划拨、交易授权或交易审批。不得使用伪造的实时行情、虚构持仓或静默跳过缺失来源来制造「成功」。

## 代码核对后的基线（区别于未来角色清单）

- **现有入口**：`cmd/aegis/main.go` 的 `-portfolio-review -portfolio-file`；[运行说明](README.md) 提供以 `{"state":"NOT_PROVIDED"}` 运行的示例。实际 Hosted Integration 尚需独立复核最近一次结果。
- **现有端到端编排**：`internal/research/full_portfolio_workflow.go` 先调用 `Research.RunArtifact`，然后 `PortfolioReview.Run`，并可选执行限制、What-if 情景和后置语义审查；不会替代缺失的团队职责。
- **已有独立角色**：Market、Technical、Event → Synthesis → Bull/Bear → Research Manager → Trading Agent → Risk Analyst → Portfolio Manager；[现状索引](README.md) 描述这些模块，最终仍需以代码/Hosted 运行验证。
- **明确缺口**：Fundamental Analyst 和独立 Sentiment Analyst 未列为当前实现；[风险设计](11-Risk-Portfolio-Review.md) 明确三方 Risk Debate 尚未实现；Portfolio Manager 目前采用确定性非执行审查。不要将 TradingAgents 参考资料中的角色误写成 Aegis 已实现。
- **不重复开发**：现有 Evidence Ledger、Semantic Review、归档等代码保持兼容，不扩展新功能。现有 Risk/Portfolio 校验不得为了演示通过而放宽。

## 功能顺序（每项有可运行验收）

### F0：验证现有主路径——立即执行

1. 固定当前 main 提交，执行 `go test ./... -count=1`、`go vet ./...`。
2. 按 `docs/README.md` 和 `.github/workflows/integration.yml` 的正式入口运行完整 CLI；记录是否真的调用真实模型及真实来源、输出 JSON 是否包含 `research_artifact`、`portfolio_review`。
3. 将失败分为「环境/凭证/外部来源」「流程接线」「核心契约」三类，只修阻止端到端运行的缺陷。不得以测试桩冒充真实来源。
4. 验收：可复现命令 + 输入来源说明 + 真实执行记录/受控失败原因。文档或单元测试不能代替 Hosted E2E。

### F1：补齐必要 Analyst 职责

1. 首先实现 Fundamental Analyst 的最小数据能力、typed Context、模型解释和 Synthesis 交接；无可用财报时明确报告不足，不凭空编数值。
2. 按对实际研究的独立信息增益评估 Sentiment 是否必须是独立 Agent；若需要，实现同样的最小闭环，不为凑齐角色数创建空 Agent。
3. 验收：对应角色独立测试 + Graph 实际调用 + 全链路 JSON 可消费；所需外部数据的缺失/失败可解释。

### F2：确认 Research Debate / Trading 的完整协作

1. 审计现有 Bull/Bear 是否消费对方先前论点、轮数是否有界，以及 Research Manager 输出是否实际交给 Trading；只修阻塞协作的功能缺口。
2. 验收：一次真实 workflow 记录多轮交接（如配置为多轮）、最终研究计划与 TradeIntent 能沿同一次运行传递；无重复上游数据获取。

### F3：补齐 Risk/Portfolio 的角色协作

1. **先检查数据条件**：已观察到的账户状态、估值、风险限制是否足以让不同风险角色产生独立信息；不足时保留现有单 Risk Analyst 并标记限制。
2. 在真实职责和输入都存在时，增加有界 Aggressive / Conservative / Neutral Risk Debate + Risk Manager，并交给 Portfolio Manager；否则优先实现可信的现有单角色完整路径，禁止假装具备三方辩论。
3. 增强 Portfolio Manager 的研究总结与冲突解释时，保留已有确定性风险否决边界，不把 LLM 结论升级为交易授权。
4. 验收：Research Plan → TradeIntent → 风险评估（满足条件时含辩论）→ PortfolioDecision 完整可观察；未知账户仍为未知，绝不默认为空仓。

### F4：正式端到端收口

1. 提供一条可复制的 CLI 命令、最小配置说明、样例最终 JSON 结构和阶段日志。
2. 覆盖正常完整路径以及至少一个真实缺失数据/模型失败的受控退出路径；保留现有 CI 基础门禁。
3. 验收：无需开发者手动拼接各 Agent；具备所需外部配置时可从输入到最终研究结果运行。明确区分真实 Hosted E2E 和模拟单元测试。

## 开发纪律

- 严格按 F0 → F1 → F2 → F3 → F4 推进；已有能力只验收、不重复发明。
- 每个 PR 解决一个**阻塞功能闭环**的缺口。Issue 要写明触发路径、实际缺口、最小变更及可执行验收。
- 必做 Go 测试、vet、CLI 输入/输出检查；不要求每个 PR 增加归档、语义测评或新框架。
- 非阻塞优化只记录，不把修一个边界问题扩展成多个版本。
- 真实数据、PIT、证据、账户状态和非执行安全边界是**功能正确性**，不是可以暂时跳过的「细节」。
- 全部功能闭环验收之后，才启动 Memory、RAG 深度优化、语义评测及性能与成本专项。

## 第一张开发 Issue（F0）

**标题**：V2.13 F0：核验现有 Multi-Agent CLI 全链路真实执行

**范围**：只验证现有 `-portfolio-review` 完整执行路径，修复阻断完整运行的最小接线问题；不新增 Analyst、不改变 Risk 决策规则，不碰归档和 Semantic Review。

**完成证据**：当前 main 上 Go vet/tests、CLI 黑盒执行、外部模型与数据来源调用证明、最终 JSON 结构/失败证据、对应 CI 链接；任何未经执行的项目必须保持未完成。
