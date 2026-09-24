# Aegis 多 Agent 架构与职责边界

> 状态：当前架构约束（Authoritative）
> 更新时间：2026-09-24
>
> 本文定义 Aegis 后续演进时必须遵守的架构边界。新增 Agent、Tool、Skill、MCP、Crawler、API、数据库或 Graph 节点之前，应先检查是否符合本文。

## 1. 一句话原则

Aegis 按**研究职责**拆 Agent，按**能力**暴露 Tool，按**来源**实现 Provider / Binding，由 Graph 负责协作。

```text
Agent = 谁负责研究什么
Context = Agent 之间传递什么
Capability = Agent 需要什么能力
Provider / Binding = 能力从哪里来
Graph = Agent 如何协作
```

不得按数据来源拆 Agent。

错误：

```text
Tushare Agent
MCP Agent
Skill Agent
Crawler Agent
Database Agent
```

正确：

```text
Market Agent
Technical Agent
News/Event Agent
Fundamental Agent
Capital Flow Agent
Trading Agent
```

Skill、MCP、Crawler、HTTP API、DuckDB、Tushare 都不是 Agent 类型，它们只是能力实现或工具来源。

## 2. 目标架构

```text
                         Research Graph
                              │
                              ▼
                         Market Agent
                              │
                       MarketContext
                              │
              ┌───────────────┼────────────────┐
              ▼               ▼                ▼
       Technical Agent   News/Event Agent   Fundamental Agent
              │               │                │
      TechnicalContext   EventContext    FundamentalContext
              │               │                │
              └───────────────┼────────────────┘
                              │
                              ▼
                         Trading Agent
                              │
                           Decision
```

后续真实需求出现时可增加 Capital Flow、Sentiment、Risk、Reflection 等 Agent，但只有职责足够独立、输入输出可形成稳定契约时才新增。当前阶段不为了“看起来像 Multi-Agent”而提前创建空 Agent。

## 3. Agent 职责

### Market Agent

负责整体市场环境，输出 MarketContext。典型职责：宽基指数、涨跌家数、涨跌停、总成交额、市场风格、板块强弱、整体风险环境。

不负责：单股技术指标、单股新闻、单股财务、最终交易判断。

### Technical Agent

负责标的价格行为、技术状态和确定性技术指标，输出 TechnicalContext。

典型职责：OHLCV、PIT 复权、收益率、MA、RSI、MACD、量能、波动、停牌/未上市/数据缺失状态。

不负责：公司新闻、财务报表解释、全市场环境、最终 WATCH / AVOID / CANDIDATE。

### News/Event Agent

负责与标的或市场相关的新闻、公告、政策和事件，输出 EventContext。

输入能力可以来自 News API、公告 API、Web Search、Crawler、MCP、Skill、本地新闻库。Agent 不关心来源形式，只关心事件研究职责。

必须区分：

```text
Fact           → 某公告在某时间发布
Interpretation → 该事件可能构成何种影响
```

Crawler 只是 Capability / Tool 的一种实现，不默认成为 Agent。

### Fundamental Agent

负责公司基本面、财务质量与估值，输出 FundamentalContext。典型职责：营收、利润、现金流、ROE、毛利率、资产负债、估值和财务趋势。

### Capital Flow Agent

只有在资金研究成为独立且稳定的职责后再引入。可能负责主力资金、融资融券、龙虎榜、北向/沪深港通、大单结构，输出 CapitalFlowContext。

### Trading Agent

Trading Agent 是**综合判断者**，不是全能研究 Agent。

目标输入：

```text
MarketContext
TechnicalContext
EventContext        // 存在时
FundamentalContext  // 存在时
CapitalFlowContext  // 存在时
```

输出 Decision。

Trading Agent 可以比较不同研究维度、识别证据冲突、综合市场与个股条件、调整 confidence、输出 WATCH / AVOID / CANDIDATE。

Trading Agent 不应该自己抓行情、临时写 Python 算指标、爬新闻、承担所有 Tool 调用，也不应该知道 TechnicalContext 最终来自 Tushare、MCP 还是 DuckDB。

## 4. Context 是 Agent 间稳定契约

Agent 之间优先传递结构化 Context，不以长篇自然语言 report 作为主契约。

```text
Market Agent      → MarketContext
Technical Agent   → TechnicalContext
News/Event Agent  → EventContext
Fundamental Agent → FundamentalContext
Trading Agent     → Decision
```

原则：

1. Context 是业务事实和解释结果的结构化边界。
2. Context 必须可 Validate。
3. 字段必须有明确单位与时间语义。
4. 缺失、无权限、未覆盖不能伪装成 0。
5. Agent 内部可以自然语言推理，但 Agent 间主接口应结构化。
6. Graph 不解析 Provider 特有格式。

TradingAgents 当前共享 State 中大量使用 market_report、news_report、fundamentals_report 等字符串；Aegis 借鉴其多专家协作思想，但优先使用 typed Context。

## 5. Capability 与 Provider 分层

Capability 表达“需要什么”，例如：

```text
Technical: StockSnapshot / PriceHistory / AdjustmentFactor / TradingCalendar / TechnicalIndicators
News: CompanyNews / Announcements / PolicyNews / ArticleFetch / SiteSearch
```

Provider / Binding 表达“从哪里来”：

```text
PriceHistory
├── Tushare
├── DuckDB
├── MCP
└── HTTP API

CompanyNews
├── News API
├── MCP
├── Crawler
└── Local DB
```

Agent 不应把 Provider 名写进核心业务逻辑。

当前仍遵守：先出现第二个真实实现，再抽象共同接口、router 或 fallback。不要提前建立万能 Registry、Factory、ProviderManager，但业务代码从一开始必须避免与具体来源强绑定。

## 6. Skill、MCP、Crawler、API、DB 的位置

Skill 表达方法论、工作流、知识和可执行辅助脚本；它可以帮助 Agent 使用能力，但 Skill 本身不是 Data Provider 类型。

MCP 是 Tool / Capability 的接入协议之一。

Crawler 是获取 Web 内容的基础能力。只有当 Web Research 本身形成复杂、独立、可复用的研究职责时，才考虑 Web Research Agent。

API、DuckDB、MySQL、本地文件属于基础设施来源，它们解决“数据从哪里来”，不决定“谁负责研究什么”。

## 7. 确定性计算与 LLM 边界

可以确定计算的内容不得交给 LLM 临场计算。

```text
Raw Data
   ↓
Normalize
   ↓
Deterministic Calculation
   ↓
Typed Facts
   ↓
Domain Agent Interpretation
```

例如 PIT_QFQ、MA、RSI、MACD、收益率、单位转换、交易日窗口，应由确定性代码完成。

LLM / Agent 负责选择需要哪些事实、解释趋势、比较证据、识别冲突、得出研究判断。

确定性代码可以位于 Skill script、Go package、服务或 Provider 内；物理位置不是核心，关键是不能由 Trading Agent 临时生成不稳定计算逻辑。

## 8. Graph 的职责

Graph 只负责 Agent 执行顺序、Context 传递、真实需要时的分支/汇聚、生命周期和错误传播。

Graph 不负责数据清洗、技术指标计算、Provider 路由细节或业务结论。

当前最小 Graph：

```text
START → Market Agent → Trading Agent → END
```

下一阶段目标：

```text
START → Market Agent → Technical Agent → Trading Agent → END
```

先顺序执行，证明契约后再考虑并行。News/Event、Fundamental 等 Agent 只有在实现成熟后再进入 Graph。

## 9. TradingAgents 给 Aegis 的参考

TradingAgents 当前采用多个职责型 Analyst、Bull/Bear Researcher、Research Manager、Trader、Risk Debate、Portfolio Manager，并让不同 Analyst 拥有不同 Tool 集；Tool 下方的数据接口再按 vendor 路由。

值得借鉴：

1. 按研究职责拆 Analyst。
2. 每个 Analyst 有自己的 Tool 集。
3. Tool 下方再按 vendor 路由。
4. Graph 统一编排，不由 Trader 手动调用所有 Analyst。
5. 后续决策节点消费前序研究结果。

Aegis 不直接照搬：

1. 不把长文本 report 作为 Agent 间主要契约，优先 typed Context。
2. 当前不引入 Bull/Bear debate。
3. 当前不引入 aggressive/neutral/conservative risk debate。
4. 不为了复刻 TradingAgents 节点数量而创建没有稳定职责的 Agent。
5. 不绑定其 yfinance / AlphaVantage 数据模型。

Aegis 借鉴的是组织思想，不是复制节点数量。

## 10. 新功能设计检查表

### 要不要新 Agent？

只有当以下条件大部分成立时才新建：

- 有独立研究职责；
- 有独立输入与输出；
- 值得形成稳定 Context；
- 可以独立测试；
- 使用独立 Prompt / Tool 集 / 模型更合理。

如果只是新增数据源，不创建 Agent。

### 要不要新 Capability？

当 Agent 出现新的“需要什么数据/动作”时，创建或扩展窄 Capability。

### 要不要新 Provider？

只有 Capability 已存在，而出现新的真实来源时增加实现。

### 要不要进 Graph？

先保证 Agent 单独可运行、Context 契约稳定、测试通过，再接 Graph。

## 11. 明确禁止的走偏方式

### 禁止 1：Trading Agent 重新变成单体 Agent

```text
Trading Agent
├── 查行情
├── 算技术指标
├── 爬新闻
├── 查财务
├── 查资金
└── 做 Decision
```

### 禁止 2：数据源变成 Agent

默认禁止 Tushare Agent / MCP Agent / Crawler Agent。

### 禁止 3：Provider 细节进入业务 Context

如果审计确需 provenance，可作为元数据保留，但不得成为领域职责判断本身。

### 禁止 4：LLM 重复确定性计算

已有确定性计算能力后，不允许 Agent 临时生成脚本重复实现同一公式。

### 禁止 5：先造大而全 Registry

没有第二个真实实现之前，不提前建通用 Provider Registry。

### 禁止 6：自然语言 Report 取代核心契约

可以保留 summary，但 Graph 的主要数据交换应使用 typed Context。

## 12. 当前下一步

V2.6 的正确下一步不是继续强化 Trading Agent 的数据获取能力，而是：

```text
1. 定义 TechnicalContext
2. 实现 Technical Agent
3. Technical Agent 独立测试
4. 为 Technical Agent 接入第一套真实数据能力
5. 验证 PIT / 技术指标 / 时间语义
6. Graph 升级为 Market → Technical → Trading
7. Trading Agent 改为消费 TechnicalContext
```

随后再以相同模式实现 News/Event Agent、Fundamental Agent、Capital Flow Agent。每次只增加一个真实职责。