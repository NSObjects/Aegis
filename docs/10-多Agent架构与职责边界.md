# Aegis 组织级 Multi-Agent 架构与职责边界

> 状态：最高优先级架构约束（Authoritative）
> 更新时间：2026-09-24
>
> 本文定义 Aegis 长期的组织级 Multi-Agent 模型，以及当前增量实现必须遵守的边界。
> 目标不是堆很多 Agent，而是让系统能够像一个真实投资研究组织一样，由不同 Team / Role 分工协作，同时保持结构化、可测试、可替换。

## 1. 核心原则

Aegis 的长期抽象分为七层：

```text
Team        = 一组共同完成某类组织目标的角色
Role        = 组织中的稳定职责
Agent       = Role 的具体运行实例
Context     = Role / Team 之间传递的结构化工作产物
Capability  = Agent 完成职责所需要的能力
Provider    = Capability 的具体数据或执行来源
Graph       = 公司工作流，负责 Team / Agent 的协作与交接
```

其中最重要的规则是：

```text
Agent 按职责拆
Capability 按能力拆
Provider 按来源拆
Graph 按协作关系组织
```

不得把数据来源直接当成 Agent。

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
Technical Analyst
News/Event Analyst
Fundamental Analyst
Research Manager
Trader
Risk Analyst
Portfolio Manager
```

Skill、MCP、Crawler、HTTP API、DuckDB、Tushare 都属于能力或基础设施层，不是组织角色。

---

## 2. Aegis 的长期组织模型

Aegis 的目标不是：

```text
很多分析 Agent
      ↓
一个超级 Trading Agent
```

而是形成两个互相连接、但职责不同的系统：

```text
① Online Decision Loop
   当前市场事实 → 研究 → 交易 → 风险/组合决策

② Offline Evolution Loop
   历史/PIT 数据 → 策略研究 → 独立回测 → 鲁棒性验证
   → Approved Strategy → 进入下一轮 Online Decision
```

组织结构：

```text
                    Aegis Investment Research Firm

        ┌──────────────── Online Decision Loop ────────────────┐
        │                                                      │
        │                 Analyst Team                         │
        │   Market / Technical / News / Fundamental / Flow    │
        │                        │                             │
        │                        ▼                             │
        │                 Research Team                        │
        │          Bull / Bear / Research Manager             │
        │                        │                             │
        │                        ▼                             │
        │                  Trading Team                        │
        │                      Trader                          │
        │                        │                             │
        │                        ▼                             │
        │              Risk / Portfolio Team                  │
        │          Risk Analyst / Portfolio Manager           │
        │                        │                             │
        └────────────────────────┼─────────────────────────────┘
                                 │
                                 │ production outcome / feedback
                                 ▼
        ┌──────────────── Offline Evolution Loop ──────────────┐
        │                                                      │
        │                 Strategy Team                         │
        │     Strategy Research / Regime Research / Tuning     │
        │                        │                             │
        │                 StrategyCandidate                    │
        │                        ▼                             │
        │              Backtest / Validation Team              │
        │   Backtest / Walk-forward / Robustness / Cost Test   │
        │                        │                             │
        │                 ValidationReport                     │
        │                        ▼                             │
        │                 ApprovedStrategy                     │
        └────────────────────────┼─────────────────────────────┘
                                 │
                                 └────→ 下一轮 Online Decision
```

这是长期组织模型，不代表当前版本要一次性实现所有节点。

---

## 3. Team / Role / Agent 的关系

### 3.1 Team

Team 是组织边界，描述“一组角色共同负责什么”。

例如：

```text
Analyst Team
→ 负责收集、验证、解释各研究维度的事实

Research Team
→ 负责把多个研究维度组织成投资逻辑和反方论证

Trading Team
→ 负责把投资逻辑转化为交易方案

Risk / Portfolio Team
→ 负责组合约束、风险预算和最终组合决策

Strategy Team
→ 负责发现、设计、改进和版本化策略，不负责证明自己有效

Backtest / Validation Team
→ 独立验证策略在历史、样本外、不同市场环境和真实成本下是否成立
```

Team 不是数据源，也不是一个万能 Agent。

### 3.2 Role

Role 是稳定的组织职责。

例如：

```text
Technical Analyst
News/Event Analyst
Bull Researcher
Bear Researcher
Research Manager
Trader
Risk Analyst
Portfolio Manager
```

Role 应明确：

- 输入契约
- 输出契约
- 职责边界
- 可使用的 Capability
- 默认 Prompt / Policy
- 可选 Model Profile

### 3.3 Agent

Agent 是 Role 的具体运行实例。

```text
Role: Technical Analyst
        ↓
Agent instance
├── Model
├── Prompt
├── Middleware
├── Capabilities / Tools
└── Runner
```

未来同一个 Role 可以有不同 Agent 实现，例如不同模型、不同市场版本或不同策略版本。

但当前阶段**不实现通用 Role/Team Registry**。先用包结构和 Graph 表达职责；只有出现真实重复后再抽象运行时类型。

---

## 4. Analyst Team：回答“发生了什么”

Analyst Team 是事实研究层。

### Market Analyst / 当前 Market Agent

负责整体市场环境：

- 宽基指数
- 市场广度
- 涨跌停
- 总成交额
- 市场风格
- 板块强弱
- 整体风险偏好

输出：

```text
MarketContext
```

### Technical Analyst

负责单标的技术状态：

- OHLCV
- PIT 复权
- 收益率
- MA / RSI / MACD
- 量能
- 波动
- 停牌 / 未上市 / 数据缺失状态

输出：

```text
TechnicalContext
```

### News/Event Analyst

负责新闻、公告、政策、事件研究。

可使用：

```text
News API
公告 API
Web Search
Crawler
MCP
Skill
Local DB
```

输出：

```text
EventContext
```

必须区分：

```text
Fact           → 某事件在什么时间真实发生
Interpretation → 该事件可能产生什么影响
```

Crawler 默认只是 Capability，不因为需要抓网页就创建 Crawler Agent。

### Fundamental Analyst

负责：

- 营收 / 利润
- 现金流
- ROE / 毛利率
- 资产负债
- 估值
- 财务趋势

输出：

```text
FundamentalContext
```

### Capital Flow Analyst

真实需求成熟后再引入，可能负责：

- 主力资金
- 融资融券
- 龙虎榜
- 沪深港通
- 大单结构

输出：

```text
CapitalFlowContext
```

### 可扩展 Analyst Role

未来还可以自然增加：

```text
Macro Analyst
Sector Analyst
Sentiment Analyst
Quant Analyst
Alternative Data Analyst
```

新增条件是职责独立、Context 稳定、可独立测试，而不是为了增加 Agent 数量。

---

## 5. Research Team：回答“这些事实意味着什么”

Analyst Team 产生的是多维研究事实和各自解释。

当研究维度逐渐增多时，不应把所有 Context 永久直接塞给 Trader，否则 Trader 会再次退化成超级 Agent。

正确方向是增加研究汇总层：

```text
MarketContext
TechnicalContext
EventContext
FundamentalContext
CapitalFlowContext
        ↓
   Research Team
```

### Bull Researcher

职责：

- 构建支持候选机会的 strongest case
- 引用明确证据
- 回应主要反方风险
- 不制造新的底层事实

### Bear Researcher

职责：

- 构建反方 case
- 找出证据缺口、矛盾和下行情景
- 质疑过度乐观假设
- 不制造新的底层事实

### Research Manager

职责：

- 综合 Analyst Context
- 比较 Bull / Bear 论证
- 判断哪些证据更可靠
- 明确未解决冲突
- 形成 Investment Thesis

目标产物：

```text
InvestmentThesis
├── symbol
├── as_of
├── thesis
├── supporting_evidence
├── opposing_evidence
├── unresolved_risks
└── confidence
```

具体 Go 契约只有在进入该阶段时再定义，不提前创建空结构。

---

## 6. Trading Team：回答“如果参与，交易计划是什么”

Trader 不再负责重新做所有研究。

目标输入：

```text
InvestmentThesis
+ 必要的价格 / 技术执行上下文
```

职责：

- 把研究结论转为交易计划
- 定义参与条件
- 定义失效条件
- 定义观察条件
- 在需要时形成 entry / stop / sizing proposal

长期产物可以演进为：

```text
TradeProposal
```

当前 Aegis 的 WATCH / AVOID / CANDIDATE Decision 属于 V2.x 过渡期研究输出，继续保留，直到 Trading / Risk / Portfolio 分层真的落地。

不要因为长期目标存在 TradeProposal，就现在把 Decision 契约推倒重来。

---

## 7. Risk / Portfolio Team：回答“这个方案在组合层能不能接受”

只有当 Aegis 开始具备真实组合、仓位、风险预算等输入时再实现。

### Risk Analyst

未来可能负责：

- 单笔风险
- 波动 / 回撤
- 事件风险
- 流动性风险
- 相关性
- 集中度
- 组合暴露

输出可演进为：

```text
RiskAssessment
```

### Portfolio Manager

未来可能消费：

```text
TradeProposal
RiskAssessment
PortfolioContext
```

然后形成组合级最终决策。

在没有 PortfolioContext 之前，不创建假的 Portfolio Manager。

---

## 8. Strategy Team：持续寻找“当前环境下更合适的策略”

Strategy Team 是**策略研发部门**，不直接负责某一次标的判断。

它回答：

```text
当前市场环境下，
什么规则、参数、持有周期、过滤条件和组合方式更值得使用？
```

但这里的“最优”必须严格定义为：

```text
在指定数据范围、市场 regime、成本模型、风险约束和验证方法下，
当前已验证候选集合中的相对更优方案
```

不能宣称存在永久、全市场、未来确定的“绝对最优策略”。

### 8.1 Strategy Researcher

职责：

- 从历史结构和当前 MarketContext 中发现可研究假设
- 设计入场 / 退出 / 过滤 / 持有周期规则
- 组合已有 Analyst Context 形成策略特征
- 设计参数搜索空间
- 生成明确、可执行、可回测的 StrategyCandidate
- 根据回测反馈产生新版本，而不是直接修改生产策略

不负责：

- 自己决定回测是否通过
- 使用未来数据证明策略
- 因一次近期收益好就自动上线

### 8.2 Regime Research

策略不能只看全样本平均收益。

Strategy Team 应研究：

```text
RISK_ON
MIXED
RISK_OFF

趋势 / 震荡
高波动 / 低波动
放量 / 缩量
大盘 / 小盘风格
行业轮动环境
流动性环境
```

目标不是找“一套永远最好”的策略，而是建立：

```text
Market Regime
      ↓
Validated Strategy Set
      ↓
当前可用策略及适用条件
```

例如未来可以形成：

```text
StrategyPolicy
├── regime
├── eligible_strategies
├── ranking_metrics
├── risk_constraints
└── selection_reason
```

### 8.3 Strategy Artifact

策略必须是版本化、可复现的结构化 Artifact，而不是一句 Prompt。

长期至少应表达：

```text
StrategySpec
├── id
├── version
├── universe
├── required_features
├── entry_rules
├── exit_rules
├── holding_period
├── filters
├── parameters
├── applicable_regimes
├── risk_constraints
└── data_requirements
```

具体 Go 类型等真正实现 Strategy Team 时再定义。

---

## 9. Backtest / Validation Team：独立证明策略是否站得住

Backtest Team 与 Strategy Team 必须分离。

核心原则：

```text
Strategy Team 提案
        ↓
Backtest Team 验证
        ↓
通过后才成为 ApprovedStrategy
```

不能：

```text
Strategy Agent
├── 发明策略
├── 自己选择最有利数据
├── 自己回测
└── 自己宣布上线
```

这会形成典型的过拟合和自我确认。

### 9.1 Backtest Engineer

负责确定性执行：

- 历史信号生成
- 订单 / 成交模拟
- 持仓状态
- 手续费
- 滑点
- 涨跌停
- 停牌
- 流动性约束
- 交易日历
- 组合净值

LLM 不参与逐笔收益计算。

### 9.2 Validation / Robustness

验证至少逐步覆盖：

```text
In-sample
Out-of-sample
Walk-forward
Rolling windows
不同 Market Regime
参数敏感性
交易成本敏感性
流动性约束
不同股票池
不同时间区间
```

必须防止：

- look-ahead bias
- survivorship bias
- future adjustment leakage
- data snooping
- parameter overfitting
- cherry-picking
- 忽略交易成本
- 用全样本调参后再把同一全样本称为验证集

### 9.3 ValidationReport

长期产物应是结构化验证结果：

```text
ValidationReport
├── strategy_id / version
├── data_period
├── in_sample
├── out_of_sample
├── walk_forward
├── regime_breakdown
├── return
├── max_drawdown
├── volatility
├── win_rate
├── turnover
├── cost_assumptions
├── robustness
├── failure_modes
└── verdict
```

通过验证的策略才能进入 ApprovedStrategy 集合。

---

## 10. 持续进化闭环

Aegis 的长期“不断升级”不是让一个 Agent 永久自我修改，而是建立可审计闭环：

```text
历史/PIT 数据
    ↓
Strategy Team
    ↓
StrategyCandidate vN
    ↓
Backtest / Validation Team
    ↓
ValidationReport
    ↓
Reject / Revise / Approve
              │
              ▼
      ApprovedStrategy vN
              ↓
      Online Decision Loop
              ↓
      真实运行结果 / 新数据
              ↓
      Performance Review
              ↓
Strategy Team 产生 vN+1
```

关键规则：

1. 生产策略必须版本化。
2. 新策略不能覆盖旧策略历史记录。
3. 每个版本必须绑定数据窗口和验证结果。
4. 策略升级必须能解释“为什么变更”。
5. 在线结果可以反馈给策略研究，但不能直接污染历史 OOS 验证。
6. 策略选择与策略生成分离。
7. 回测计算必须确定性、可复现。
8. 任何“当前最优”都必须带 regime、时间和约束条件。

### Online Strategy Selection

在线阶段不应该临时重新优化整个策略空间。

正确方式：

```text
Current MarketContext
        ↓
Regime / Environment
        ↓
ApprovedStrategy Set
        ↓
Strategy Selector
        ↓
Selected Strategy / Policy
        ↓
Research / Trading Pipeline
```

也就是说：

- Offline Loop 负责“研发和验证策略”
- Online Loop 负责“在已批准策略中选择当前更合适的方案”

这可以避免实时环境下反复调参导致数据追逐和过拟合。

---

## 11. Context 是组织交接物

Aegis 不把 Agent 之间的主要通信建立在长篇自然语言 report 上。

优先：

```text
Typed Context
+
Human-readable Summary（可选）
```

例如：

```text
TechnicalContext
├── deterministic facts
├── evidence
├── risk
├── confidence
└── summary (optional)
```

结构化字段用于：

- 下游 Agent
- Graph
- Validator
- 存储
- 回测
- 可观测性
- 自动化测试

Summary 用于：

- 人类阅读
- Agent 高层理解
- 日志 / 报告

不能只留下 summary 而丢掉结构化事实。

### Context 层级

未来 Context 也按组织层级演进：

```text
Analyst Contexts
├── MarketContext
├── TechnicalContext
├── EventContext
├── FundamentalContext
└── CapitalFlowContext
        ↓
Research Artifact
└── InvestmentThesis
        ↓
Trading Artifact
└── TradeProposal
        ↓
Risk / Portfolio Artifacts
├── RiskAssessment
└── PortfolioDecision
```

这避免最终 Agent 一次吞掉无限多低层 Context。

---

## 12. Capability / Provider：组织结构与数据源彻底解耦

Capability 表达“Role 需要什么能力”。

例如：

```text
Technical Analyst
├── StockSnapshot
├── PriceHistory
├── AdjustmentFactor
├── TradingCalendar
└── TechnicalIndicators

News/Event Analyst
├── CompanyNews
├── Announcements
├── PolicyNews
├── ArticleFetch
└── SiteSearch
```

Provider 表达“能力从哪里来”。

```text
PriceHistory
├── Tushare
├── DuckDB
├── MCP
└── HTTP API

CompanyNews
├── News API
├── Crawler
├── MCP
└── Local DB
```

Agent / Role 不应该把具体 Provider 写入核心业务 Prompt 或 Context。

如果需要审计来源，可以保存 provenance 元数据，但它不改变组织职责。

### Provider 抽象时机

继续遵守：

```text
第一套真实实现
    ↓
直接跑通
    ↓
出现第二套真实实现
    ↓
观察重复
    ↓
再抽象 interface / routing / fallback
```

不因为长期可能有很多 Provider 就提前建立 ProviderManager / Registry / Factory。

---

## 13. Skill / MCP / Crawler / API / DB 的正确位置

### Skill

Skill 表达方法论、工作流、知识和可执行辅助脚本。

```text
Agent
  ↓
Skill
  ↓
Tool / Script / Reference
```

Skill 不是 Agent Role，也不等于 Provider。

### MCP

MCP 是 Tool / Capability 的接入协议之一。

### Crawler

Crawler 是 Web 获取能力的实现。

只有当“Web Research”本身出现独立、复杂、可复用的研究职责，例如多轮搜索、追原始来源、引用验证、跨站点交叉核验，才考虑 Web Research Role / Agent。

### API / Database

API、DuckDB、MySQL、本地文件等都是基础设施来源。

它们解决：

```text
数据从哪里来
```

而 Agent / Team 解决：

```text
谁负责研究什么
```

---

## 14. 确定性计算与 Agent 推理边界

确定性计算不能交给 LLM 临场发挥。

```text
Raw Data
   ↓
Normalize
   ↓
Deterministic Calculation
   ↓
Typed Facts
   ↓
Analyst Interpretation
```

例如：

```text
PIT_QFQ
MA
RSI
MACD
收益率
单位转换
交易日窗口
```

由确定性代码负责。

Agent 负责：

- 决定需要哪些事实
- 解释事实
- 比较证据
- 识别冲突
- 形成职责范围内的判断

确定性代码可以位于 Skill script、Go package、服务或 Capability 实现中；物理位置不是核心。

---

## 15. Graph 是公司的工作流

Graph 不是“Agent 数组”，而是组织流程。

它负责：

- Team / Role 的执行顺序
- Context / Artifact 的交接
- 并行与汇聚
- 真实需要时的 debate / review
- 生命周期
- 错误传播

它不负责：

- 数据清洗
- 指标计算
- Provider 路由细节
- 业务结论本身

### 当前 V2.5

```text
START
  ↓
Market Agent
  ↓
MarketContext
  ↓
Trading Agent
  ↓
Decision
  ↓
END
```

### V2.6 当前演进

```text
START
  ↓
Market Agent
  ↓
Technical Agent
  ↓
Trading Agent
  ↓
END
```

这是为了先验证第二个职责型 Agent 和 typed Context，不代表长期公司结构只有三个 Agent。

### 长期

Aegis 有两条 Graph：

```text
Online Decision Graph

Analyst Team
     ↓
Research Team
     ↓
Trading Team
     ↓
Risk / Portfolio Team
```

以及：

```text
Offline Evolution Graph

Historical/PIT Data
     ↓
Strategy Team
     ↓
Backtest / Validation Team
     ↓
ApprovedStrategy
     ↓
Strategy Selection Policy
```

两条 Graph 通过版本化 Strategy Artifact、ValidationReport 和生产反馈连接。

并行、debate、manager、自动策略搜索节点只有真实需要出现时再进入 Graph。

---

## 16. TradingAgents 给 Aegis 的参考

TradingAgents 的核心价值不是 Agent 数量，而是组织分层：

```text
Analysts
  ↓
Bull / Bear Researchers
  ↓
Research Manager
  ↓
Trader
  ↓
Risk Analysts
  ↓
Portfolio Manager
```

并且不同 Analyst 有各自 Tool 集，Tool 下方再进行 vendor routing。

Aegis 借鉴：

1. 按职责建立组织角色。
2. Analyst / Research / Trading / Risk 分层。
3. 每个 Role 拥有自己的 Tool / Capability 集。
4. Graph 负责组织协作。
5. Trader 不负责亲自完成所有数据研究。
6. Provider 位于 Tool / Capability 下层。

Aegis 不直接照搬：

1. 不以长篇 report string 作为主要内部契约。
2. 优先 typed Context / Artifact。
3. 不立刻引入 Bull/Bear debate。
4. 不立刻引入 Risk debate。
5. 不在没有 PortfolioContext 时创建 Portfolio Manager。
6. 不为了模拟公司而创建没有真实工作的 Agent。
7. 不绑定 TradingAgents 的 yfinance / AlphaVantage 数据模型。

---

## 17. 扩展新 Agent 的判定规则

新增 Agent 前必须回答：

### 是否是独立 Role？

- 有独立职责吗？
- 有独立输入吗？
- 有稳定输出吗？
- 可以单独验证吗？
- 独立 Prompt / Model / Capability 集是否有意义？

如果只是新增数据源，答案通常是否，不创建 Agent。

### 是否应该形成新 Context？

当该 Role 的结果需要被其他 Role 稳定消费、存储、测试或回测时，形成 typed Context / Artifact。

### 是否应该形成新 Team？

只有多个 Role 已经围绕同一个更高层目标协作，并存在明确的团队级交付物时才形成 Team。

### 是否应该进入 Graph？

先独立跑通：

```text
Role/Agent
  ↓
Context
  ↓
Validate
  ↓
Integration Test
```

再接 Graph。

---

## 18. 明确禁止的走偏方式

### 禁止 1：超级 Trading Agent

```text
Trading Agent
├── 查行情
├── 算技术指标
├── 爬新闻
├── 查财务
├── 查资金
├── 做 Bull/Bear
├── 做风控
└── 做最终组合决策
```

这是单体 Agent，不是组织级 Multi-Agent。

### 禁止 2：数据源 Agent

默认禁止：

```text
Tushare Agent
MCP Agent
Crawler Agent
Database Agent
```

### 禁止 3：所有 Context 永久直连最终 Trader

当 Analyst 数量增长后，应引入 Research 层汇总，不让 Trader 无限膨胀。

### 禁止 4：自然语言 report 替代 typed Context

Report 可以作为 Summary，但不是唯一协议。

### 禁止 5：LLM 重复确定性计算

已有公式实现后，不允许 Agent 临时再写一套。

### 禁止 6：策略部门自己给自己打分

禁止 Strategy Team 同时承担策略设计、样本选择、回测验证和上线批准。策略提案与独立验证必须分开。

### 禁止 7：实时无限调参

Online Decision 不在当天数据上持续搜索参数来追逐“最优”。参数研发和策略筛选属于 Offline Evolution Loop。

### 禁止 8：先实现组织框架再找职责

当前不创建：

```text
TeamRegistry
RoleRegistry
AgentFactory
OrganizationManager
ProviderRegistry
```

除非真实代码已经出现重复并需要它们。

---

## 19. 当前实现策略

长期架构确定，不等于一次性开发。

当前仍按最小增量演进：

```text
V2.5
Market Agent
  ↓
Trading Agent

V2.6
Market Agent
  ↓
Technical Agent
  ↓
Trading Agent

后续
+ News/Event Analyst
+ Fundamental Analyst
+ Capital Flow Analyst

独立并行建设
+ Strategy Team
+ Backtest / Validation Team
+ StrategySpec / ValidationReport
+ Regime-aware Strategy Selection

研究维度足够多以后
  ↓
Research Team
  ↓
Bull / Bear / Research Manager

出现真实交易方案需求以后
  ↓
Trading Team

出现 PortfolioContext / 风险预算以后
  ↓
Risk / Portfolio Team
```

V2.6 当前实现状态：

```text
Technical Agent                         ✅
TechnicalContext                       ✅
stock_technical_research Capability    ✅
Technical 独立真实 integration          ✅
Market → Technical → Trading Graph     ✅
Trading 只消费结构化 Context            ✅
Trading research capability isolation ✅
Trading 独立 integration               ✅
Hosted V2.6 Research Pipeline          ✅（测试已落代码）
Full Market Agent E2E                  ⏳
```

V2.6 最后的验收动作不是继续增加 Agent，而是在具备本地 `skills/` 的真实环境中运行：

```text
Market Agent
  ↓
MarketContext
  ↓
Technical Agent
  ↓
TechnicalContext
  ↓
Trading Agent
  ↓
Decision
```

Hosted integration 通过即可证明 V2.6 新增职责拆分与 Graph 交接成立；完整 Market Agent E2E 通过后，可以完成系统级收口。

之后再进入下一个真实研究维度；按当前组织模型，优先候选是 News/Event Analyst，但只有在其 Context、Capability 和独立测试需求明确后才开始实现。

不要因为长期目标是“虚拟投资公司”，就跳过中间可验证阶段。