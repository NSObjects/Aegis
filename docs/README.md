# Aegis 文档与当前能力

> 状态基准：2026-10-08 · `main` 已合并 V2.11.4（PR #86）；该版本的 What-if 组合情景研究能力已进入主线。  
> **权威架构**：[10-多Agent架构与职责边界.md](10-多Agent架构与职责边界.md)。本文是按仓库代码整理的能力索引，不覆盖该架构约束。

## 当前开发路线与下一项任务

**开发唯一导航入口：** [DEVELOPMENT_ROADMAP.md](DEVELOPMENT_ROADMAP.md)。查看其顶部「当前任务/下一项」并按任务编号执行。**V1 内核验收后首先开发完整研究工作台**，而不是仓位管理。

- **R1 研究工作台**：R1-01 共用 Application Service → R1-02 任务生命周期 → R1-03 持久化 → R1-04 HTTP/SSE → R1-05 Web 任务页 → R1-06 结构化报告 → R1-07 历史/Dashboard → R1-08 Web E2E。
- **R2 对话式研究**：在已可用的工作台上接 Conversation Agent，支持发起研究、证据追问、多轮上下文和历史比较。
- **R7 组合与动态仓位管理**：单独的大型后续 Epic，**非 R1/R2 上线依赖**；现有 Risk/Portfolio 审查先保留，不当作完整的仓位管理功能。

[MULTI_AGENT_ROADMAP.md](MULTI_AGENT_ROADMAP.md) 只负责当前 R0/F0～F4 的收尾；[10-多Agent架构与职责边界.md](10-多Agent架构与职责边界.md) 是永久权威架构与研究边界。

> 下方「截至 V2.11.4」的描述为历史能力索引；每次实施前按最新 main/PR/CI/Hosted 证据核实，不按旧索引认定开发状态。

## 产品定位

Aegis 是面向 A 股的**投资研究与决策辅助系统**，借鉴 [TradingAgents](research/tradingagents/README.md) 的分角色研究、Bull/Bear 论证与研究管理方式。

**永久不做真实交易执行**：没有券商下单、委托/成交、订单审批、资金划拨或交易授权接口。名称中的 `Trading Agent`、`TradeIntent`、`Portfolio Manager` 均为**研究角色或研究结论**，不能当作自动交易能力。回测中如有成交记录，也只应是离线模拟。

## 已进入 main 的研究链路（截至 V2.11.4）

```text
Market Agent ─────────→ MarketContext    ┐
Technical Agent ──────→ TechnicalContext ├─→ Synthesis
Event/News Agent ─────→ EventContext     ┘      ↓
                                       ResearchArtifact
                                              ↓
                                  Bull / Bear Debate
                                              ↓
                                      Research Manager
                                              ↓
                                  Trading Agent → TradeIntent
                                              ↓
                                    Risk Analyst → RiskContext
                                              ↓
                              Portfolio Manager → PortfolioDecision
```

- **Market**：市场环境与市场风险偏好；实际数据能力和可信度以当次研究输入及返回证据为准。
- **Technical**：通过 `internal/capability/technical` 的原生 Go HTTP/Tushare 能力提供技术研究上下文；不再依赖旧的临时 `stock_technical_research.py` 路径。
- **Event/News**：支持公告、新闻与搜索来源的获取、证据规范化、时间语义、相关性与语义核验；某个外部来源失败不等于一定可以安全得出完整研究结论。
- **Research Synthesis**：校验多维事实，并生成带来源的 `ResearchArtifact`。
- **Bull / Bear + Research Manager**：基于已核验证据形成并交叉检查投资论点，不应编造证据、引用或论证 ID。
- **Trading Agent**：把研究论点形成 `TradeIntent`（例如 `MONITOR / CANDIDATE / REJECTED`），不是委托对象。
- **Risk Analyst / Portfolio Manager**：形成 `RiskContext` 与 `PortfolioDecision`（`NO_ACTION / BLOCKED / REVIEW_REQUIRED`）；以上都是不可执行的研究判断。

`internal/research` 仍保留历史 Decision 路径以兼容旧调用方。完整 Portfolio Review 是显式启用的独立路径，不把两条研究图偷偷串起来重复运行。

## 账户与组合研究能力

| 版本 | 已实现能力 | 核心边界 |
|---|---|---|
| V2.10 | 真实 ResearchArtifact → Debate → Research Manager → TradeIntent → Risk / Portfolio | `PortfolioContext` 必须明确传入；未知账户是 `NOT_PROVIDED`，**不是空仓** |
| V2.11.1 | 可选账户快照时效限制 | 按研究截止时间执行 PIT 校验，不以本机时间臆断历史数据 |
| V2.11.2 | 可选已观察账户估值和当前敞口 | CNY 分整数、账户严格对账；无估值不推算权重 |
| V2.11.3 | 调用方定义的当前组合风险限制 | 可独立提供风险策略文件，确定性检查单股权重与总投资比例；`INSUFFICIENT_DATA` 不等于 `PASS` |
| V2.11.4 | **已合并至 main（PR #86）** | 可选研究性 What-if 组合金额情景；仅对显式账户估值与风险策略进行确定性测算，不是订单、无审批或执行能力 |

相关契约：[13-生产组合审查链路.md](13-生产组合审查链路.md) · [14-快照时效](14-V2.11.1-Portfolio快照时效约束.md) · [15-账户估值](15-V2.11.2-已观测账户估值与敞口.md) · [16-当前风险策略](16-V2.11.3-当前风险策略检查.md) · [17-情景研究](17-V2.11.4-显式交易情景与交易后风险诊断.md)。

## 当前代码导航

| 代码路径 | 职责 |
|---|---|
| `cmd/aegis` | 正式 CLI、参数及显式研究输入校验 |
| `internal/config`, `internal/llm` | 模型配置与 DeepSeek Adapter（含既有协议兼容处理） |
| `internal/runtime`, `internal/middleware` | Runtime、Skill / Filesystem / Context 能力 |
| `internal/agent/market`, `technical`, `event` | 独立事实研究 Agent |
| `internal/capability/technical`, `event` | 技术数据与新闻/公告/搜索数据能力 |
| `internal/research/synthesis`, `debate`, `manager` | 结构化事实交接、正反研究与研究管理 |
| `internal/research/workflow.go`, `full_portfolio_workflow.go` | 上游研究和完整 Portfolio Review 的编排 |
| `internal/agent/trading`, `risk`, `portfolio` | 研究观点、风险分析与组合研究角色 |
| `internal/portfolio`, `internal/portfoliodecision` | 账户事实、估值、当前限制和不可执行的组合判断 |
| `internal/research/planning` | 研究计划与受约束的研究能力调用 |

并未因为架构文档描述了 Fundamental / Capital Flow / Strategy / Backtest 等长期角色，就把它们标成已完成。Agent 应按真实独立职责增量加入，不应按 Tushare、MCP、Web Search 等数据来源创建 Agent。

## 运行与验证

使用真实来源时，请通过外部环境配置相应凭证，勿提交到仓库。生产 CLI 的完整审查路径要求**显式账户状态**：

```bash
printf '{"state":"NOT_PROVIDED"}\n' > portfolio.json
go run ./cmd/aegis -symbol 600519.SH \
  -portfolio-review -portfolio-file ./portfolio.json
```

此 `NOT_PROVIDED` 只表示未提供账户信息，**不代表没有持仓、现金或敞口**。这是生产入口的保守输入示例，不能证明券商账户已经连接。

仅在调用方拥有真实、完整的账户估值与自行确定的策略时，才另外使用：

- `-portfolio-max-age`：账户快照最大容许年龄；
- `-risk-limits-file`：独立的当前组合风险规则；
- `-trade-scenario-file`：V2.11.4 已进入 `main` 的可选 What-if 研究输入；仅在 `-portfolio-review` 已启用且同时提供 `-risk-limits-file` 时允许使用，不代表交易或审批。

普通 CI 运行 `go vet ./...`、`go test ./... -count=1` 及真实 CLI 黑盒非法输入门禁。手动触发的 `.github/workflows/integration.yml` 的 `run_production_cli` 使用真实模型与来源、以 `NOT_PROVIDED` 验证完整研究链路和 JSON 契约。**它不验证真实持仓、真实市值或新增情景输入**；这些计算应由确定性领域/Workflow 测试验证。

外部 API、行情、新闻、模型响应均可能失败。实际研究必须尊重来源证据、PIT 时间及 `INSUFFICIENT_CONTEXT`，不能为了让 Hosted 测试通过而引入假数据或跳过校验。

## 文档维护与工程约束

- [10-多Agent架构与职责边界](10-多Agent架构与职责边界.md)：最高优先级，尤其是永久研究边界、Role/Agent/Capability/Provider 分层。
- [01-设计原则](01-设计原则.md)、[07-工程实践与踩坑](07-工程实践与踩坑.md)：设计原则、确定性验证、接口与工具实践。
- [09-Technical数据口径](09-Technical数据口径.md)：技术指标与时间语义。
- [12-Event Hosted E2E稳定性](12-Event-Hosted-E2E稳定性与语义匹配契约.md)：事件取证和真实来源验证要求。
- [13-生产组合审查链路](13-生产组合审查链路.md)：正式 CLI 的 Risk / Portfolio 端到端交接。
- [08-演进路线与现状](08-演进路线与现状.md)：含历史阶段记录，**不应把 V2.6 章节当作今日最新架构**。

本仓库当前未发现 `AGENT.md`、`AGENTS.md`、`CONTRIBUTING.md`。如果后续新增正式贡献规范，应更新此索引并保证与 [10-架构约束](10-多Agent架构与职责边界.md) 一致。
