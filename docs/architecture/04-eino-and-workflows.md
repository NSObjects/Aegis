# 04. Eino Agent Loop 与策略内部 Workflow

> 目标：复用 Eino 已有 Agent 运行机制，而不是自己再写 ReAct Runtime。返回 [架构索引](README.md)。

## 1. 什么由谁决定

| 决策 | 负责方 | 原因 |
|---|---|---|
| 什么时候启动/取消策略、超时/预算 | Aegis Host | 可靠性、隔离和统一治理 |
| 研究阶段、并行/条件/受控循环 | Strategy 内的 Graph / Go Workflow | 不同策略需要不同流程 |
| 当前还缺什么证据、选哪个已授权 Tool | 策略内部 Eino Agent Loop | 探索性研究需要自主决策 |
| Tool 底层查本地 DB、在线 API 还是 MCP | Data Capability 的确定性 Resolver | 时点、新鲜度、来源和成本不该让 LLM 猜 |
| 公式计算、批量扫描 | Go / 确定性执行 | 高吞吐、可验证、避免 LLM 错算 |
| 强制权限、PIT、安全诊断及结果验证 | Host / 受信服务 | 不能靠 Prompt 是否主动调用 |

`adk.Runner` 是执行 Agent 的入口，`ChatModelAgent` 内部的 Tool Calling / ReAct Loop 才负责模型自主决定后续工具。Graph 负责编排明确的阶段/边界；Eino 本身不定义金融策略。

## 2. 两种必须同时支持的策略

```text
Strategy A: TradingAgents-style Research
  Graph -> Market / Technical / News Research
      -> each Analyst ChatModelAgent -> (Tool -> Observation -> Tool ...) bounded
      -> Bull/Bear Debate (bounded)
      -> Research Manager
      -> Typed ResearchResult

Strategy B: Trend Pullback
  Go batch Data Capability request
      -> deterministic indicators / support / shrinking volume
      -> screening rules / candidate evidence
      -> Typed ScreeningResult
```

两条路径都可直接使用 `market.bars` Capability；第二条不经过 LLM。不能为了“所有能力都是 Tool”强迫纯 Go 策略走 Agent Loop，也不应由 Aegis Host 规定策略 A 的 Bull/Bear 是策略 B 的必选阶段。

## 3. 将官方能力暴露为语义 Tool

推荐 `get_market_bars`、`get_market_snapshot`、`search_news`、`search_disclosures`、`calculate_indicators` 等**语义型 Tool**，而不是默认提供 `execute_shell`、`run_arbitrary_sql`、`fetch_any_url`。

同一 Capability 可有两种受控入口：
- **Tool Adapter**：给 Eino ChatModelAgent，附类型化参数/来源状态、权限、预算。
- **Go SDK / service API**：给批量与确定性策略，避免按标的重复消耗 token。

未来有大量 Tool 可使用目录搜索/按需暴露，但**先授权再发现**，不能通过 ToolSearch 查到未授权的第三方工具。LLM 工具选择不能越过网络与数据访问策略。

## 4. 需要保留确定性边界的流程

- 多空辩论可以由策略自己的 Graph 控制轮数、参与者与退出规则；模型负责论证，不负责绕过最大轮数。
- Market Regime 可以影响平台选择哪些独立策略执行，但不应在一个策略内部递归调用另一个社区策略。
- 事件研究若要求多独立来源，应由工具结果结构、检索计划和校验器验证，不能仅靠“请多查几个来源”的提示词。
- 严格历史研究必须在平台访问层过滤未来消息/修订数据，不可以由模型自行决定是否忽略。
- 模型输出 JSON Schema、Evidence ID 与引用关系应做程序化校验；模型声称有新闻不等于真的有对应证据。

## 5. MCP / Skill 的最佳用法及风险

**MCP**：优先对接提供商已有 Server，获取实时新闻、公告、行情等现成 Tool；验证 Schema、许可、鉴权、限流、异常和来源。它减少重复 HTTP 适配代码，但不是天然高质量/高可靠数据。

**Skill**：指导模型如何使用相关工具，也可能引用脚本或外部文档。Skill 不直接等价于数据源；第三方 Skill 不可信，不能因为其内容要求安装依赖或 `execute` 命令就赋予权限。

**自建 API / Crawler**：有必要时保留官方适配器，尤其官方定时批量采集；不要追求所有 Provider 都包一层 MCP。

Eino Agent Tool 调用具有轮次、超时、并发、成本限制；原始 MCP/网页返回可能有提示注入，不能以高信任系统指令处理。

## 6. 独立 Workflow 的执行轨迹

Host 需要统一消费和持久化安全事件：
- strategy run started/completed/failed/cancelled
- graph stage / agent started/completed/skipped/failed
- tool/provider call name、开始结束、耗时、来源及状态
- 输入/输出工件引用、版本与错误码

不得记录秘密、模型隐藏推理链、任意原始私人内容。Web 应能展示**实际走过**的节点，而不是把未执行的预设 Graph 画成成功状态。

## 7. 故障试验

1. Agent 看到空新闻结果仍编造“多个来源支持” → 应被证据引用验证拦截。
2. LLM 不主动调用必需风险检查 → 由确定性平台阶段补齐或阻止产出，不依赖提示词。
3. 模型反复搜索同一新闻 → 最大轮数/成本预算/去重有效终止。
4. MCP 返回未来公告供历史回测使用 → PIT 被拒绝。
5. 5000 只股票逐一 Agent Tool Calling → 架构验收失败，必须改用批量 SDK。
6. 某个 Tool/LLM 不可用 → 保持明确错误、数据缺口和可审计的失败/部分研究结论，不强制伪成功。
