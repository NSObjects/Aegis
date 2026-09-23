# TradingAgents Agent 体系

本章以 TradingAgents v0.5.0（commit `2d17df8`，2026-09-17）源码为唯一事实来源，拆解其 Agent 体系的五个层面：角色划分、Prompt 构造、结构化输出、Tool 装配、Agent 间信息传递。重点不在 Python 语法，而在可移植到其他技术栈（如 Go + Eino）的机制（mechanism）：谁产生信息、谁消费信息、信息以什么形状在节点之间流动、"自主循环"由谁驱动。行号均相对仓库根目录。

---

## 1. 完整角色清单

### 1.1 工厂函数与状态导出

`tradingagents/agents/__init__.py` 是统一出口，导出 12 个工厂函数、3 个状态类型与 1 个工具函数（`__init__.py:19-37`）。关键约定：**所有 Agent 都是"工厂函数 + 闭包节点"两段式**——工厂接收 `llm` 并返回 `node(state) -> dict`；节点不做副作用，唯一效果是返回局部 dict，由 LangGraph 合并进全局 state。`create_trader` 返回 `functools.partial`（`agents/trader/trader.py:101`），因为 `trader_node(state, name)` 需要额外的 `name` 参数；其余 11 个直接返回节点闭包。

### 1.2 角色总表

| # | 角色（图节点名） | 代码位置 | 分类 | 输入（读 state） | 输出（写 state） | 绑定 tool |
|---|---|---|---|---|---|---|
| 1 | Market Analyst | `agents/analysts/market_analyst.py:12` | Analyst | `messages`, `trade_date`, `instrument_context` | `messages`, `market_report` | `get_stock_data`, `get_indicators`, `get_verified_market_snapshot` |
| 2 | Sentiment Analyst | `agents/analysts/sentiment_analyst.py:55` | Analyst | `company_of_interest`, `trade_date`, `instrument_context`, `messages` | `messages`, `sentiment_report` | 无（数据预取注入 prompt） |
| 3 | News Analyst | `agents/analysts/news_analyst.py:13` | Analyst | `messages`, `trade_date`, `asset_type`, `instrument_context` | `messages`, `news_report` | `get_news`, `get_global_news`, `get_macro_indicators`, `get_prediction_markets` |
| 4 | Fundamentals Analyst | `agents/analysts/fundamentals_analyst.py:13` | Analyst | `messages`, `trade_date`, `instrument_context` | `messages`, `fundamentals_report` | `get_fundamentals`, `get_balance_sheet`, `get_cashflow`, `get_income_statement` |
| 5 | Bull Researcher | `agents/researchers/bull_researcher.py:9` | Researcher | `investment_debate_state`, 四份 report, `instrument_context`, `asset_type` | `investment_debate_state` | 无 |
| 6 | Bear Researcher | `agents/researchers/bear_researcher.py:9` | Researcher | 同上（对手视角） | `investment_debate_state` | 无 |
| 7 | Research Manager | `agents/managers/research_manager.py:17` | Manager | `investment_debate_state.history`, `instrument_context` | `investment_debate_state`, `investment_plan` | 无（结构化输出） |
| 8 | Trader | `agents/trader/trader.py:22` | Trader | `company_of_interest`, `investment_plan`, `market_report`, `instrument_context`, `portfolio_context` | `messages`, `trader_investment_plan`, `sender` | 无（结构化输出） |
| 9 | Aggressive Analyst | `agents/risk_mgmt/aggressive_debator.py:10` | Risk Mgmt | `risk_debate_state`, `trader_investment_plan`, 四份 report, `instrument_context`, `portfolio_context` | `risk_debate_state` | 无 |
| 10 | Conservative Analyst | `agents/risk_mgmt/conservative_debator.py:10` | Risk Mgmt | 同上 | `risk_debate_state` | 无 |
| 11 | Neutral Analyst | `agents/risk_mgmt/neutral_debator.py:10` | Risk Mgmt | 同上 | `risk_debate_state` | 无 |
| 12 | Portfolio Manager | `agents/managers/portfolio_manager.py:26` | Manager | `risk_debate_state`, `investment_plan`, `trader_investment_plan`, `past_context`, `instrument_context`, `portfolio_context` | `risk_debate_state`, `final_trade_decision` | 无（结构化输出） |

三点补充：

- **12 个 Agent 中只有 3 个绑定工具**（Market / News / Fundamentals），其余 9 个是纯生成节点。这是理解整个体系的关键（见第 6 节）。
- `create_social_media_analyst`（`agents/analysts/sentiment_analyst.py:201`）是废弃别名：发 `DeprecationWarning` 后直接返回 `create_sentiment_analyst(llm)`；`agents/analysts/social_media_analyst.py` 整个文件只是 23 行转发 shim。**实际角色数是 12，不是 13**。
- 节点字符串名与工厂的映射集中在 `agents/analysts/analyst_execution.py:20-53`（`ANALYST_NODE_SPECS`）；其中 `social` 这个 key 被刻意保留为 wire value 以兼容旧配置，节点标签则是 `Sentiment Analyst`（`analyst_execution.py:28-38`）。

---

## 2. 角色分类逻辑

### 2.1 五层分工与模型分配

| 层 | 角色 | 核心问题 | 输出形态 | 模型 |
|---|---|---|---|---|
| Analyst | Market / Sentiment / News / Fundamentals | "事实是什么" | 长文 markdown 报告 | `quick_thinking_llm` |
| Researcher | Bull / Bear | "这些事实支持哪个方向" | 逐轮辩论发言 | `quick_thinking_llm` |
| Manager | Research Manager | "辩论哪一方赢了" | 5 档投资计划 | `deep_thinking_llm` |
| Trader | Trader | "具体怎么下单" | 3 档交易提案 | `quick_thinking_llm` |
| Risk Mgmt | Aggressive / Conservative / Neutral | "这笔交易的风险收益比是否可接受" | 逐轮辩论发言 | `quick_thinking_llm` |
| Manager | Portfolio Manager | "最终定档" | 5 档最终决策 | `deep_thinking_llm` |

依据 `graph/setup.py:75-92`：只有 `Research Manager` 与 `Portfolio Manager` 拿到 `deep_thinking_llm`，其余全部是 `quick_thinking_llm`；两者来自同一 provider 的不同模型名（`trading_graph.py:129-143`；默认 `default_config.py:83-84`：`deep_think_llm = "gpt-5.6"`、`quick_think_llm = "gpt-5.6-luna"`）。

### 2.2 生产信息的 Agent vs 对信息做判断的 Agent

**生产信息（Producer）——4 个 Analyst。** 产出是"关于标的的观测"，写入四个平级字段 `market_report` / `sentiment_report` / `news_report` / `fundamentals_report`。它们的 prompt 都带同一条职责边界（`market_analyst.py:66`、`news_analyst.py:41`、`fundamentals_analyst.py:40`、`sentiment_analyst.py:96`）：

```text
Report what your tools support; another agent decides the trade.
```

这条约束把"陈述事实"与"下结论"在 prompt 层切开：Analyst 无权给出 Buy/Sell。**设计意图**是防止四个信息源各自给出方向性结论后，下游被迫做"投票统计"而非论证；**代价**是同一批事实要在 Researcher 层被重新论证一遍，token 成本翻倍。**隐含假设**是 Analyst 不会夹带隐性方向建议——prompt 只能约束措辞，无法约束语义，因此这是"软边界"。

**做判断（Judge）——2 个 Manager + 1 个 Trader。** 不产生新事实，只做三件事：读 Producer 产出（或下游压缩件）、在预设档位集合上选一个值、给出理由。共同特征：都覆盖"冲突不构成 Hold 的理由"，且都用结构化输出。`Research Manager` 判断方向（5 档），`Trader` 判断动作（3 档），`Portfolio Manager` 判断最终定档（5 档）。

**做对抗（Adversary）——5 个 Researcher/Debator。** 不产生新事实，也不产出最终档位，唯一产出是对手方的反驳。Bull/Bear 与 Aggressive/Conservative/Neutral 机制完全同构（"读对手最新发言 + 读全部报告 + 输出发言 + 自增计数器"），区别只在人数（2 vs 3）与轮次上限倍数（`2 * max_debate_rounds` vs `3 * max_risk_discuss_rounds`，`graph/conditional_logic.py:55-58` 与 `:65-68`）。

**为什么这样分层。** 核心意图是把"观点生成"从"观点裁决"中剥离，且裁决者**看不到原始工具数据**：`Research Manager` 的 prompt 只注入 `instrument_context` 与辩论历史（`research_manager.py:21-22, 43-44`），不含四份分析师报告；`Portfolio Manager` 只拿到 plan / trader proposal / risk debate / past_context（`portfolio_manager.py:30-43`）。这是一条"强制抽象层"：裁决者只能对已压缩的论据做判断。**代价**是分析师报告中的具体数字必须先被 Bull/Bear 引用一次才能进入最终决策，存在信息损耗；**收益**是 Manager 的上下文长度与输入方差大幅可控。

**一处方向性不一致（未确认是否为有意设计）。** Trader 是唯一同时读"研究报告摘要"与"原始技术报告"的判断者：`trader.py:34` 显式取 `market_report`，注释（`trader.py:29-33`）说明原因是 investment_plan 消化了辩论但丢掉了精确价格结构。Trader 层刻意打破了 Manager 层的抽象隔离，因为入场价/止损位必须锚定真实价格结构（ATR、支撑阻力）。

---

## 3. Prompt 结构分析

### 3.1 三类 Prompt 构造模式

**模式 A：共享外壳 + 局部 `partial`（Market / News / Fundamentals）。** 外壳含 5 条协作契约与 3 个插槽：

```python
# tradingagents/agents/analysts/market_analyst.py:58-80（同一外壳在 news_analyst.py:35-44、fundamentals_analyst.py:34-43 逐字重复）
prompt = ChatPromptTemplate.from_messages([
    ("system",
     "You are a helpful AI assistant, collaborating with other assistants."
     " If you are unable to fully answer, that's OK; another assistant with different tools"
     " will help where you left off. Execute what you can to make progress."
     " Report what your tools support; another agent decides the trade."
     " You have access to the following tools: {tool_names}."
     " Today's date is {current_date}; treat it as 'now' for all analysis and tool-call date ranges. {instrument_context}\n"
     "{system_message}"),
    MessagesPlaceholder(variable_name="messages"),
])
prompt = prompt.partial(tool_names=", ".join([tool.name for tool in tools]))
prompt = prompt.partial(system_message=system_message).partial(current_date=current_date)
chain = prompt.partial(instrument_context=instrument_context) | llm.bind_tools(tools)
```

未抽公共常量，是复制粘贴式模板。它做了四件机制性的事：（1）**动态注入工具清单**——`", ".join([tool.name for tool in tools])`（`market_analyst.py:76`）从真实 tool 对象反射，保证 prompt 与绑定集合永不漂移；（2）**动态注入日期**——`current_date = state["trade_date"]`（`:15`），并声明"treat it as 'now' for all analysis and tool-call date ranges"；（3）**动态注入标的身份**（见 3.4）；（4）**注入 `MessagesPlaceholder`**——分析师是唯一读 `state["messages"]` 的角色类别。

**模式 B：纯 f-string 单段拼接（Bull/Bear、3 个 Risk Debator、Research Manager、Portfolio Manager、Trader）。** 无 `ChatPromptTemplate`，直接把 state 字段插值进大字符串（如 `bull_researcher.py:31-49`），结构固定为五段式：角色宣言 → 关注点清单 → 资源清单 → 对手最新发言 → 行动指令。

**模式 C：预取数据 + 结构化块 + 方法论清单（Sentiment Analyst）。** `_build_system_message`（`sentiment_analyst.py:133-195`）把三份预取数据包在 XML 风格分隔符里：

```text
### News headlines — Yahoo Finance, past 7 days
Institutional framing. Fact-driven, slower-moving signal.
<start_of_news>
{news_block}
<end_of_news>
```

（`sentiment_analyst.py:147-152`；StockTwits 与 Reddit 同构，见 `:154-166`）

用 `<start_of_x>` / `<end_of_x>` 而非 markdown 代码块，是为了给模型一个**不可与正文混淆的硬边界**——这些数据自身可能含 markdown。随后是 8 条"如何分析这些数据"的方法论（`:168-184`）与字段说明（`:186-193`）。字段说明与 `schemas.py` 的 `Field(description=...)` 内容重复（对比 `sentiment_analyst.py:190-193` 与 `schemas.py:322-349`），属冗余强化：schema 是给 provider 的解析约束，prompt 里再写一遍是给模型的显式提醒。

### 3.2 代表片段一：Market Analyst 的证据与不确定性约束

```text
Before writing the final report, call get_verified_market_snapshot for this ticker and the current date, and treat it as the source of truth for any exact OHLCV, price-level, or indicator-value claim. If another tool's output conflicts with the verified snapshot, flag the discrepancy rather than inventing a reconciled number. Do not claim historical validation, support/resistance bounces, or exact percentage moves unless they are directly supported by tool output with concrete dates and prices.
```

（`tradingagents/agents/analysts/market_analyst.py:51`）

- **指定 truth source 并强制调用顺序**："call ... Before writing the final report"把某个 tool 从"可选项"提升为"必经步骤"。这也解释了 `get_verified_market_snapshot` 为何同时出现在 `bind_tools` 集合（`:21`）与 `ToolNode`（`trading_graph.py:236`）：prompt 要求调用，图必须能执行。
- **冲突时的行为规定**："flag the discrepancy rather than inventing a reconciled number"——面对两个来源矛盾，要求**报告矛盾而非调和**。可移植机制：同一事实有多个来源时，禁止模型自行平均/折中，改为显式暴露分歧。**负向约束的形式**是 "Do not claim X unless Y" 而非 "Be accurate"——后者不可验证，前者给出可检查的触发条件。

同段的另一条约束同时覆盖指标冗余与参数格式（`market_analyst.py:49`）：

```text
- Select indicators that provide diverse and complementary information. Avoid redundancy (e.g., do not select both rsi and stochrsi). Also briefly explain why they are suitable for the given market context. When you tool call, please use the exact name of the indicators provided above as they are defined parameters, otherwise your call will fail.
```

最后一句是**工具参数层的负向约束**：指标名是枚举型参数，用错名字直接导致 tool call 失败。这类约束无法用 schema 表达（`get_indicators` 的 `indicator` 参数类型是 `str`，`technical_indicators_tools.py:13`），只能在 prompt 里用自然语言补偿。

### 3.3 代表片段二：Portfolio Manager 的决策纪律

```text
Ground every conclusion in specific evidence from the analysts. The risk debate always contains conflicting stances; deciding which is stronger is the job, so conflict alone is not a reason to Hold. Commit to the stronger case, sized by how decisively it wins. Choose Hold only when the evidence is still balanced after that weighing, or too thin to support a call; do not force a direction to appear decisive. Weigh the analysts on their merits, independent of speaking order.
```

（`tradingagents/agents/managers/portfolio_manager.py:69`）

- **掐死"折中逃避"路径**："conflict alone is not a reason to Hold"直指 LLM 在对抗信息下的默认行为——退到中性档；与"do not force a direction to appear decisive"形成双向夹逼。
- **消除位置偏差**："independent of speaking order"——辩论是顺序发言（Aggressive → Conservative → Neutral），最后发言者天然占优。这是**针对图拓扑副作用的补偿**：拓扑决定顺序，prompt 负责声明顺序不含信息。
- **同一约束三处出现**：`portfolio_manager.py:69`、`research_manager.py:39` 的等价表述、`schemas.py:107-111` 与 `:233-238` 的 `Field(description=...)`。

### 3.4 代表片段三：动态上下文注入的反幻觉设计

`instrument_context` 注入**全部 12 个 Agent**，由 `build_instrument_context`（`agent_utils.py:138-198`）生成：

```python
# tradingagents/agents/utils/agent_utils.py:158-191
context = (
    f"The {instrument_label} to analyze is `{ticker}`. "
    "Use this exact ticker in every tool call, report, and recommendation, "
    "preserving any exchange suffix (e.g. `.TO`, `.L`, `.HK`, `.T`, `-USD`)."
)
...
context += (
    f" Resolved identity: {'; '.join(details)}. "
    "Do not substitute a different company or ticker unless a tool "
    "result explicitly disproves this resolved identity."
)
today = get_current_date()
if curr_date and str(curr_date) < today:
    context += (
        f" This identity is how the vendor describes the instrument today "
        f"({today}), not necessarily on {curr_date}: a name or "
        f"classification changed since then would read as the current one."
    )
```

- **确定性身份注入**：`resolve_instrument_identity`（`:94-135`）在 run 启动时做一次 yfinance 查询，把 company name / sector / industry / exchange 写进 state。理由见其 docstring（`:98-102`）："without a ground-truth name, the market analyst would pattern-match the price action to a narrative and invent an identity that then cascaded through every downstream agent"。这是**用一次确定性 I/O 消除一类系统性幻觉**，而非靠 prompt 说"不要编造公司名"；查询结果带 `functools.lru_cache`（`:94`），每 ticker 每进程至多一次。
- **失效开放（fail open）**：查询失败返回 `{}`，调用方退回 ticker-only context（`:103-107`），run 不因元数据缺失而中断。
- **时序诚实**：身份数据没有历史版本，因此 `curr_date < today` 时 prompt 显式声明"这是今天的描述，未必是分析日的描述"（`:185-191`）。配套的 `withhold_live_profile`（`dataflows/date_window.py:112` 起）在历史 run 中直接**拒绝提供**公司画像并返回解释文本——"宁可缺数据也不泄漏未来信息"。

### 3.5 空值语义：用显式标记替代空白

三个 helper 处理同一类问题——**空白字段会被模型填满**：

| Helper | 位置 | 处理 | 理由（源码注释） |
|---|---|---|---|
| `opponent_argument_or_opening` | `agent_utils.py:70-81` | 空对手发言 → `(The X has not spoken yet — open the debate with your own case.)` | "interpolating it into a 'refute the opponent' prompt makes the model fabricate the other side's position" |
| `report_or_absent` | `agent_utils.py:219-230` | 缺报告 → `(No X report in this run: it is not available, not an empty finding.)` | "presents an absence as a blank finding, and the reading agent fills it in from nothing" |
| `get_portfolio_context_from_state` | `agent_utils.py:233-247` | 无持仓输入 → 明说"不知道持仓，不要假设空仓" | "the agents would otherwise size as if the caller held nothing, which is a claim about an account we were never told about" |

这是最容易移植、也最容易被忽略的机制：**"字段为空"与"字段为空字符串"不是同一件事**，必须在 prompt 层显式区分。

### 3.6 负向约束、证据要求、置信度

**负向约束清单（"不要做什么"）：**

| 约束 | 位置 |
|---|---|
| `Use only the evidence provided in this prompt. Do not call external tools or search the web; if something is missing, say so explicitly.`（`NO_EXTERNAL_TOOLS`） | `utils/structured.py:36-39`，注入 RM/Trader/PM/Sentiment 四处 |
| `Do not claim historical validation, support/resistance bounces, or exact percentage moves unless ...` | `analysts/market_analyst.py:51` |
| `Avoid redundancy (e.g., do not select both rsi and stochrsi)` | `analysts/market_analyst.py:49` |
| `do not infer engagement`（Reddit 无投票/评论数） | `analysts/sentiment_analyst.py:174` |
| `Past sentiment is not predictive.` | `analysts/sentiment_analyst.py:184` |
| `entry price ... never a percentage or a range` | `trader/trader.py:58-61`、`schemas.py:164-179` |
| `Do not substitute a different company or ticker unless a tool result explicitly disproves this resolved identity.` | `utils/agent_utils.py:182-183` |
| `do not assume a flat book` | `utils/agent_utils.py:244-246` |
| `do not manufacture a direction to appear decisive` / `do not force a direction` | `managers/research_manager.py:39`、`managers/portfolio_manager.py:69` |
| `Output conversationally as if you are speaking without any special formatting.` | `risk_mgmt/{aggressive,conservative,neutral}_debator.py:46`（三处逐字重复） |

`NO_EXTERNAL_TOOLS` 的存在理由写在其定义处注释（`structured.py:31-35`）：schema-only 结构化输出在底层只绑定一个 tool（即 schema 本身），模型若去调搜索工具会产生未知 tool call，导致整个结构化尝试被丢弃并退回自由文本。**这是"结构化输出与工具调用互斥"的显式补偿**——不靠绑定机制保证，而在 prompt 里明说。

**证据引用要求：** 分析师统一用 "specific, actionable insights with supporting evidence"（`market_analyst.py:53`、`news_analyst.py:28`、`fundamentals_analyst.py:26`）；辩论者用 "with specific data and sound reasoning"（`bull_researcher.py:37`、`bear_researcher.py:38`）；PM 用 "Ground every conclusion in specific evidence"（`portfolio_manager.py:69`）。最强的一条在 `SentimentReport.narrative` 的字段描述里，逐项规定叙述结构并要求引用计数（`schemas.py:350-363`："cite message counts, ratios, notable posts"）。

**置信度：只有 Sentiment Analyst 有该字段。**

```python
# tradingagents/agents/schemas.py:342-349
confidence: Literal["low", "medium", "high"] = Field(
    description=(
        "Confidence in the assessment based on data quality and sample size. "
        "Use 'low' when one or more sources returned a placeholder or fewer "
        "than 5 data points; 'medium' when data is present but sparse; "
        "'high' when all three sources returned substantive data."
    ),
)
```

定义方式是**数据质量与样本量的函数**，并给了可判定阈值（"fewer than 5 data points"）。`ResearchPlan`、`TraderProposal`、`PortfolioDecision` 三个面向决策的 schema **都没有置信度字段**——"方向与仓位"不带自评可信度。权衡在于：置信度被当作**数据可靠性元数据**（供下游加权），而非**决策者的自我评估**（容易系统性偏高）。若 Aegis 需要"决策 + 置信度"成对输出，这套源码不能直接照搬。

---

## 4. 结构化输出机制

### 4.1 设计定位

`schemas.py` 头部注释（`:1-17`）规定了整个机制的边界：

```text
The framework's primary artifact is still prose ... Structured output is layered onto the
three decision-making agents (Research Manager, Trader, Portfolio Manager) so that:
- Their outputs follow consistent section headers across runs and providers
- Each provider's native structured-output mode is used (json_schema for OpenAI/xAI,
  response_schema for Gemini, tool-use for Anthropic)
- Schema field descriptions become the model's output instructions, freeing
  the prompt body to focus on context and the rating-scale guidance
- A render helper turns the parsed Pydantic instance back into the same
  markdown shape the rest of the system already consumes
```

四条要点构成闭环：**结构化只用于"档位选择"这一窄场景 → 用 provider 原生模式保证跨模型一致 → 字段描述承担输出指令 → 渲染回 markdown 以保证下游零改动**。第一条"primary artifact is still prose"说明结构化输出没有取代文本，而是叠加在文本之上。

### 4.2 Schema 清单与字段含义

**两个档位枚举：**

```python
# tradingagents/agents/schemas.py:66-73
class PortfolioRating(str, Enum):
    """5-tier rating used by the Research Manager and Portfolio Manager."""
    BUY = "Buy"; OVERWEIGHT = "Overweight"; HOLD = "Hold"
    UNDERWEIGHT = "Underweight"; SELL = "Sell"

# tradingagents/agents/schemas.py:76-87
class TraderAction(str, Enum):
    """3-tier transaction direction used by the Trader."""
    BUY = "Buy"; HOLD = "Hold"; SELL = "Sell"
```

`TraderAction` 的 docstring 给出档位差异的理由：Trader 只决定"本轮执行 Buy / Sell / 按兵不动"，**仓位大小与相对基准的超配/低配留给 Portfolio Manager**。这是执行层的认知负荷削减——Trader 不需要理解 Overweight 与 Buy 的语义差别（该映射关系由 Trader 的 prompt 显式给出，见 `trader.py:78-80`）。

**`ResearchPlan`（`schemas.py:95-127`）**——Research Manager 的产出：

| 字段 | 类型 | 含义 |
|---|---|---|
| `recommendation` | `PortfolioRating` | 方向档位；字段描述内含"冲突不是 Hold 的理由"的纪律 |
| `rationale` | `str` | 双方论点的会话式总结，结尾说明哪个论点决定了结论 |
| `strategic_actions` | `str` | 给 Trader 的具体步骤，含相对标准仓位的 sizing 指引 |

`rationale` 的描述里有一句关键边界声明（`:123-125`）："The research team does not see the caller's holdings; the trader and portfolio manager apply the actual position."——研究层被明确告知自己**看不到持仓**，因此不许给出绝对仓位。

**`TraderProposal`（`schemas.py:146-188`）**：

| 字段 | 类型 | 含义 |
|---|---|---|
| `action` | `TraderAction` | Buy / Hold / Sell |
| `reasoning` | `str` | 2–4 句，锚定分析师报告与研究计划 |
| `entry_price` | `float \| None` | 绝对价格，明确禁止百分比与区间 |
| `stop_loss` | `float \| None` | 绝对价格，禁止百分比；要求把百分比距离换算成价位，或省略 |
| `position_sizing` | `str \| None` | 如 `'5% of portfolio'` |

**`PortfolioDecision`（`schemas.py:221-265`）**：

| 字段 | 类型 | 含义 |
|---|---|---|
| `rating` | `PortfolioRating` | 最终档位 |
| `executive_summary` | `str` | 2–4 句行动计划（入场策略、仓位、关键风险位、时间跨度） |
| `investment_thesis` | `str` | 详细论证，锚定辩论中的具体证据；若 prompt 有历史教训则需纳入 |
| `price_target` | `float \| None` | 目标价 |
| `time_horizon` | `str \| None` | 如 `'3-6 months'` |

该类 docstring（`:221-228`）明确：**所有字段由一次主 LLM 调用填写，无需单独抽取轮次**；字段描述即输出指令。

**`SentimentBand` + `SentimentReport`（`schemas.py:296-363`）**：

| 字段 | 类型 | 含义 |
|---|---|---|
| `overall_band` | `SentimentBand`（6 档） | Bullish / Mildly Bullish / Neutral / Mixed / Mildly Bearish / Bearish |
| `overall_score` | `float`，`ge=0.0, le=10.0` | 0–10 强度分；描述给出与 band 的分段对应（"Only the 0–10 bounds are enforced"） |
| `confidence` | `Literal["low","medium","high"]` | 数据质量与样本量 |
| `narrative` | `str` | 分五段的长文，含一张 markdown 汇总表 |

6 档而非 5 档的理由在枚举 docstring（`:299-301`）："Six tiers keep the signal granular enough to be actionable while remaining small enough for every provider to map reliably from its JSON output."

**唯一自定义 validator：`_coerce_optional_float`（`schemas.py:33-58`）**，挂在 `entry_price`/`stop_loss`（`:185-188`）与 `price_target`（`:262-265`）上：

```python
# tradingagents/agents/schemas.py:49-58
def _coerce_optional_float(value):
    if not isinstance(value, str):
        return value
    text = value.strip()
    if text.lower() in _NULLISH_FLOAT or text.endswith("%"):
        return None
    cleaned = text.replace(",", "").lstrip("$€£¥").strip()
    try:
        return float(cleaned)
    except ValueError:
        return None
```

占位词集合见 `:30`（`"none"`, `"n/a"`, `"null"`, `"tbd"`, `"unknown"` 等）。注释给出"为什么百分比必须丢弃而非解析"的理由（`:39-42`）：把 `"15%"` 读成 15 会在 600 美元的股票上把止损设在 15 美元。这是**"单字段降级优于整条决策作废"**的取舍——宁可一个价位为空，也不让整个 proposal 因一个字段丢失全部正确字段。

### 4.3 与模型的绑定方式

绑定发生在**工厂创建时**，而非调用时：

```python
# tradingagents/agents/utils/structured.py:42-56
def bind_structured(llm: Any, schema: type[T], agent_name: str) -> Any | None:
    try:
        return llm.with_structured_output(schema)
    except (NotImplementedError, AttributeError) as exc:
        logger.warning("%s: provider does not support with_structured_output (%s); "
                       "falling back to free-text generation", agent_name, exc)
        return None
```

调用路径有**两层降级**（`structured.py:59-89`）：（1）创建期 provider 不支持 → `structured_llm = None`，此后每次走自由文本；（2）调用期结构化调用抛异常或返回 `None`（注释 `:77-80`：thinking 模型可能用纯文本作答而不调 tool，解析器无结果）→ 记 warning，用同一 prompt 重试一次普通 `llm.invoke`：

```python
# tradingagents/agents/utils/structured.py:73-89
    if structured_llm is not None:
        try:
            result = structured_llm.invoke(prompt)
            if result is None:
                raise ValueError("structured output returned no parsed result")
            return render(result)
        except Exception as exc:
            logger.warning("%s: structured-output invocation failed (%s); retrying once as free text",
                           agent_name, exc)
    response = plain_llm.invoke(prompt)
    return response.content
```

机制要点：**同一段 prompt 同时服务两条路径**（`:66-71`）。因此 prompt 正文必须自带完整的输出段落说明（如 `portfolio_manager.py:71-77` 的 `## Output` 小节），不能只依赖 schema 字段描述——降级路径下模型看不到 schema。调用侧另有两处实现细节：Sentiment Analyst 先把模板 `format_messages` 成具体消息列表再交给两条路径（`sentiment_analyst.py:112-123`）；`structured_llm` 在工厂里只绑定一次并被所有调用复用（`research_manager.py:18`、`trader.py:23`、`portfolio_manager.py:27`、`sentiment_analyst.py:63`）。

### 4.4 Render 层：结构化 → markdown 的反向通道

四个 render 函数把 Pydantic 实例转回 markdown：`render_research_plan`（`schemas.py:130-138`）、`render_trader_proposal`（`:191-213`）、`render_pm_decision`（`:268-288`）、`render_sentiment_report`（`:366-379`）。`render_pm_decision` 生成 `**Rating** / **Executive Summary** / **Investment Thesis** / **Price Target** / **Time Horizon**` 五段。两个可移植细节：（1）**字段缺失也成行**——可选字段缺失时渲染为 `not provided` 而不是省略该行，注释解释（`:283-284`）："a missing line reads as a field nobody asked for, so a reader cannot tell 'no target' from 'target not reported'"（与 3.5 节的空值语义同一原则）；（2）**保留向后兼容的字符串契约**——`render_trader_proposal` 结尾固定追加 `FINAL TRANSACTION PROPOSAL: **BUY**`（`schemas.py:209-212`），因为外部代码 grep 这一行，结构化改造没有破坏既有接口。

### 4.5 `utils/rating.py` 解决什么问题

它解决**"结构化输出失败或自由文本降级后，如何仍确定性地取回档位"**。`RATINGS_5_TIER`（`rating.py:23-25`）是 `("Buy", "Overweight", "Hold", "Underweight", "Sell")`，另有 `RATING_REVIEW = "REVIEW"`（`:31`）。

`extract_rating`（`:49-78`）在 NFKC 归一化文本上做两遍扫描（使全角 `Rating：Overweight` 与 ASCII 等效）：

1. 找显式标签 `Rating: X`，正则容忍 markdown 粗体与各种破折号（`_RATING_LABEL_RE`，`:37-38`），并**取最后一个**匹配——"a decision states its rating after discussing the alternatives"；同时用 `_RATING_SCALE_RE`（`:41`）跳过模型复述的评级量表行。
2. 无标签时扫描全文独立档位词，**仅当恰好命中一个不同档位时返回**：`named = {m.group(1).capitalize() for m in _RATING_WORD_RE.finditer(norm)}; return named.pop() if len(named) == 1 else None`（`:77-78`）。理由在注释（`:74-77`）：一段先否定 Buy 再结论 Underweight 的文字，若按"第一个出现的档位"解析会被读成 Buy。

`REVIEW` 哨兵的设计意图是解析失败**不降级为 Hold**——注释（`:12-14`）："a decision nobody can read is not a Hold, and a Hold recorded in its place is quoted back to the next run as a call that was never made"。调用方 `SignalProcessor.process_signal`（`graph/signal_processing.py:29-38`）返回 `REVIEW`，并要求使用方先用 `is_review()` 判断（`rating.py:91-93`）。这是**"宁可显式失败也不静默给出可交易信号"**。因此档位有**双路径冗余**：结构化输出保证主路径确定性，正则解析保证降级路径可解析。该 5 档词表被四处共享（Research Manager、Portfolio Manager、signal processor、memory log 条目标签），集中定义以避免漂移（`:1-15`）。

---

## 5. Tool 的组织方式

### 5.1 定义风格

全部 12 个 tool 使用同一种风格：`langchain_core.tools.tool` 装饰器 + `Annotated[type, "description"]` 参数描述 + docstring 功能说明。

```python
# tradingagents/agents/utils/core_stock_tools.py:10-28
@tool
def get_stock_data(
    symbol: Annotated[str, "ticker symbol of the company"],
    start_date: Annotated[str, "Start date in yyyy-mm-dd format"],
    end_date: Annotated[str, "End date in yyyy-mm-dd format"],
    trade_date: Annotated[str, InjectedState("trade_date")] = "",
) -> str:
    start_date, end_date = as_of_window(start_date, end_date, trade_date)
    return route_to_vendor("get_stock_data", symbol, start_date, end_date)
```

三个结构性约定：

1. **一切 tool 返回 `str`**。没有结构化返回值、没有对象；返回格式化好的 markdown/表格文本，直接进模型上下文。
2. **`trade_date` 是注入参数而非模型参数**。`Annotated[str, InjectedState("trade_date")]`（来自 `langgraph.prebuilt`）表示值由图从 state 注入，模型看不到也不需要传——这是**"模型不可篡改的运行时上下文"**机制。
3. **业务逻辑不在 tool 里，而在 vendor 层**。每个 tool 体都是一行 `route_to_vendor(<method>, ...)`（`dataflows/interface.py:177`）；供应商链是"显式配置即链路，不做隐式兜底"（`interface.py:188-202`）。

**点时空窗保护也在 tool 层**：`as_of(requested, trade_date)`（`dataflows/date_window.py:73-82`）返回"模型给的日子，但绝不晚于 run 的日子"——`return requested if parsed is not None and parsed <= _parse(trade_date) else trade_date`。这是**"不信任模型传入的日期"**的机制化实现：即使模型传了今天，也会被 backtest 的 `trade_date` 覆盖。`as_of_window`（`:85-96`）对区间做同样处理，并保持窗口长度不变。Aegis 若做历史回测，这一层是必需的。

### 5.2 按 Agent 分组装配

装配有**两个互相独立的位置**，必须同时改才不会出错：

- **绑定集合**：agent 工厂内的 `tools = [...]` 与 `llm.bind_tools(tools)`，决定**模型能看到什么**。
- **执行集合**：`_create_tool_nodes()`（`graph/trading_graph.py:224-264`）为每个 `ToolNode` 列出的工具，决定**图能执行什么**。

| key | 绑定处 | ToolNode 执行集合 | 是否一致 |
|---|---|---|---|
| `market` | `market_analyst.py:18-22` | `trading_graph.py:227-238`：`get_stock_data`, `get_indicators`, `get_verified_market_snapshot` | 一致 |
| `news` | `news_analyst.py:20-25` | `trading_graph.py:245-254`：`get_news`, `get_global_news`, `get_insider_transactions`, `get_macro_indicators`, `get_prediction_markets` | **不一致**：`get_insider_transactions` 在执行集合但未绑定给模型 |
| `fundamentals` | `fundamentals_analyst.py:18-23` | `trading_graph.py:255-263`：`get_fundamentals`, `get_balance_sheet`, `get_cashflow`, `get_income_statement` | 一致 |
| `social` | 无（sentiment 不 `bind_tools`） | `trading_graph.py:239-244`：`get_news` | **不一致**：见 6.3 |

`get_insider_transactions` 在 `agent_utils.__all__`（`agent_utils.py:38`）中作为公共 API 导出，也被注册进 news 的 `ToolNode`，但没有任何 agent 的 `bind_tools` 集合包含它——模型不知道它存在，永远不会产生对应 tool call。这是**回归残留**（只注册不绑定）。

### 5.3 Tool 清单

| Tool 名 | 作用 | 参数（模型可见部分） | 绑定给 | 定义处 |
|---|---|---|---|---|
| `get_stock_data` | OHLCV 行情序列 | `symbol`, `start_date`, `end_date` | Market Analyst | `core_stock_tools.py:10-28` |
| `get_indicators` | 单个技术指标序列；支持逗号分隔多指标并在内部拆分（`:32-38`），单个指标失败不中断其余 | `symbol`, `indicator`, `curr_date`, `look_back_days=30` | Market Analyst | `technical_indicators_tools.py:10-39` |
| `get_verified_market_snapshot` | 确定性验证快照：截至 `curr_date` 的最新 OHLCV 行 + 常用指标 + 近期收盘，作为精确数值主张的事实来源 | `symbol`, `curr_date`, `look_back_days=30` | Market Analyst | `market_data_validation_tools.py:10-26`；默认指标集 `dataflows/market_data_validator.py:21-25` |
| `get_fundamentals` | 综合基本面报告 | `ticker`, `curr_date` | Fundamentals Analyst | `fundamental_data_tools.py:10-25` |
| `get_balance_sheet` | 资产负债表 | `ticker`, `freq="quarterly"`, `curr_date` | Fundamentals Analyst | `fundamental_data_tools.py:28-45` |
| `get_cashflow` | 现金流量表 | 同上 | Fundamentals Analyst | `fundamental_data_tools.py:48-65` |
| `get_income_statement` | 利润表 | 同上 | Fundamentals Analyst | `fundamental_data_tools.py:68-85` |
| `get_news` | 个股新闻 | `ticker`, `start_date`, `end_date` | News Analyst；Sentiment Analyst 以 `get_news.func(...)` 直接调用 | `news_data_tools.py:10-28` |
| `get_global_news` | 宏观新闻 | `curr_date`, `look_back_days=None`, `limit=None`（省略则用配置默认值） | News Analyst | `news_data_tools.py:30-51` |
| `get_insider_transactions` | 内部人交易 | `ticker` | **无 agent 绑定**（仅注册于 news ToolNode） | `news_data_tools.py:53-66` |
| `get_macro_indicators` | FRED 宏观序列（政策利率、国债收益率、通胀、就业、增长）；接受友好别名（`cpi`/`core_pce`/`fed_funds_rate`/`yield_curve`…）或原始 series ID | `indicator`, `curr_date`, `look_back_days=None` | News Analyst | `macro_data_tools.py:10-39` |
| `get_prediction_markets` | Polymarket 对前瞻事件的隐含概率 | `topic`, `limit=None` | News Analyst | `prediction_markets_tools.py:9-33` |

**分组逻辑是按分析视角，而不是按数据源。** `get_news` 同时出现在 news 分组（模型可见）与 social 分组（ToolNode），并被 Sentiment Analyst 绕过 tool 协议直接调用——同一能力出现三处，是"视角分组"而非"能力分组"的必然结果。**代价**是新增数据源时要判断它属于哪个视角，且同一工具易被重复注册（`get_insider_transactions` 的悬空即此类）。---

## 6. 关键设计点：自主 ReAct 循环还是单次 LLM 调用

### 6.1 结论

**Agent 本身不做循环，全部是单次 LLM 调用；"ReAct 循环"由图拓扑实现，且只有 3 个 Analyst 参与。** 拆成三个可验证的子命题。

**（a）agent 节点内没有 `while` / 重试 / 循环。**

```python
# tradingagents/agents/analysts/market_analyst.py:80-92
        chain = prompt | llm.bind_tools(tools)
        result = chain.invoke(state["messages"])
        report = ""
        if len(result.tool_calls) == 0:
            report = result.content
        return {"messages": [result], "market_report": report}
```

节点内只有一次 `invoke`。工具执行不在节点里——节点只是把带 `tool_calls` 的 AIMessage 写回 `messages` 并立刻返回。

**（b）循环由图的条件边 + 独立 ToolNode 驱动。**

```python
# tradingagents/graph/conditional_logic.py:14-20
    def should_continue_market(self, state: AgentState):
        last_message = state["messages"][-1]
        if last_message.tool_calls:
            return "tools_market"
        return "Msg Clear Market"

# tradingagents/graph/setup.py:124-129
            workflow.add_conditional_edges(
                current_analyst,
                getattr(self.conditional_logic, f"should_continue_{spec.key}"),
                [current_tools, current_clear],
            )
            workflow.add_edge(current_tools, current_analyst)
```

形成环：`Analyst → (有 tool_calls) → ToolNode → Analyst → ... → (无 tool_calls) → Msg Clear → 下一个 Analyst`。**这是标准的图级 ReAct 展开**：模型的"要不要调工具"决策与"真的去调"被拆成两个节点，循环由边而非 agent 代码控制。

一个易漏的实现细节：`market_analyst.py:86-87` 的 `if len(result.tool_calls) == 0` 意味着**中间轮次会把 `market_report` 写成空字符串**，只有终止轮才写入真实内容。因此该字段语义是"循环终止时的最终发言"，不是累积输出。

**（c）循环没有 per-agent 轮次上限，唯一边界是全局递归限制。** 仓库内未发现按分析师的 tool 轮次计数器；只有 `max_recur_limit`（`propagation.py:14,80`，默认 100，`default_config.py:118`），它被直接放进图调用配置：`config = {"recursion_limit": self.max_recur_limit}`（`propagation.py:80`）。**权衡**：这给了模型较大自主空间（可按需多轮取数），但代价是**单个分析师的失控循环会消耗整条 run 的递归预算**——该预算全图共享，覆盖 4 个分析师 + 两轮辩论 + 判断。Aegis 若移植，建议给每个 ReAct 环单独配轮次上限，而不是只依赖全局递归深度。

### 6.2 哪些 Agent 不同

| Agent | 绑定 tool | 参与图级循环 | 调用路径 | 依据 |
|---|---|---|---|---|
| Market Analyst | 是 | 是 | `prompt \| llm.bind_tools(tools)` | `market_analyst.py:80` |
| News Analyst | 是 | 是 | 同上 | `news_analyst.py:55` |
| Fundamentals Analyst | 是 | 是 | 同上 | `fundamentals_analyst.py:54` |
| Sentiment Analyst | **否** | **否** | `prompt.format_messages(...)` → `invoke_structured_or_freetext` | `sentiment_analyst.py:113-123` |
| Bull / Bear Researcher | 否 | 否 | `llm.invoke(prompt)`，prompt 为字符串 | `bull_researcher.py:51`、`bear_researcher.py:53` |
| Aggressive / Conservative / Neutral | 否 | 否 | 同上 | `*_debator.py:48`（三处） |
| Research Manager | 否 | 否 | `structured_llm.invoke(prompt)` | `research_manager.py:56-62` |
| Trader | 否 | 否 | `structured_llm.invoke(messages)` | `trader.py:87-93` |
| Portfolio Manager | 否 | 否 | `structured_llm.invoke(prompt)` | `portfolio_manager.py:81-87` |

即 **9/12 个 Agent 是严格单次调用**。它们之所以"多轮"，是因为**图把它们串成了多轮**：辩论轮次由 `count` 计数器 + 条件边实现，`should_continue_debate` 在 `count >= 2 * max_debate_rounds` 时转向 Research Manager（`conditional_logic.py:52-61`），`should_continue_risk_analysis` 在 `count >= 3 * max_risk_discuss_rounds` 时转向 Portfolio Manager（`:63-73`）。默认两个上限都是 1（`default_config.py:116-117`），即 Bull/Bear 各发言一次、三个风险角色各发言一次。

### 6.3 一个反例：Sentiment Analyst 明确放弃了工具循环

`sentiment_analyst.py` 的模块 docstring 记录了这次改造的动机，是本仓库最有价值的工程注释之一：

```text
Previously named ``social_media_analyst``. Renamed and redesigned because
the old version had a prompt that demanded social-media analysis but the
only tool available was Yahoo Finance news — which led LLMs to fabricate
Reddit/X/StockTwits content under prompt pressure (verified live).
...
The agent does not use tool-calling; the data is in the prompt from turn 0.
```

（`sentiment_analyst.py:1-25`）实现上先做三次预取（`:74-80`）：`news_block = get_news.func(ticker, start_date, end_date)`、`stocktwits_block = fetch_stocktwits_messages(ticker, limit=30, start_date=..., end_date=...)`、`reddit_block = fetch_reddit_posts(ticker, start_date=..., end_date=...)`。`get_news.func(...)` 绕过 `@tool` 包装直接调用底层 Python 函数。注释还解释了为何连"工具日期范围"这类措辞都要删除（`:97-99`）："No tool-calling here: the data is pre-fetched into the prompt, so tool-range wording would only invite a hallucinated tool call (#1130)"；因此 prompt 外壳替换为 `NO_EXTERNAL_TOOLS`（`:101`）。

**两种模式的机制对比：**

| | 工具循环式（Market/News/Fundamentals） | 预取注入式（Sentiment） |
|---|---|---|
| 数据何时确定 | 模型决定，运行时 | run 开始前，代码决定 |
| 模型自由度 | 高（可换参数、多轮） | 低（数据固定） |
| 失败面 | 参数错、调不存在的东西、循环失控 | 无中生有引用未提供的内容 |
| 可测试性 | 低（依赖模型决策）；时延为多轮 LLM + 多轮工具 | 高（输入是死数据）；时延为 1 次 LLM + 预取 |

源码选择两种都保留，按数据源特性分配：行情/财务/新闻是"可以按参数精确查询"的，走工具；社交情绪是"只有固定近期窗口、没有查询参数空间"的，走预取。**这是一个可移植判据**：如果数据源本身没有可参数化的查询维度，工具循环只会增加失败面而不增加信息量。由此也解释了 5.2 节的悬空节点：`social` 的 `ToolNode` 仍然注册（`trading_graph.py:239-244`），`should_continue_social` 仍然检查 `tool_calls`（`conditional_logic.py:22-34`），但 Sentiment Analyst 从不绑定工具、返回的 AIMessage 也没有 `tool_calls`，因此该分支在 v0.5.0 中**永远不会被路由到**——这是从工具循环式改为预取注入式后遗留的图残迹。

---

## 7. Agent 之间的信息传递

### 7.1 state 是唯一的通信总线

`AgentState`（`agents/utils/agent_states.py:47-77`）继承 `MessagesState`（带一个 `messages` 通道），其余全部是显式命名字段；初始值由 `Propagator.create_initial_state`（`graph/propagation.py:18-71`）一次性填好。

**关键观察：12 个 Agent 中有 9 个完全不读 `state["messages"]`。** 只有 4 个 Analyst 通过 `MessagesPlaceholder(variable_name="messages")` 读它（`market_analyst.py:71`、`news_analyst.py:46`、`fundamentals_analyst.py:45`、`sentiment_analyst.py:104,115`）；辩论者、Manager、Trader 的 prompt 全部是本地字符串拼接，只读显式命名字段。因此**多 Agent 协作在此体系中不是共享自然语言历史，而是结构化字段的读写**。`messages` 通道只服务两件事：承载分析师的工具调用往返，以及承载初始的 human 消息（`propagation.py:36`）。

每个分析师跑完后有一个 `Msg Clear X` 节点清空 `messages` 并塞入一个锚定标的与日期的占位消息（`create_msg_delete`，`agent_utils.py:250-274`）；注释（`:252-259`）说明占位消息不能是裸的 `"Continue"`，否则某些 OpenAI 兼容 provider 会把它当作调用方的任务而输出关于"continue"这个词的内容（#888）。这是**按轮次清理上下文以控制 token 增长**的机制。### 7.2 字段 → 生产者 → 消费者

| state 字段 | 生产者（写入位置） | 消费者（读取位置） |
|---|---|---|
| `company_of_interest` | `Propagator`（`propagation.py:37`） | Sentiment Analyst（`sentiment_analyst.py:66`）；Trader（`trader.py:26`）；`get_instrument_context_from_state` 兜底（`agent_utils.py:214`） |
| `asset_type` | `Propagator`（`propagation.py:38`） | News Analyst（`news_analyst.py:16`）；Bull/Bear（`bull_researcher.py:23`、`bear_researcher.py:23`）；`build_instrument_context` 兜底（`agent_utils.py:215`） |
| `trade_date` | `Propagator`（`propagation.py:40`） | Market/News/Fundamentals/Sentiment（`market_analyst.py:15` 等）；全部 12 个 tool 的 `InjectedState` |
| `instrument_context` | `trading_graph.py:555`（启动时解析身份） | **全部 12 个 Agent**（经 `agent_utils.py:201-216`） |
| `past_context` | `trading_graph.py:552`（读 memory log） | **仅 Portfolio Manager**（`portfolio_manager.py:38-43`） |
| `portfolio_context` | `trading_graph.py:556` | 3 个 Risk Debator（`*_debator.py:28/28/27`）；Trader（`trader.py:35`）；Portfolio Manager（`portfolio_manager.py:31`） |
| `market_report` | Market Analyst（`market_analyst.py:91`） | Bull/Bear（`bull_researcher.py:18`、`bear_researcher.py:18`）；3 Risk Debator（`*_debator.py:23/23/26`）；Trader（`trader.py:34`） |
| `sentiment_report` | Sentiment Analyst（`sentiment_analyst.py:127`） | Bull/Bear（`:19`）；3 Risk Debator（`*_debator.py:24/24/25`） |
| `news_report` | News Analyst（`news_analyst.py:65`） | Bull/Bear（`:20`）；3 Risk Debator（`*_debator.py:25/25/23`） |
| `fundamentals_report` | Fundamentals Analyst（`fundamentals_analyst.py:65`） | Bull/Bear（`:21`）；3 Risk Debator（`*_debator.py:26/26/24`） |
| `investment_debate_state.history` | Bull/Bear（`bull_researcher.py:56`、`bear_researcher.py:58`） | Bull/Bear 自身回读；Research Manager（`research_manager.py:22`） |
| `investment_debate_state.current_response` | Bull/Bear（`:59`/`:61`） | 下一个发言者（`bull_researcher.py:15-17`）；路由判断（`conditional_logic.py:59`） |
| `investment_debate_state.count` | Bull/Bear（`:60`/`:62`） | 路由判断（`conditional_logic.py:55-58`） |
| `investment_debate_state.judge_decision` | Research Manager（`research_manager.py:65`） | 无下游读取（存档用） |
| `investment_plan` | Research Manager（`research_manager.py:75`） | Trader（`trader.py:28`）；Portfolio Manager（`portfolio_manager.py:35`） |
| `trader_investment_plan` | Trader（`trader.py:97`） | 3 Risk Debator（`*_debator.py:30`）；Portfolio Manager（`portfolio_manager.py:36`） |
| `risk_debate_state.history` | 3 Risk Debator（`*_debator.py:53`） | 3 Risk Debator 自身回读；Portfolio Manager（`portfolio_manager.py:33`） |
| `risk_debate_state.latest_speaker` | 3 Risk Debator（`:57`） | 路由判断（`conditional_logic.py:69-72`） |
| `risk_debate_state.current_{aggressive,conservative,neutral}_response` | 各自（`:58-62` 等） | 对手方（`aggressive_debator.py:16-21` 等） |
| `risk_debate_state.count` | 3 Risk Debator（`:63`） | 路由判断（`conditional_logic.py:65-68`） |
| `final_trade_decision` | Portfolio Manager（`portfolio_manager.py:104`） | 图外：`TradingMemoryLog.store_decision`（`memory.py:30-52`）、`SignalProcessor.process_signal`（`signal_processing.py:29-38`） |
| `sender` | Trader（`trader.py:98`） | 无下游读取 |
| `messages` | 4 个 Analyst（工具往返）、Trader（AIMessage） | 4 个 Analyst |

传递机制的两个要点：（1）辩论状态是**累加式**的——每个发言者把新发言 `history + "\n" + argument` 追加回同一字段（`bull_researcher.py:56`），因此辩论长度随轮次线性增长，且每个 Debator 都能看到完整历史而不只是对手的最新发言；（2）`current_response` 之外还有 `bull_history` / `bear_history` / `aggressive_history` 等**分角色历史**字段，但除被保留在 state 中以外，**当前没有任何消费者读取它们**（只写不读）。

### 7.3 记忆（历史教训）的注入路径

历史记忆不进入 `messages`，而是作为一段文本注入 `past_context`，且**只有 Portfolio Manager 消费**（`portfolio_manager.py:38-43`）：`past_context = state.get("past_context", "")`，非空时包装成 `- Lessons from prior decisions and outcomes:\n{past_context}\n` 注入 prompt。

数据来源是 append-only 的 markdown 决策日志（`agents/utils/memory.py:9-52`），条目标签形如 `[trade_date | ticker | rating | raw | alpha | holding | resolved:date]`（`memory.py:236-249`）。`get_past_context`（`:73-110`）按"同标的最近 5 条 + 跨标的最近 3 条"取样；同标的条目注入**完整决策文本**（`_format_full`，`:321-329`），跨标的条目只注入反思摘要（`_format_reflection_only`，`:331-337`）。

**时点安全**：历史条目按 `resolved` 日期过滤（`memory.py:86-87`），只纳入在 `as_of` 之前已知结果的教训，防止 backtest 从尚未发生的结局中学习。**权衡**：记忆只喂给最后一个判断者，能影响终局定档，但**无法影响分析师的事实采集与辩论过程**——节省 token（只注入一次），却失去了"这次别再用上次失败的方法取数"这类过程性纠错能力。

---

## 8. 结论：对 Aegis 的机制要点

按可移植性排序：

1. **角色按"生产信息 / 对抗论证 / 裁决"三层切开**，并用一句固定 prompt 声明（"another agent decides the trade"）划定生产者不越界。
2. **裁决者看不到原始数据，只看压缩后的论据**；信息损耗与上下文可控性是被有意取舍的一对代价。
3. **"自主 ReAct"由图的环 + 条件边实现，不由 agent 内部循环实现**；只有 3/12 个 Agent 参与该环，且无 per-agent 轮次上限（唯一边界是全局 `recursion_limit`）。
4. **预取注入是工具循环的合法替代**，判据是"数据源是否存在可参数化的查询维度"。
5. **结构化输出只覆盖档位选择**，字段描述承担输出指令，render 层把类型转回 markdown 以保护既有字符串契约；解析失败不降级为可交易信号（`REVIEW` 哨兵），并保留自由文本降级路径与正则兜底解析。
6. **空值必须有语义**（"未提供" ≠ "空"），由 `opponent_argument_or_opening` / `report_or_absent` / `get_portfolio_context_from_state` 三处机制化。
7. **不信任模型的时空参数**：`InjectedState` 注入 `trade_date`，`as_of` / `as_of_window` 在 tool 层强制夹取，`resolve_instrument_identity` 用一次确定性查询消除身份幻觉。
8. **置信度作为数据质量元数据只出现在信息生产端**（Sentiment Analyst 的 `confidence` 字段），决策端不带自评置信度。

[← 返回研究索引](README.md)
