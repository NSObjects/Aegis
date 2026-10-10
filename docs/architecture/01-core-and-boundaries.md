# 01. Aegis Core、Capability 与策略边界

> 长期目标，不代表当前代码已经实现。返回 [架构索引](README.md)。

## 1. 六个不同概念

| 对象 | 应负责 | 不应负责 |
|---|---|---|
| **Host / Core** | 策略注册、启动/终止、权限、隔离、任务生命周期、执行预算、事件追踪、结果存储 | 固定研究流程、趋势回踩规则、Bull/Bear 必选 |
| **Capability** | 可复用的语义能力：行情、搜索、指标、LLM、记忆、回测基础 | 某策略的完整业务决策 |
| **Provider** | Capability 具体来源，如本地 DB、TuShare、新闻服务商 | 独立研究 Agent 或策略 |
| **Adapter / Mechanism** | 接入协议，如 SQL、HTTP、MCP、Crawler | 定义金融研究策略 |
| **Skill** | 指导 Agent 如何调用工具或执行操作；可包含脚本和文档 | 保证数据真实或获得额外权限 |
| **Strategy Package** | 自行定义输入、Workflow、Agent/规则、依赖和类型化结果 | 直接调用其他策略构建跨包链路 |

数据、LLM、Agent 基础设施不要求全部成为插件；仅在确有扩展需求时开放注册接口。一个单体仓库中的清晰逻辑边界完全可行。

## 2. 执行逻辑

```text
User/Automation -> Application -> Host
   1. Verify strategy identity/version, RunRequest and grants
   2. Resolve immutable package, capability bindings and limits
   3. Start independent strategy workflow
      - Eino multi-agent or Graph
      - Go deterministic computation
      - optionally trusted isolated worker
   4. Validate typed result, temporal policy and evidence
   5. Save result/events/errors/lineage under Run ID
```

Host 不是另一层超级 Trading Agent。它不知道具体哪个策略需要 Market Analyst 或财报；策略自行判断。另一方面，Host **必须知道**权限、错误、时间、结果契约和运行状态。

## 3. 为什么 TradingAgents 只是其中一种策略

当前 Aegis 的 Market / Technical / Event / (optional Fundamental) → Synthesis → Bull/Bear → Research Manager → TradeIntent → Risk/Portfolio 是某种**研究组织方法**，不是全市场选股的唯一正确路径。未来可以将其作为官方内置 `research` 类型包；不在本 PR 搬动代码。

趋势回踩可能只需本地批量行情和 Go 规则；事件研究可能用多 Agent 调用最新 MCP 新闻；仓位策略需要已观察账户与风险预算。它们的业务输入输出**不可能强行统一为一个 Decision**。

## 4. 策略隔离的明确含义

- A 和 B 互不调用、无 runtime-to-runtime 依赖、不共享可变状态；各自错误、重试、暂停和版本隔离。
- 它们可通过同一个官方 Data Capability 读取行情，可以使用同一个受控 LLM Provider。
- Web/应用层可单独启动 A 和 B 并比较结果，不意味着 A 依赖 B。
- 同一策略内部可有多个 Agent、Tool、Graph/子流程，允许内部 Agent-as-Tool；**不允许将另一个 Strategy Package 作为 Tool 递归调用**。
- 市场 Regime 可影响 Application/Host 对独立策略的启停/调度；不是改变其他策略内部逻辑。
- 逻辑独立不意味着现在必须为所有官方策略创建容器；第三方不可信代码需要更强隔离。

## 5. 严格边界

- **永久只做研究与决策辅助**：无券商下单、订单审批、自动交易、真实自动调仓、资金划转。
- 风险判断中的 `PASS` 或账户组合诊断仅表示已声明条件下的模拟/研究结果，不表示实盘许可。
- `NOT_PROVIDED` 账户状态不能被当作空仓；`INSUFFICIENT_DATA` 不能当成 PASS。
- 数据时点、来源、权限、计算一致性、Run 结果校验必须由确定性机制实现；不可寄希望于提示词“请严格遵守”。
- 执行 `COMPLETED` 表示协议内任务完成，并不代表股票判断正确或投资有效。

## 6. 迁移不变量

迁移现有内置研究 Workflow 到策略边界前，必须通过现有 CLI/Hosted 的行为兼容验收：同一研究只跑一遍、证据与 ResearchArtifact 连续、Debate/TradeIntent/Risk/Portfolio 结构与失败语义不改变、PIT 不倒退。参见 [06 验收计划](06-adoption-plan-and-acceptance.md)。
