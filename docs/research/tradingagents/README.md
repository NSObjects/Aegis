# TradingAgents 框架研究

对 [TauricResearch/TradingAgents](https://github.com/TauricResearch/TradingAgents) 的架构研究。
Aegis 把它当作**架构研究对象**——研究「多 Agent 怎么协作、数据时效怎么做、决策怎么变成经验」，而不是技术栈参考。

> **研究对象版本**：v0.5.0，commit `2d17df8`（2026-09-17）
> 本目录所有结论均标注 `路径:行号`，可回源核对。结论只对该版本负责。

---

## 一句话结论

**TradingAgents 值得学的不是它的 Agent 名单，而是三件事**：
① 用**具名字段**而不是对话传递 Agent 之间的信息；
② 在每次分析**开始之前**先结算历史决策、生成反思、再注入本次上下文；
③ 把**数据时效性（point-in-time）**当作贯穿全系统的约束，而不是某个模块的特性。

而它**没有**解决的、也正是 Aegis 存在的理由：**它不把「当前市场环境适合什么打法」当作第一层决策**——
它以「给定一只股票，产出一份高质量判断」为目标。

---

## 章节地图

| 章节 | 内容 | 研究的问题 |
|---|---|---|
| [01-整体架构](01-整体架构.md) | 图、状态、控制流、运行生命周期、双层模型、checkpoint | 它到底是怎么跑起来的？ |
| [02-Agent体系](02-Agent体系.md) | 12 个角色、职责分层、Prompt 结构、结构化输出 schema、Tool 组织 | 角色怎么划分？Agent 是自主循环还是单次调用？ |
| [03-数据层与时效性](03-数据层与时效性.md) | 数据源路由、vendor 清单、point-in-time 机制、缓存、错误契约 | 怎么保证不回看未来？ |
| [04-记忆与反思](04-记忆与反思.md) | 记忆分层与检索、反思机制、是否构成闭环学习、结构化输出可靠性 | 「经验」是怎么产生和使用的？ |
| [05-模型接入与运行形态](05-模型接入与运行形态.md) | 多供应商抽象、模型目录、能力探测、密钥、配置分层、CLI、部署 | 多模型怎么接？配置怎么分层？ |
| [06-对Aegis的取舍](06-对Aegis的取舍.md) | 吸收清单 / 改造清单 / 不搬清单 / 纠偏 / 差距分析 / 落地顺序 | Aegis 该拿它怎么办？ |

**如果只读两章**：读 [01](01-整体架构.md) 和 [06](06-对Aegis的取舍.md)。

---

## 框架速览

### 形态与规模

```text
TradingAgents v0.5.0
├── tradingagents/     ~9,450 行   核心包
│   ├── graph/          图、状态、条件逻辑、checkpoint、反思
│   ├── agents/         12 个 Agent（4 分析师 + 2 研究员 + 2 经理 + 3 风控 + 1 交易员）
│   ├── dataflows/      20 个数据源模块（yfinance / Alpha Vantage / FRED / SEC EDGAR / Reddit / StockTwits / Polymarket）
│   └── llm_clients/    多供应商抽象 + 模型目录 + 能力探测
├── cli/               ~2,420 行   交互式命令行（typer + rich 全屏 TUI）
└── tests/            ~10,400 行   ← 测试体量超过核心包
```

技术栈是 **Python + LangGraph**。执行模型是一个 `StateGraph`：节点是 Agent，边由条件函数决定。

### 值得注意的发现

**① 控制流不在 Agent 手里。**
循环轮数、谁下一个发言、何时退出，全部由 `ConditionalLogic` 这个纯函数类根据 state 字段决定
（`graph/conditional_logic.py:52`、`:63`）。所以它本质是「状态机 + LLM 决策节点」，不是「一群 Agent 自己聊」。
——好处是可预测、可单测。

**② Agent 本身全是「单次 LLM 调用」，ReAct 循环是图拓扑实现的。**
分析节点返回 `tool_calls` → 条件边 `should_continue_*` → `ToolNode` → 回到分析节点
（`graph/setup.py:124-129` + `graph/conditional_logic.py:14-20`）。
模型侧看不到任何循环，循环完全在图里。而且**只有 3/12 个分析师走这条循环**，
且**没有 per-agent 轮次上限**，唯一边界是全局 `recursion_limit=100`。

**③ 裁决者看不到原始数据。**
Research Manager 与 Portfolio Manager 的 prompt 里**没有**四份分析师报告，只有辩论历史与上游计划
（`agents/managers/portfolio_manager.py:38-43`）。这是一个**刻意的强制抽象层**——迫使裁决基于被加工过的结论而不是原始数据。
唯一的例外是 Trader，它需要读 `market_report` 拿价格结构。

**④ 12 个 Agent 中 9 个完全不读 `state["messages"]`。**
协作靠结构化字段而不是自然语言历史。

**⑤ 每次运行开始前，先结算上一次的决策。**

```text
create_run_state()                                   graph/trading_graph.py:539
   ├── _resolve_pending_entries()    拉实现收益（含 alpha）→ 生成反思 → 批量写回日志
   ├── get_past_context(as_of=...)   注入已结算的教训
   └── resolve_instrument_context()  确定性解析标的身，防幻觉
```

三条配套纪律：**结果未定不结算**、**反思失败不阻断**、**按 `as_of` 过滤后注入**。
结算发生在注入之前，所以本次刚结算出的教训，同一次运行里就会被 Portfolio Manager 读到。

> ⚠️ **但这不是闭环学习。** 教训的唯一消费点是 Portfolio Manager 的 prompt 一处
> （`agents/managers/portfolio_manager.py:38`）；穷举全仓**不存在任何把反思写回 prompt / 配置 / 策略的代码路径**，
> 反思也只是纯字符串、不带结构化归因。即它实现的是 **Outcome → 下次 context**，
> 而不是 Execution → Outcome → Reflection → **Proposal → Evaluation → Promotion**。
> 详见 [04-记忆与反思](04-记忆与反思.md) §4。

**⑥ point-in-time 正确性是它最大的工程主题。**
12 个文件涉及，且 v0.3.1 → v0.4.0 → v0.5.0 **连续三个版本**都在修 look-ahead 问题（issue #1220 / #1275 / #1300）。
每个带日期的数据路径都按 `curr_date` 过滤；连「教训」也只注入当时已结算的那些。

> 但它**并不完备**：`_fill_price_gaps` 用 `ffill().bfill()`，`bfill` 方向是前视；
> `get_YFin_data_online` 没有显式上界裁剪；**全库没有交易日历**——`look_back_days` 一律按自然日计算。
> 详见 [03-数据层与时效性](03-数据层与时效性.md)。

**⑦ 用确定性工具当事实基准，并明确冲突处理。**
Market Analyst 的 prompt 强制要求调用 `get_verified_market_snapshot` 并把它当作精确价格/指标值的**唯一事实来源**；
其他工具与之冲突时要求**标注冲突**而不是自己编一个协调值。
注意：提供这个快照的 `market_data_validator.py` **不是**「未来数据检测器」，
而是**抗 LLM 编造数字的 ground-truth 基准**——真正的时效校验在 `stockstats_utils._assert_ohlcv_not_stale`。

**⑧ 双层模型分工。**
`quick_thinking_llm` 用于产出信息的节点（分析师、研究员、交易员、风控辩手），
`deep_thinking_llm` 用于做最终仲裁的节点（Research Manager、Portfolio Manager）。

**⑨ 置信度只出现在「信息生产端」。**
只有 Sentiment Analyst 的 schema 带置信度（`agents/schemas.py:342-349`）；
三个决策 schema（`ResearchPlan` / `TraderProposal` / `PortfolioDecision`）**都没有置信度字段**。
这与 Aegis 的 `MarketContext` 显式带 `confidence` 形成有趣对比。

### 演进史（从 CHANGELOG 看它的重心）

| 版本 | 主题 |
|---|---|
| v0.2.4 | 结构化输出 Agent、checkpoint resume、**持久决策日志** |
| v0.2.5 | 环境变量可配置、API key 自动探测、非美市场基准 |
| v0.3.0 | **verified data-access contract**、Provider 注册表扩展 |
| v0.3.1 | Alpha Vantage look-ahead 过滤、**图路由崩溃安全**、图形状感知的 checkpoint 恢复 |
| v0.4.0 | **FRED / 社交情绪 / 决策日志的 look-ahead 修复**、决策信号更清晰、Trader 价格接地 |
| v0.5.0 | **每条带日期的路径都做到 point-in-time**、SEC EDGAR 按原始披露、网格回测、持仓感知 |

一句话读法：**这个项目的迭代主线是「正确性」，不是「更多 Agent」。**

---

## 对 Aegis 的结论摘要

完整论证见 [06-对Aegis的取舍](06-对Aegis的取舍.md)，这里只放结论。

### 直接吸收

| 机制 | 落点 |
|---|---|
| 字段即接口的 State 契约 | 补齐跨 Agent 字段契约（`MarketContext` → Trading Agent） |
| 控制流外置到纯函数 | 与 `docs/01` §4「Graph 管阶段、ADK 管智能行为」同源；**但改用枚举，不用字符串前缀** |
| 运行开始先结算历史 | `docs/08`「Execution → Outcome → Reflection」闭环的地基 |
| `as_of` 过滤 + `resolution_date` | 把 `docs/06` 的「规则」变成「机制」 |
| 确定性验证工具 + 冲突标注 | 加进 Market Agent prompt |
| 解析失败返回独立取值（`REVIEW`） | 不要与「中性结果」共用取值；「未观测」≠「不存在」 |
| 回测的范围声明（评估决策质量，不做持仓模拟） | 回测模块开头写死边界 |

### 改造后吸收

- **双层模型** → 保留 Aegis 已有的 per-agent override，在其上叠加 tier 语义
- **能力探测表** → 抽 `internal/llm/capabilities`，取代 `deepseek.go:35` 的硬编码；
  **注意**：TradingAgents 的模型目录与能力表是两张需人工同步的表，已经因此出现缺陷（见下）
- **Checkpoint** → 若引入，必须把「图形状签名」编进 thread id，配置变更不能误当成续跑
- **持仓上下文** → 三种状态（有持仓 / 空仓 / 未提供）必须区分，不能把「未提供」当「空仓」

### 它帮不上忙的地方（A 股特有，必须自己设计）

| 空白 | TradingAgents 的现状 | Aegis 必须做什么 |
|---|---|---|
| **交易日历** | **完全没有**。`look_back_days` 按自然日算，`MAX_OHLCV_STALE_DAYS = 10` 是为长假放宽的经验值 | 引入真实交易日历（含调休）。Aegis 要求的「最近一个数据完整的交易日」用自然日近似会失真 |
| **复权语义** | 用 `auto_adjust=True`，即**隐式前复权**，历史价格随除权除息重算 | 显式建模复权方式，并回答「回测用哪种、Agent 看到哪种、两者是否一致」——否则 point-in-time 会被复权悄悄破坏 |

### 明确不搬

整图形态（缺市场环境第一层决策）、字符串前缀路由、固定轮数辩论、串行分析师、
清空 messages 后信息降级为文本、全局单例 config、ticker 维度的记忆结算局限。

### 顺带发现的三处缺陷（作为反例）

研究过程中发现的三处源码级不一致，价值在于它们都是**「两处实现各自演化」的典型后果**：

**① 模型目录与能力表不同源。**
`llm_clients/model_catalog.py:158` 把 `deepseek-flash` 作为主推模型，
但 `llm_clients/capabilities.py:95-98` 只登记了 `deepseek-chat` / `deepseek-reasoner` / `deepseek-v4-flash` / `deepseek-v4-pro`，
正则 `^deepseek-v\d`（`capabilities.py:113`）也匹配不到它 ——
于是 `deepseek-flash` **静默落到 `_DEFAULT`**，拿到一组可能不正确的默认能力。

**② 记忆检索与回测评分策略不一致。**
`get_past_context` 只过滤 pending 与 `as_of`，**不排除 `REVIEW` 条目**（`agents/utils/memory.py:85-87`），
而回测的 `summarize()` 显式排除 `RATING_REVIEW`。
同一个概念在两处有不同定义，会导致「不可判定」的决策被当作经验重新注入。

**③ 记忆的 ticker 键未归一化。**
写入与查询用的是 `company_name` 原样（`graph/trading_graph.py:576`、`:552`），
而 `_fetch_returns` 内部确实调用了 `normalize_symbol`（`:317`）。
于是同一标的的不同写法会成为**两条彼此独立的记忆**。

> 教训：**同一个概念只能有一处定义**。能力、评级、标的标识这三者，
> 在 Aegis 里都应该收敛成单一来源（single source of truth），而不是让两处实现各自解释。

### Aegis 已经领先的地方

**TradingAgents 不支持 per-agent 模型选择**——只有全局单 Provider + quick/deep 两档
（`graph/setup.py:76-92`，12 个 Agent 里只有 2 个用 deep）。
Aegis 的 `ModelProfileForAgent`（`default_model` + per-agent override）表达能力更强，**不需要向它看齐**。

---

## 研究方法与可信度

- **来源**：`git clone` 该仓库并锁定到 `2d17df8`；所有结论来自源码阅读，未依赖二手描述
- **引用规范**：每处结论附 `路径:行号`；对话注释里的 issue 编号（如 `#1251`）原样保留，便于溯源
- **诚实标注**：源码无依据的内容标注「未确认」；模型名与上下文长度等取自该版本目录原文，未做外部核实
- **未覆盖**：`tests/` 仅在与设计契约相关处引用；未做性能与成本实测；未评估其策略有效性

复现研究环境：

```bash
# github.com 在部分网络下不可达，需走代理
git -c http.proxy=http://127.0.0.1:7890 clone --depth 1 \
  https://github.com/TauricResearch/TradingAgents.git
cd TradingAgents && git log -1 --format='%h %ad %s' --date=short
# 期望：2d17df8 2026-09-17 Merge pull request #1364 from TauricResearch/v0.5.0
```
