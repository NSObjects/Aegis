# 对 Aegis 的取舍：吸收什么、改造什么、不搬什么

本章把前五章的结论收敛成可执行的判断。左侧是 TradingAgents v0.5.0 的**实际做法**（附 `路径:行号`），
右侧是 Aegis 的落点（对应到 `docs/` 中的设计文档与现有代码）。

> 判定标准有两条：① 这个机制解决的是**通用问题**（多 Agent 协作、数据时效、状态管理），还是**它自己的问题**；
> ② Aegis 的场景（A 股短线、市场环境优先）是否需要它。只满足第一条的，改造后吸收；两条都满足的，直接吸收。

---

## 1. 直接吸收：机制成熟、Aegis 正需要

### 1.1 字段即接口的 State 契约

**TradingAgents 做法**：Agent 之间不通过自然语言对话传递信息，而是写入具名 state 字段——
`market_report` / `investment_debate_state.{bull,bear}_history` / `investment_plan` / `trader_investment_plan` /
`final_trade_decision`（`tradingagents/agents/utils/agent_states.py:47-77`）。

**为什么值得抄**：它让「谁生产什么、谁消费什么」变成可枚举、可断言、可持久化的东西。图能断点续跑，正是因为它不依赖对话历史。

**Aegis 落点**：`internal/agent/market/context.go` 的 `Context` 已经是这个形态。需要补的是**跨 Agent 的字段契约**——
Market Agent 产出 `MarketContext`，Trading Agent 消费它。目前 `internal/agent/trading/agent.go` 只有一行 `package trading`，
这条契约还没有落地。建议把「Agent 输出类型 ↔ 消费方」整理成一张表放进代码文档，作为组装的依据。

### 1.2 控制流外置到纯函数

**TradingAgents 做法**：循环与路由不在 Agent 里，而在 `ConditionalLogic`（`tradingagents/graph/conditional_logic.py:52`、`:63`）：
轮数上限、下一个发言人、何时退出，全部由普通 Python 函数根据 state 字段决定。

**为什么值得抄**：整个流程变成可单测的确定性代码，不受 LLM 波动影响。

**Aegis 落点**：这正是 `docs/01-设计原则.md` §4「Graph 管阶段、ADK 管智能行为」的同一结论，TradingAgents 提供了一个验证过的样本。

> **但必须改造一处**：它用回复文本前缀判断发言人（`current_response.startswith("Bull")`），并且要靠补全 `path_map` 来防止崩溃（`graph/setup.py:28-31`，issue #1088）。
> Aegis 用 Go，应当用**枚举或显式状态位**表达发言人，把这个脆弱点彻底消除，而不是照抄兜底方案。

### 1.3 运行开始先结算历史（自成长闭环的最小可用形态）

这是本次研究**最有价值的单个机制**。

**TradingAgents 做法**（`tradingagents/graph/trading_graph.py:539`）：

```text
create_run_state()
   ├── ① _resolve_pending_entries(ticker)      取历史 pending 决策 → 拉实现收益 → 生成反思 → 批量写回
   ├── ② get_past_context(as_of=trade_date)    注入已结算的教训（past_context）
   └── ③ resolve_instrument_context(ticker)    确定性解析标的身，防止幻觉
```

配套的三条工程纪律同样重要：

| 纪律 | 证据 | 意义 |
|---|---|---|
| 持有窗口没走完就**保持 pending** | `:323-324`（#1169） | 绝不用部分数据提前结算 |
| 反思失败**不阻断**本次分析 | `:377-382` | 主流程与学习流程解耦 |
| 教训按 `as_of` 过滤后才注入 | `:409-418`（#1251） | 历史/回测运行不会看到未来才知道的教训 |
| 结算先于注入 | `:547` 前于 `:552` | 本次刚结算的教训，同一次运行里就被消费 |

#### 但它不是闭环学习——不要把「注入 prompt」当成学习方法

这一点必须说清楚，否则容易把 TradingAgents 的能力估计过高：

- **教训的唯一消费点只有一处**：Portfolio Manager 的 prompt（`agents/managers/portfolio_manager.py:38`）
- **穷举全仓不存在任何把反思写回 prompt / 配置 / 策略的代码路径**
- **反思是纯字符串、不带结构化归因**（`graph/reflection.py:63`、`agents/utils/memory.py:219`）
- 没有 Evaluation，也没有 Promotion

所以它实现的是 **Outcome → 下次 context**，而不是
`Execution → Outcome → Reflection → **Proposal → Evaluation → Promotion**`。

**对 Aegis 的直接含义**：`docs/08-演进路线与现状.md` §4 定的六阶段目标里，
TradingAgents 覆盖了前三阶段的一半（Execution、Outcome、Reflection 产出），
**Proposal / Evaluation / Promotion 完全不具备**。
Aegis 应当直接吸收它的**时序模型与工程纪律**（上表四条），
但要清醒认识：那一跳半是**最难的工程部分**（怎么定义「结果已知」、怎么在正确时间点结算、怎么不污染实盘日志），
而**真正让它变成「成长」的是后三跳**——而这三跳没有现成参考，必须自己设计。

> 反过来说：如果 Aegis 只做到「把教训拼进 prompt」，那得到的只是**更长更贵的 prompt**，不是自成长。

### 1.4 `as_of` 过滤 + `resolution_date`

**TradingAgents 做法**：不只是「数据按日期取」，而是**「教训也只注入当时已知的」**。
`_fetch_returns` 返回 `resolution_date`（最后使用的价格 bar 的日期，即结果已知之时，`:335-337`），
`_memory_as_of` 用它作为注入截点（`:409`）。

**Aegis 落点**：`docs/06-时间语义与输出契约.md` 已经确立了 `as_of` 语义的五条规则，但**只有规则、没有机制**。
TradingAgents 补上的正是机制部分：把「数据的时间点」与「认知的时间点」分开记录。
这一点在 Aegis 做回测时是必需的——否则回测会用到当时不可能知道的教训。

### 1.5 确定性验证工具作为事实基准（反幻觉）

**TradingAgents 做法**：Market Analyst 的 prompt **强制要求**在写报告前调用 `get_verified_market_snapshot`，
并要求把它当作任何精确 OHLCV / 价格位 / 指标值的**唯一事实来源**；若其他工具输出与之冲突，
**要求标注冲突而不是自己编一个协调后的数字**。同时明确禁止「无工具输出与具体日期价格支撑的历史验证、支撑阻力反弹、精确涨跌幅」。

**Aegis 落点**：这正是 `docs/02-数据能力层.md` 里 Capability / 数据契约要解决的问题，
但 TradingAgents 多给了一层**prompt 侧的强制约束**——光有契约不够，还要在指令里明确「谁是事实基准」以及「冲突时怎么办」。
建议 Aegis 的 Market Agent prompt 增加同款条款。

### 1.6 结构化输出的优雅降级 + 渲染回 Markdown

**TradingAgents 做法**：只有 3 个 Agent 用 `with_structured_output`（Research Manager / Trader / Portfolio Manager，
`agents/managers/portfolio_manager.py:27`）；`bind_structured` 在 Provider 不支持时返回 `None`，
由 `invoke_structured_or_freetext` 降级为自由文本（`agents/utils/structured.py:42-56`）。
类型化结果再通过 `render_pm_decision` **渲染回 Markdown** 存入 state，让记忆日志、CLI、报告继续消费同一种形状。

**Aegis 落点**：Aegis 当前是「JSON object 模式 + 自己 parse + Validate」（`internal/agent/market/agent.go`）。
这套做法可以保留，但需要补两点：① Provider 不支持结构化输出时的**降级路径**；
② 若将来改为 schema 绑定，**不要把下游消费方一起改掉**——渲染回既有形状是对的。

### 1.7 解析失败要显式失败，不要编造中性值

**TradingAgents 做法**：`extract_rating` 解析不出评级时返回 `REVIEW`，
源码明确说明（`agents/utils/rating.py:23-31`）：
> 这不是一个可交易仓位；它标记的是「需要人工介入或重跑」的输出，而不是悄悄降级成 `Hold`。

**Aegis 落点**：与 Aegis `Market Agent` 的 `Validate` 同源思路，但 TradingAgents 把它推进了一步：
**失败信号本身要有独立的取值**，不能与「中性的正常结果」共用一个值。
Aegis 的 `Regime` 有 `MIXED`，需要警惕 `MIXED` 不要成为「没想清楚」的垃圾桶——TradingAgents 的 `REVIEW` 是一个更好的范式。

---

## 2. 改造后吸收：方向对，但不能照抄

### 2.1 双层模型 → 保留 per-agent override 并叠加 tier

**TradingAgents 做法**：全局单 Provider + 两档模型（`quick_thinking_llm` / `deep_thinking_llm`），
12 个 Agent 里只有 Research Manager 与 Portfolio Manager 用 deep（`graph/setup.py:76-92`）。
**它不支持 per-agent 模型选择。**

**Aegis 更强的地方**：`internal/config` 的 `default_model` + per-agent override（`ModelProfileForAgent`）表达能力更强。

**建议**：保留 Aegis 的 override，在此之上叠加 tier 语义——即 profile 可以声明自己是 quick 还是 deep，
组装时默认按 tier 选，需要时再按 Agent 覆盖。两者不冲突，且 TradingAgents 的两档分工经验（**产出信息用快模型、最终仲裁用强模型**）值得直接采用。

### 2.2 模型能力探测：用数据表取代硬编码

**TradingAgents 做法**：`llm_clients/capabilities.py` 用精确 ID → 正则 → 默认的三级解析返回 `ModelCapabilities`（6 个字段），
`StructuredMethod` 有四个取值，并定义了三条降级路径。Provider 元数据用声明式注册表（`openai_client.py:183-234`，17 个 Provider）。

**发现的一处接缝缺陷**（可作为反例）：`model_catalog.py:158` 把 `deepseek-flash` 作为主推模型，
但 `capabilities.py:95-98` 只登记了 `deepseek-chat` / `deepseek-reasoner` / `deepseek-v4-flash` / `deepseek-v4-pro`，
正则 `^deepseek-v\d`（`:113`）也匹配不到 `deepseek-flash` —— 于是它静默落到 `_DEFAULT`，拿到一组可能不正确的默认能力。

**教训**：能力表必须与模型目录**同源或有校验**，否则新增模型会静默拿到错误能力。Aegis 若要引入能力表，
应当让「模型目录」与「能力表」由同一处定义派生，而不是两张需要人工同步的表。

**Aegis 落点**：目前 `internal/llm/deepseek.go:35` 把 `ResponseFormatTypeJSONObject` 写死在 DeepSeek 创建逻辑里。
建议抽出 `internal/llm/capabilities`，让「该模型是否支持结构化输出 / tool calling / 多长上下文」成为可查询事实。

### 2.3 Checkpoint：签名要覆盖「图形状」

**TradingAgents 做法**：`thread_id` 里编码图形状签名（分析师选择、辩论轮数、风控轮数、资产类型、持仓指纹，`:420-434`）；
续跑时传 `None` 而不是初始 state（`:490-498`，否则消息会重复）；成功后清除 checkpoint（`:516`）。

**Aegis 落点**：Aegis 目前还没有 checkpoint 设计。若要引入，这三点是必须的：
**配置变更不能被误当成续跑**、**续跑语义要与状态初始化区分**、**成功即清**。

### 2.4 Backtest：把它当作「决策质量评估」，而不是持仓模拟器

**TradingAgents 做法**（`tradingagents/backtest.py`，201 行）：用**同一张图**在 (ticker, date) 网格上重跑，
用决策日志本身当结果表。它的 docstring 有一段非常明确的边界声明：

> 本模块评估**决策质量**。它不是持仓模拟器，并且**不允许**长成一个。
> 把评级变成成交单需要数量、成交价和现金账本，这三样系统都没有；在这里发明它们，
> 等于把一个执行模型塞进一个评估工具。

配套的工程细节同样值得学：
- **独立的决策日志**（`results_dir/backtest/<run_id>/trading_memory.md`），避免回测污染实盘上下文
- **幂等续跑**：日志里已有的 (ticker, date) 单元格直接跳过，中断后重跑即续
- **收尾补一次 `settle_pending`**：因为结算发生在下次运行的开头，每个 ticker 的最后一个单元格否则会永远 pending
- 单个 vendor 失败**不终止**整个扫描

**Aegis 落点**：`docs/01-设计原则.md` §2 的映射表把 Backtest 列为「不属于 Eino，独立回测引擎」——方向一致。
建议采纳 TradingAgents 的**范围声明方式**：在 Aegis 回测模块的开头就写死「评估决策质量，不做持仓模拟」，
并同样采用独立日志 + 幂等续跑。

### 2.5 持仓上下文：三态必须区分

**TradingAgents 做法**（`tradingagents/portfolio.py`）：`portfolio_context` 有三种状态且必须保持区分——
**有持仓 / 空仓 / 完全未提供**。源码理由：把「未提供」当成「空仓」等于**凭空捏造了一个关于调用方账户的事实**。

**Aegis 落点**：Aegis 未来一定会传入账户信息，这个三态区分应当提前定进契约，而不是等到踩坑。

同源的还有一条更普遍的纪律，值得单独提出：

> **「未观测」必须与「不存在」分开表达。**

TradingAgents 用 `REVIEW` 表达「解析不出来」，而不是 `Hold`；用三态表达「未提供持仓」，而不是「空仓」。
Aegis 的错误分类里应当有对应的一类——建议引入 `ErrNotObserved` 之类的错误位，
明确区分「数据源说没有」与「我们没去查 / 查不到」。

### 2.6 A 股特有的两个空白：交易日历与复权语义

这两点 TradingAgents **完全无解**，Aegis 必须从零设计。它们不是「学它」，而是「它帮不上忙」的地方。

**① 交易日历**

全库**没有任何交易日历**：`look_back_days` 一律按自然日计算，
`MAX_OHLCV_STALE_DAYS = 10` 是为跨长周末放宽的**经验值**（`tradingagents/dataflows/stockstats_utils.py`），
`_fetch_returns` 里 `holding_days` 转日历跨度用的是 `round(holding_days * 7 / 5) + 7` 这种近似
（`graph/trading_graph.py:311`）。

这在美股/港股场景勉强可用，但 A 股会失效：春节与国庆长假、调休安排会让自然日回看严重失真。
而 Aegis 的时间语义明确要求「当前交易日数据尚未完整时，使用最近一个数据完整的交易日」
（`docs/06-时间语义与输出契约.md`）——这需要**真实的交易日历（含调休）**，不是自然日近似。

**② 复权语义**

TradingAgents 用 `auto_adjust=True`，这是**隐式前复权**：历史价格会随每次除权除息被重算。
这与 point-in-time（「当时看到的价格」）在概念上**直接冲突**——它让你在回测里看到的「历史价格」其实是今天才确定的数字。

A 股的复权选项更多（不复权 / 前复权 / 后复权），且短线策略对价格连续性敏感。
Aegis 必须**显式建模复权方式**，并明确回答：回测用哪种、Agent 看到哪种、两者是否一致。
这个问题不解决，point-in-time 做得再严格也会被复权悄悄破坏。

> 结论：Aegis 的数据能力层（`docs/02-数据能力层.md`）在落地时，
> 交易日历与复权语义应当作为**契约的一部分**显式声明，而不是留给数据源实现各自解释。

---

## 3. 明确不搬

| 不搬的东西 | 理由 |
|---|---|
| **整图形态**（分析师流水线 → 多空辩论 → 交易计划 → 风控辩论 → 组合决策） | 它以「给定一只股票产出判断」为目标，**没有把「当前市场环境适合什么打法」提升为第一层决策**。Aegis 的第一问必须是「现在是什么市场」 |
| **字符串前缀路由** | 脆弱，靠补全 path_map 兜底；Go 实现应用枚举 |
| **固定轮数辩论** | 轮数是配置常量，不随证据冲突程度自适应，一遍跑满。Aegis 应按 Evidence 冲突程度触发 |
| **分析师串行执行** | 端到端延迟累加，无并行 fan-out |
| **清空 messages 后信息降级为文本** | 后续 Agent 只拿到报告字符串，原始 tool 结果丢失；Aegis 应保留可追溯的中间证据 |
| **全局单例式 config**（`set_config`） | 进程内全局投递不利于并发多实例；Aegis 用依赖注入 |
| **ticker 维度的记忆结算局限** | 其他 ticker 的待结算条目会堆积，需外部调度补偿；Aegis 若做策略维度反思应设计批量结算 |

---

## 4. 对 Aegis 现有文档的纠偏

研究过程核实了 `docs/01-设计原则.md` §2 对 TradingAgents 的描述，**绝大部分准确**（Agent 名单、条件逻辑主导执行、六类机制、point-in-time 的存在）。
以下三处需要修正：

| 原文 | 实际情况 | 修正建议 |
|---|---|---|
| 映射表：「Backtest → **不属于 Eino**，独立回测引擎」 | TradingAgents **有** `tradingagents/backtest.py`（201 行），但不是持仓模拟器，而是决策质量评估器 | 保留「Aegis 回测独立于 Eino」的结论，但补上 TradingAgents 的范围声明方式作为参照 |
| 映射表：「Signal Processing → —— → Go deterministic logic」 | TradingAgents 的 `SignalProcessor` **也已经是确定性的**（`signal_processing.py:20-38`，不再调用 LLM，只做正则提取） | 这不是 Aegis 的差异点，应改为「双方都用确定性逻辑，TradingAgents 的做法是解析失败返回 `REVIEW`」 |
| §2.3「值得吸收的机制」提到 Memory / Reflection | 准确，但**低估了它的完整度** | 补上「运行开始先结算历史」这一具体机制（本章 1.3），这是比「有记忆」重要得多的工程细节 |

另外补充一条原文未提及、但 Aegis 应当知道的事实：
**TradingAgents 不支持 per-agent 模型选择**（只有全局 Provider + quick/deep 两档）。
Aegis 的 `ModelProfileForAgent` 在这方面**已经领先**，不需要向它看齐。

---

## 5. 差距分析：Aegis 现在缺什么

| 能力 | TradingAgents | Aegis 现状 | 优先级 |
|---|---|---|---|
| 数据时效性（point-in-time） | 贯穿 12 个文件的系统约束，连续 3 个版本在修 | `docs/06` 有**规则**，无机制 | 高——回测的前提 |
| 结果结算 → 反思 → 注入 | 已实现（同一 ticker、持有期后） | **未实现**（`docs/08` 列为长期目标） | 高——自成长的地基 |
| 记忆日志 | Markdown 追加式 + 幂等去重 + 轮转上限 | 无 | 高——结算的载体 |
| 数据能力契约与降级 | vendor 链 + 校验 + 错误分类 | **已设计、未实现**（Capability / Binding） | 高 |
| Runtime（Skill + 执行） | 不适用（Python 进程内） | `internal/runtime` 未落地，暂用 `internal/skillruntime` | 高——当前阻塞项 |
| **交易日历（含调休）** | **无**，自然日近似 + 经验常量 | 无 | 高——A 股必需，它帮不上忙 |
| **复权语义** | 隐式前复权（`auto_adjust=True`），与 PIT 冲突 | 未定义 | 高——A 股必需，它帮不上忙 |
| 能力探测表 | 有（但存在目录/能力不同源的缺陷） | 硬编码在 `deepseek.go:35` | 中 |
| Checkpoint / 恢复 | 已实现（图形状签名） | 无 | 中 |
| 回测 | 已实现（决策质量评估 + 幂等续跑） | 无 | 中——依赖 point-in-time |
| per-agent 模型 | **不支持** | **已实现** | — 已领先 |
| 市场环境第一层决策 | **没有**（Aegis 的核心差异） | 设计已定（Market Agent），实现中 | 最高——这是 Aegis 存在的理由 |

---

## 6. 建议的落地顺序

严格遵循 Aegis 自己的增量纪律（`docs/README.md`「工程纪律」），**一次只动一小块**：

```text
1. 实现 internal/runtime                      ← 当前明确的下一步，其它不动
2. Market integration test 换用 rt.Handlers()  验证「原本能跑，改完还能跑」
3. 接第二个 Provider                          在配置层验证 per-agent 多供应商
4. 抽 internal/llm/capabilities               把 deepseek.go 的硬编码变成可查询事实
5. 数据能力层 Capability / Binding + 数据契约  先落一个能力（market.overview）跑通降级链
6. 记忆日志 + 结果结算                        最小形态：Markdown 追加 + pending/settled + 幂等
7. point-in-time 机制                         把 as_of 从「规则」变成「代码约束」
8. Trading Agent + 跨 Agent 字段契约
9. Checkpoint / 恢复
10. 回测（决策质量评估，不做持仓模拟）
```

第 6 步是本报告最想强调的一步：它是 `docs/08` 中「Agent 自成长」从口号变成代码的分界线。
TradingAgents 的实现（`trading_graph.py:346-418` + `agents/utils/memory.py`）已经证明这条路可行，
且给出了三条必须遵守的纪律：**结果未定不结算、反思失败不阻断、按 `as_of` 过滤后注入**。

---

[← 返回研究索引](README.md)
