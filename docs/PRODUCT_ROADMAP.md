# Aegis 产品与研发总路线图（Product Roadmap）

> 创建时间：2026-10-10。制定依据：与产品负责人的讨论、仓库权威架构及当日主分支代码/PR 核查。
>
> 代码基线：main commit 41c32faf59b0a92e26de8311705ed16754c1c3c4（合并 PR #122）；当时 PR #123 仍为开放状态。这里是**规划与状态快照**，不是持续自动更新的进度表，不声称当前或未来 CI 已通过。
>
> **版本口径**：本文件的「产品 V1、V1.1、V1.2、V1.5、V2」是产品交付里程碑；仓库已有的 V2.13 等是工程迭代编号，两者不冲突，不对已有代码重新编号。
>
> **规范优先级**：任何冲突以 [10-多Agent架构与职责边界](10-多Agent架构与职责边界.md) 为准；当前多 Agent 收尾按 [MULTI_AGENT_ROADMAP](MULTI_AGENT_ROADMAP.md) 的 F0～F4 执行。本文件统筹后续产品能力与里程碑，不替代底层设计、接口 ADR 或 Issue。

## 1. 产品定位与北极星目标

Aegis 是**面向 A 股的 AI 原生投资研究与决策辅助工作台**。核心价值不是堆叠 Agent，也不是一个能聊天的行情查询框，而是将下列流程串成可观察、可验证、可持续改进的研究闭环：

~~~text
发现机会 / 用户提问
        ↓
真实市场与公司数据（PIT / 证据）
        ↓
多 Agent 分工研究 → Bull/Bear 论证 → Research Manager
        ↓
TradeIntent（研究性参与意图）
        ↓
Risk / Portfolio → 组合风险与研究性目标仓位建议
        ↓
研究报告 / Web / 对话 / 观察池
        ↓
市场变化监测 → 假设验证 → 复盘反思
        ↓
离线策略研发与独立验证 → 新版本研究策略
~~~

主要用户场景：

1. **按标的研究**：输入股票代码、研究时点及真实可用数据，形成可追溯的研究报告。
2. **对话式探索**：询问「这只股票能否建仓」「和昨天相比哪里变了」「为什么多空意见分歧」；系统选择合适的工具/工作流。
3. **组合研究**：结合明确的账户快照、风险预算及风险限制给出**研究性**仓位情景，不生成真实委托。
4. **主动发现**：从策略规则筛选的股票池发现候选机会，再进行深入研究。
5. **持续追踪**：记录研究假设、催化剂、风险失效条件和后续验证。
6. **评估改进**：将预测、实际观察和决策复盘连接起来，验证策略而非让 LLM 自证有效。

### 永久边界（不可被任何版本规划突破）

- **不接入真实交易执行**：不调用券商下单、委托/成交、自动调仓、订单审批、资金划拨或交易授权接口；回测及 What-if 中的「买卖」仅为离线研究模拟。
- Agent 不得伪造价格、财报、账户、持仓、风控参数、引用或任何数据源；缺失数据明确返回缺失/不确定性。
- 关键金融计算、资金约束、回测收益和最终限制校验由**确定性代码**负责；LLM 负责研究、解释、假设、争议梳理和工具路由。
- 区分 **观察事实 / 模型解释 / 研究假设 / 组合情景 / 验证结果**；所有研究结果保留时点、来源及局限性。
- 输入和持久化历史必须尊重 PIT（point-in-time）；不得用研究时点之后才披露的财报或新闻，或之后发生的价格数据冒充当时已知。
- Team 按职责、Capability 按能力、Provider 按来源拆分；Web、CLI、对话与自动任务应复用同一应用服务，不复制一套研究逻辑。

## 2. 现状与缺口（只描述核实过的代码能力）

| 领域 | 2026-10-10 已有基础 | 仍需验收/补齐 |
| --- | --- | --- |
| 研究链路 | Market / Technical / Event；Fundamental 已通过 PR #115 及后续工作进入主线；Synthesis / Debate / Research Manager / Trading Agent | 在**最新 main** 冻结 SHA、运行实际 Hosted E2E、验证真实来源及全链路结果 |
| 多轮论证 | PR #117、#118 完善 Bull/Bear 对对手论点的回应及端到端约束 | 真实模型对所有目标场景的通过记录；失败时可诊断 |
| 组合研究 | PortfolioContext、可选估值/敞口、限额检查、What-if、Risk Analyst、Portfolio Manager；PR #120 保护共享输入、PR #122 增强终态风险门禁 | 真实账户风险预算、建议目标仓位计算、统一组合级预算 |
| 结果形态 | 完整 Workflow + CLI JSON；证据账本与可选审查/归档 | 持久化运行记录、任务 API、结构化页面 |
| 对话/Web | 尚不作为当前已完成能力 | Conversation Agent、任务 UI、看板、报告、历史 |
| 持续研究 | 部分已有研究/证据产物 | 观察池、主动监控、假设跟踪、研究记忆和效果验证 |
| 策略进化 | 权威架构定义了 Online / Offline 双环 | 策略筛选、独立回测验证、可追溯版本晋升流程 |

> 注意：此表的「已有」不代表完成了所有真实数据集成与运行质量验收；**普通 CI 通过不等于真实 Hosted E2E 通过**。PR #123 的 F4 校验在创建本文件时尚未合并，不能预先标记完成。

### 两条版本轴与交付顺序

| 产品里程碑 | 交付主题 | 状态 |
| --- | --- | --- |
| **产品 V1** | 多 Agent 真实研究闭环 + 基础组合级动态仓位建议 | 多 Agent 收尾中；仓位计算待做 |
| **基础设施 G0** | Application Service + Task/Run + API + 持久化 | 待做；V1.1 前置条件 |
| **产品 V1.1** | Web 研究工作台与历史/组合页面 | 待做 |
| **产品 V1.2** | Conversation Agent + 工具化交互 + 会话上下文 | 待做 |
| **产品 V1.5** | 策略选股、观察池、比较研究、定期报告 | 待做 |
| **产品 V2** | 主动监控、研究假设生命周期、研究 Memory 与反思 | 待做 |
| **产品 V2.x** | 独立回测验证、策略演进、反事实与研究质量评估 | 待做 |

**约束**：先端到端有结果，再完善产品使用体验，最后提升自治与自我改进能力。版本只是目标，不是预设发布日期；未通过 Exit Gate 的里程碑不得宣布完成。

## 3. V1：多 Agent 与组合仓位研究闭环（P0）

### V1-A：完成现有 Multi-Agent 的 F0～F4 收尾

沿用 [MULTI_AGENT_ROADMAP](MULTI_AGENT_ROADMAP.md)，不要重新实现已有角色。

- 固定 main commit SHA；记录完整 Go tests、go vet、可复现 CLI 命令、Hosted Integration run URL 及最终输出结构。
- 覆盖 Market、Technical、Event、可选 Fundamental → Synthesis → 有界 Bull/Bear → Research Manager → TradeIntent → Risk → Portfolio 的同次运行交接。
- 核实 PR #122 当前限制在终态风险审查中的表现，以及开放 PR #123 实际落地结果；不得把 PR 描述当作测试通过证明。
- 区分真实数据失败、模型/枚举失败、工作流接线失败与输入契约失败；缺失来源应有受控错误或不足状态。
- 原有 Evidence Ledger、归档、语义评估仅保持兼容；非阻塞精修推迟。

**验收 Gate A**：有一条正式 CLI + 实际外部模型/数据的成功执行记录；结构化研究产物/辩论/研究计划/意图/风险/组合结论链路完整；同时有一条缺失数据或模型失败的可解释路径；测试、vet、必要 CI 完成并附 SHA/运行 URL。不能用 mock 单测替代真实运行。

### V1-B：组合级 Position Sizing（最小可用版本）

**设计原则：固定风险边界 + 动态仓位计算 + Agent 辅助解释。** 不把「三三制」硬编码成产品唯一规则，也不允许 LLM 给出无约束资金百分比。

1. **可信输入**：复用现有 PortfolioContext 与估值/限额/What-if 能力；按最小增量补充可观察/调用方明确给定的总资产、可用现金、已有敞口、预算与策略约束。金额采用明确单位和整数精度，必须有 as_of/source/完整性状态；未知 ≠ 0。
2. **TradeIntent 关联**：仅在上游候选状态及必要研究数据有效时评估仓位。需要明确风险失效条件/止损研究假设与用于 sizing 的价格基准及时点；缺少必要数值不得自行补出建议比例。
3. **确定性 SizingPolicy V1**：按市场风险等级限定总敞口上限，按单笔风险预算 / 止损距离计算个股研究性仓位，再与可用现金、单股上限、组合现有暴露、已有规则取最严格可行值。该顺序与规则必须版本化、参数化、可解释，不依赖单一未经验证的 LLM 信心值。
4. **组合级裁决**：多个策略/标的共享同一账户预算；单策略各自计算后不得重复使用同一笔现金；保证组合总上限、同一证券的现有仓位以及可能的行业/相关性限制能统一核对。
5. **输出契约**：保留原 PortfolioDecision 的不可执行含义；新增独立研究性 SizingAssessment / AllocationScenario（最终类型名在实现 PR 决定），包含输入快照版本、policy_version、target_weight、current_weight、delta_weight、约束命中、计算解释、无法计算原因与研究时间点。**严禁订单字段、交易授权或执行端点**。
6. **边界行为**：数据缺失 → INSUFFICIENT_DATA / NOT_COMPUTABLE；超限 → BLOCKED/约束诊断；模型结论不能绕过强约束；没有止损假设不强行返回精确仓位；A 股交易单位、涨跌停、停牌、费用仅作为以后离线情景/可行性研究假设，不是下单逻辑。
7. **策略扩展预留**：V1 只实现一种简单规则；等基准测试成立后，V2 才比较固定比例、三三制、ATR/波动率、分批建仓等独立政策，不提前抽象过度通用的策略平台。

~~~text
ResearchArtifact → Debate → ResearchPlan → TradeIntent
                                           │
PortfolioContext + Policy + Market Regime ─┼→ Risk Assessment
                                           ↓
                                 Position Sizing (deterministic)
                                           ↓
                              Portfolio Risk Gate (deterministic)
                                           ↓
                         Final Research Decision + SizingAssessment
~~~

**验收 Gate B**：使用同一个已有 Workflow，不重跑上游研究；真实且完整输入时输出可复算、可解释的目标仓位/差额；账户未知、额度不足、止损无效、持仓超限、价格或时间过期时都保守退出；单元、组合、Workflow 与回归测试验证无执行能力。

**推荐按 4～6 个小 PR 交付**：输入契约与校验 → 确定性仓位计算 → 组合风险裁决接线 → 输出 JSON/CLI → 正反例集成与文档；具体 Issue 以仓库实际代码缺口重新核查为准。

## 4. G0：API、任务执行、持久化（V1.1 前置，P0）

Web 和 CLI **不得复制 Eino Graph**。把 CLI 当前装配代码中可复用的流程提取为应用层接口，但不推翻现有 Workflow/Role 契约。

~~~text
Web / CLI / Scheduler / Conversation Tools
                     ↓
             Application Service
         RunResearch / GetRun / ListRuns
         QueryPortfolio / EvaluateScenario
                     ↓
              Task / Run Service
         retry, cancel, timeout, progress
                     ↓
        Existing Aegis Eino Workflows
                     ↓
             Durable Artifacts / Events
~~~

### G0 最小数据对象（规划草案，非已实现 Schema）

- **ResearchRun**：run_id、请求方/入口、symbol、requested_as_of、config_hash、policy_version、状态、开始/结束时间、失败阶段/代码。
- **AgentRun / StepEvent**：所属 run_id、role、阶段、开始/完成/失败、输入/输出 artifact 引用、耗时、费用（可取时）；不输出凭证和原始敏感内容。
- **ArtifactSnapshot**：schema_version、provenance、source_time、captured_at、content_digest、与运行和上游引用关系；失败的部分结果可保留但不得伪装最终成功。
- **PortfolioSnapshot / RiskPolicy**：账户显式状态、时效与估值、来源及限制版本；与研究运行严格关联。
- **Conversation / Message**：未来会话 ID、消息序列和所引用 run_id / artifact_id；会话记忆不取代金融事实。
- **User / Workspace**：即使是单人部署，也避免未来身份、授权与多个账户数据混淆。

### 候选接口（实施时再定 ADR）

| API | 语义 |
| --- | --- |
| POST /api/research/runs | 创建异步研究任务，返回 run_id，不因一次 HTTP 请求执行耗时 Graph |
| GET /api/research/runs/{id} | 查询状态、错误阶段和概要 |
| GET /api/research/runs/{id}/events | SSE 读取阶段事件；设计可重连/按事件序号补读 |
| GET /api/research/runs/{id}/artifacts | 获取完成阶段的受版本约束研究产物 |
| POST /api/portfolio/scenarios | 显式输入的研究性 What-if / 仓位分析 |
| GET /api/research/history | 按标的/时间/状态查询历史运行 |

- 运行状态至少区分 PENDING、RUNNING、COMPLETED、FAILED、CANCELLED；同一 run_id 的终态不可被另一条请求静默覆盖。
- 每一次状态更新持久化；页面刷新和 SSE 重连仍可得到当前状态及历史事件。重试/幂等要避免重复扣费和重复写入；取消要能传播 context。
- 建议初期 Go + REST + SSE + 简单 Worker；存储可先评估 PostgreSQL，前端可优先 Next.js/TypeScript。**具体数据库、框架、鉴权、迁移/部署方案先立 ADR，不在路线图里假设已采纳**。
- API 输入必须验证授权、参数大小、时点、账户数据归属、输入/输出契约；Web 不能直接拿 API key，也不能接受用户传来的一段总结就当成校验过的研究证据。
- CLI 后续仅作为开发/测试入口继续存在。

**验收 Gate G0**：无需浏览器即可经 HTTP 启动研究并按 run_id 查询进展与最终 artifact；服务重启后可查询既有记录；无权限不能读取他人研究或账户；自动化验证至少覆盖成功、失败、取消/超时、重复请求与断线重连。不要求首版分布式任务队列。

## 5. 产品 V1.1：Web Research Workspace（P1）

Web 是**面向用户的主要入口**，不是 CLI 命令生成器。建议先实现五个页面，全部消费 G0 的同一 API。

| 页面 | 第一版能力 | 不做 |
| --- | --- | --- |
| Dashboard | 今日市场概况、最近任务、观察标的、任务异常/风险概况 | 高度定制化大屏 |
| Research | 输入股票/时点启动运行；可视化 Analyst、Bull/Bear、Risk、Portfolio 状态；事实/证据/争议/结论分层展示 | 在浏览器复制 Agent 逻辑 |
| Portfolio | 账户快照录入/导入校验、现有暴露、组合风险、建议仓位与 What-if 对照 | 券商绑定和真实调仓 |
| Decisions / History | 按时间/标的查询、对比两次研究、查看版本与证据；查看失效或缺失状态 | 仅保存 PDF 摘要而丢弃 typed artifact |
| Settings | 模型/数据源状态、研究参数、RiskPolicy 与任务设置，凭证服务端加密管理 | 任意 Agent 动态注册框架 |

交互要求：

- 同一研究页显示运行阶段、耗时、失败/不足原因；刷新/重连后状态不丢失。
- 结论必须关联研究时点、证据来源、数据新鲜度、模型/规则版本；明确区分事实与观点。
- 多空辩论不仅是大段文字，应可追踪引用、被反驳论点以及未解决争议。
- 组合页面突出**当前仓位 vs 研究性建议仓位**，并解释哪个风险约束限制了结果；未知余额时不画虚假的 0% 仓位。
- 统一响应式组件、暗/亮主题、表格/图表和错误/加载/空状态；UI 规则另建，不在此阶段做品牌细节无休止打磨。
- 前端可以先选 Next.js + TypeScript + 组件库 + ECharts/Lightweight Charts；以 ADR/Spike 确认后才定型。

**验收 Gate V1.1**：仅通过浏览器（不进入服务器终端）完成「启动研究 → 实时查看 → 阅读报告 → 查看历史 → 查看组合情景」；用真实后端 API 跑 E2E；刷新、失败、空数据、缺失账户、窄屏都可用。

## 6. 产品 V1.2：Conversation Agent（P1）

### 对话不是普通问答框

它是**交互协调角色**，理解用户问题并按需调用已有能力；不代替 Market/Technical/Bull/Bear/Risk，也不成为新的事实来源。

~~~text
Chat UI ──→ Conversation Agent (intent, context, tool routing)
                ├─ QueryResearchRun / ReadArtifact
                ├─ StartResearch (完整现有 Multi-Agent Workflow)
                ├─ MarketOrTechnicalLookup (限量、必要时调用)
                ├─ CompareResearchRuns
                ├─ QueryPortfolio / EvaluateScenario
                └─ QueryHistory
                          ↓
                 Same Application Services
~~~

典型交互与能力：

| 用户提问 | 正确系统行为 |
| --- | --- |
| 「研究一下 600519.SH」 | 确定代码/研究时点，提交 ResearchRun，流式展示任务 |
| 「刚才多头和空头争什么？」 | 读取刚才 run_id 的 Debate 和证据，不重新检索所有来源 |
| 「和上次相比发生了什么？」 | 读取两个明确时点的 Artifact，比对新增/失效证据与结论 |
| 「市场转弱，我的仓位如何变化？」 | 获取明确风险策略和账户快照，调用确定性 What-if；数据不足则说明无法量化 |
| 「最近的研究哪里错了？」 | 初期仅查询历史与验证结果；没有后验验证数据不得捏造反思 |
| 「只想看今天行情」 | 调用必要的 Market Capability，不强制跑所有 Agent |

实现约束：

- 对话拥有独立 session_id，但金融研究由 run_id 和显式时点定位；代词「刚才/它/昨天」必须能消歧，交易日与自然日不能混用。
- 采用 Agent-as-Tools / Typed Tools，工具输入输出具有 schema 校验；工具调用与错误写入任务轨迹，防止 prompt 注入诱导绕开边界。
- 明确区分读取历史（可复用）与要求重新研究（必须刷新实际来源）；历史数据带 as_of，不当成今天行情。
- 模型不能通过自然语言「批准」超出 RiskPolicy 的仓位，更不能调用不存在的真实交易执行接口。
- V1.2 只做短期会话上下文、持久化消息和研究引用，**不依赖长期 Memory/RAG 才能上线**。
- 回复保持可追溯：引用哪个 run、哪个 artifact、哪个证据、哪个数据截止时间；工具失败与结论不确定时说明原因。

**验收 Gate V1.2**：可连贯完成上述至少 6 类用户场景；验证跨轮指代、复用已有研究、不必要时不重跑 Graph、缺失账户不计算仓位、工具错误可恢复；人工查看工具 trace 与最终回答一致。

## 7. 产品 V1.5：发现机会与常规研究工作台（P2）

### 7.1 Opportunity Discovery（规则扫描先行）

- 明确可配置 universe：股票板块、停牌/退市/上市天数、流动性、风险警示及研究时点；不要硬编码用户偏好为通用默认。
- V1.5 至少实现一类明确规则的候选扫描（如上升趋势回踩），由确定性行情/技术计算产生候选，Agent 解释候选的证据与风险。
- 每次扫描保存数据截面、策略版本、规则参数、候选列表与淘汰原因；防止用当前成分股回测历史市场。
- 候选不等于建仓指令；想深入研究可从列表创建 ResearchRun。

### 7.2 Watchlist 与横向比较

- 自选/观察池可标记观察理由、有效期、风险失效条件与最近一次研究时点。
- 多标的比较应先统一日期/口径和风险参数，再给出相对研究优先级；不能直接比较未经校准的 LLM confidence 数值。
- 每日/每周研究摘要由已完成研究和已观察数据生成，可追溯原始 run；初期允许手动执行，定时化放到 V2 监控。

**验收 Gate V1.5**：扫描产生可复现的候选集；候选一键进入完整多 Agent 研究；列表可以比较至少两只股票并追溯数据版本；历史重放不读未来信息。

## 8. 产品 V2：主动监测、研究假设、持续研究 Memory（P2）

### 8.1 Monitoring Engine 与主动研究

- 配置触发条件：新公告/重大事件、研究假设失效、价格/量能跨阈值、持仓风险超界、定期复查。
- Provider 采集失败不等于事件不存在；按来源健康度、时间戳和去重键处理事件。
- 触发后先评估是否需要增量数据/部分研究还是完整 Graph；设置频率、并发、费用与冷却窗口，避免风暴式重复触发。
- 告警应给出「观察到什么、此前结论是什么、改变了哪些证据、是否需要人工复核」，不承诺确定获利。
- 按股票/策略/研究假设订阅，保留触发历史、成功/失败/忽略原因。

### 8.2 Thesis Tracker：研究假设生命周期

研究结论拆成机器可验证的假设，而不只是一段报告：

~~~text
ResearchHypothesis
  hypothesis_id / symbol / as_of
  proposition / evidence_refs / counter_evidence_refs
  validity_window / invalidation_conditions
  observable_metrics / evaluation_rule
  status: OPEN | SUPPORTED | REFUTED | INCONCLUSIVE | EXPIRED
  linked_runs / evaluation_artifacts / last_checked_at
~~~

- 假设、观察数据和后验评估分开存储，不覆盖原论断。
- 每次新信息到来比较原假设与新事实；尚未足够观察时为 INCONCLUSIVE，不强行判对错。
- 可在页面和聊天中问「昨天的空头论点还成立吗」「哪条假设被证伪了」。

### 8.3 Research Memory：记住研究经验，而不是记忆聊天文本

**优先级：原始数据/历史研究 → 可验证假设 → 后验评估 → 经验反思 → 检索使用**。

- 第一层（不可改写的事实记录）：ResearchRun、原始来源、证据、时点、策略/模型版本、Portfolio/Decision。
- 第二层（可验证研究状态）：已提出假设、观测条件、结果、后验误差、冲突记录。
- 第三层（学习产物）：错误类型、适用市场、失败案例、可检验的改进建议，标明来源 run/hypothesis 和置信状态。
- 检索时既返回支持案例，也返回反例；记忆仅作为研究线索，**不得覆盖今天的实时数据、确定性风控或真实后验结果**。
- Memory 写入需来源、去重、版本、冲突处理、可撤销及质量评估；自动形成的「经验」不得直接修改策略参数或上线规则。
- RAG/向量库是按检索质量和证据规模引入的可选实现，不把购买向量数据库或新框架当作功能完成。

**验收 Gate V2**：同一股票在多个真实研究时点上能追踪至少一个可复核假设；新信息触发增量复核和可解释变化；系统能回答「以前为什么判断错」且可引用当时的研究与后续观测；无后验数据时不会假装已验证。

## 9. 产品 V2.x：策略回测与独立评估（P3）

依据权威架构，明确分离 **Online Decision Loop** 和 **Offline Evolution Loop**：

~~~text
Strategy Research → StrategyCandidate(versioned)
            ↓
Independent Backtest / Validation
            ↓
ValidationReport (costs, robustness, OOS)
            ↓
REJECT / REVISE / APPROVE
            ↓
Approved Strategy Set
            ↓
Online selector uses current regime (no live re-fitting)
~~~

### 最小验证引擎

- 基于严格 PIT 历史数据和当时股票池重放信号；确定性计算持仓、收益、换手、成本、滑点、停牌、涨跌停、100 股交易单位、资金可用约束；注明模型假设。
- 记录输入数据版本、策略代码/参数版本、实际运行配置、成本与信号生成时点，可完全复现。
- 评估总收益/年化（适用时）、最大回撤、波动、Sharpe（适用时）、胜率、盈亏比、换手和不同市场 regime 下的分布。
- 必须有训练/验证/样本外及 walk-forward 或滚动检验；标记生存者偏差、前视偏差、复权泄露、数据挖掘和多次参数试验影响。
- **仓位策略对照**：在同一入场/退出信号、股票池和成本模型下比较固定比例、V1 风险预算、三三制、波动率仓位等，区分「策略选股效果」和「仓位分配效果」。
- Strategy Team 可提出候选与参数，Backtest/Validation 独立核验；不得自动以最近一段收益最好为理由将参数升级到在线默认。

### 后续增强（不进入首轮开发）

- Agent disagreement：将多空争议拆成可验证的信息缺口，生成下一步研究计划。
- Counterfactual：使用严格当时可得信息，比较「如果保持 30% 而未加仓」的离线情景。
- Sector / thematic / 产业链研究、Capital Flow、Sentiment 等独立 Agent：只有可证明独立信息增益和数据质量时才添加。
- Agent quality evaluation：证据真实性、任务成功率、时延/成本、校准质量、研究结论对后验结果的预测贡献。
- 多数据源降级、专项语义评估、缓存、性能优化、跨机器部署/横向扩容：依据真实瓶颈提出独立 Issue。

**验收 Gate V2.x**：同一数据/参数重跑结果一致；有成本约束的策略对照和未参加调参的样本外验证；策略候选未通过验证时不会成为 ApprovedStrategy，任何研究/回测结果都不触发真实交易。

## 10. 跨阶段架构和非功能约束

### 10.1 角色分层

- **Research Agents**：事实研究、解释、辩论、计划；不直接写组合资金状态。
- **Conversation Agent**：用户意图/任务路由/讲解与资料引用；没有越权能力。
- **Application/Task Service**：创建任务、校验权限、状态与幂等、调度 Workflow、读写事件与工件；与 Web/CLI 解耦。
- **Portfolio & Sizing Engine**：基于账户与约束计算确定性研究性组合情景；风险 Gate 优先于模型建议。
- **Monitoring / Evaluation**：离线或异步处理触发与后验验证；不能静默改变既有历史研究和生产策略。

### 10.2 审计、质量与可靠性

| 领域 | 必须做到 |
| --- | --- |
| 金融事实 | source、published_at / captured_at、requested_as_of、版本；未知单独编码 |
| 模型可靠性 | 校验 JSON/枚举/交接对象；受控重试；失败归因；fail-closed 风险路径 |
| 研究溯源 | run_id、artifact_id、证据引用、模型与 prompt/policy 版本、输入 digest |
| API/任务 | 超时、取消、限流、幂等、错误码；持久化可查询；SSE 断线重连 |
| 数据安全 | API key 仅服务端；鉴权、账户数据隔离；日志脱敏；必要的保留/删除策略 |
| 组合风险 | 输入时效、资金预算、集中度、总暴露、假设失效条件均有严格校验 |
| 测试 | Go unit/vet + Workflow contract + 模拟集成 + 真实 Hosted E2E（按需）+ Web 浏览器 E2E |
| 运营质量 | 任务成功率、阶段失败率、外部源健康度、p50/p95 延迟、模型用量/成本 |
| 交付记录 | 每个功能保存 PR、合并 commit、CI/Hosted run、复现命令与剩余风险 |

### 10.3 不做的事

- 为追求「多 Agent 数量」而复制同一上下文的 Bull/Bear/Risk 空角色。
- 把 Skill / MCP / WebSearch / Tushare 叫成 Agent；把框架改造当业务成果。
- 在 V1 没跑通前大规模实现 Memory、RAG、复杂 UI、自动调参。
- 为绿灯放松验证规则、捏造账户状态、把 mock 测试称为实时验证。
- 引入实际订单/券商执行能力，即使用户在对话中要求「买入」也只生成研究情景。

## 11. 可执行拆分：建议 Issue / PR 序列

每张 Issue 应有：**目标用户场景 / 当前代码基线 SHA / 真实缺口 / 范围与排除项 / 涉及入口与对象 / 验收命令 / 成功与失败场景 / 完成证据**。每个 PR 只完成一个可验证的纵向切片，避免无休止拓展细节。

| 优先序 | 建议 Issue 标题 | 可检查交付物 | 前置 |
| --- | --- | --- | --- |
| 01 | F4：核实 main 的完整真实 CLI、最终 Risk Gate 与失败路径 | Hosted run + SHA + JSON 验收报告；复核 PR #123 | 现有 F0～F3 |
| 02 | Portfolio：账户预算与仓位研究输入契约 | Typed context / 校验 / unknown 语义 / 单测 | 01 |
| 03 | Position：确定性基础 SizingPolicy | 可复算计算 + 不足/上限路径 + 测试 | 02 |
| 04 | Portfolio：组合级预算裁决及 Workflow 接线 | 现有 Decision 不变、独立 sizing assessment、集成测试 | 03 |
| 05 | V1 收口：正式 CLI 输出与真实/缺失数据验收 | 复现命令、报告、完整 CI/Hosted 证据 | 04 |
| 06 | G0：Application Service 与研究任务领域模型 | 统一 RunResearch/GetRun 契约，CLI 复用 | 05 |
| 07 | G0：持久化运行/事件/Artifact + 重启恢复 | Repo、迁移、幂等/取消/重试测试 | 06 |
| 08 | G0：HTTP Research API + SSE | 接口契约、鉴权、集成/断线测试 | 07 |
| 09 | Web：Research 任务页和结构化报告 | 能启动/追踪/查看报告的最小可用页面 | 08 |
| 10 | Web：History、Portfolio、Dashboard、Settings | 五页面可操作 + 浏览器 E2E | 09 |
| 11 | Chat：Conversation Agent + 只读研究工具 | 工具调用、会话/运行关联、trace | 08/09 |
| 12 | Chat：研究启动、比较和 What-if 工具 | 多轮上下文和风控拒绝测试 | 11 |
| 13 | Discovery：第一套确定性策略扫描及观察池 | 可复现候选与研究入口 | 10 |
| 14 | Monitoring：触发、去重、主动研究 | 可控后台运行和事件记录 | 13 |
| 15 | Thesis：结构化假设与验证规则 | 生命周期 API、历史验证、失效事件 | 10/14 |
| 16 | Research Memory：反思与检索 | 有出处、可撤销、含反例的学习记录 | 15 |
| 17 | Backtest：确定性回放与策略验证 | PIT/OOS/成本模型 + 报告 | 13/15 |
| 18 | Sizing：三三制/波动率政策与回测对照 | 同信号条件下的研究性比较 | 17 |

以上是**依赖建议**，不是对当前 PR 状态的自动追踪；实施前先查最新 main，若能力已完成应改为验收或直接跳过，不重复造轮子。01～05 是下一轮重点，不要同时开启 13～18。

## 12. 开发状态与维护方式（防止路线图失真）

状态枚举：

- **PLANNED**：仅规划，未确认实施。
- **IN_PROGRESS**：有对应 Issue/PR，但未通过完整 Gate。
- **IN_REVIEW**：等待 CI/Hosted 或 Review。
- **DONE**：已合并，附固定 SHA、验收与可复现证据。
- **BLOCKED**：外部数据/安全/架构前置条件无法满足，并记录原因。
- **DEFERRED**：明确延后，不能作为现阶段阻塞项。

更新约定：

1. 主路线图**只描述目标与 Gate**，阶段实现细节写在对应 Issue / ADR；不把此文档变成每次细节 PR 都要修改的流水账。
2. 进入某阶段前先复核主分支：哪些已实现、哪些只是文档/PR、哪些运行过真实数据、哪些还依赖外部 API。
3. 完成阶段必须更新**状态小结、合并 SHA、对应验收链接**；未运行绝不打勾。可在阶段 Issue 中维护动态清单，此文件只更新里程碑状态。
4. 同期只推进**一个主阶段**，可并行做独立的缺陷修复；严禁因非阻塞的边界细节长期拖住核心交付。
5. 若用户调整优先级，先更新本路线图及相关 Issue，保留为什么变更的理由；权威架构边界不被路线图覆盖。
6. 每个里程碑的 Definition of Done 至少包含：功能可运行、正常/失败场景测试、可观察的交接产物、文档/示例、与当前 main 兼容、能独立复现，不以「代码已提交」替代验收。

## 13. 现在马上做什么（Next Up）

**第一步（唯一主线）：** 核实最新 main 及 PR #123 实际状态，完成 Multi-Agent F4 Hosted CLI 终态风险门禁的真实执行、错误路径和最终证据。若已经具备完整证据，直接将阶段标记 DONE；否则只修阻止全链路的缺陷。

**第二步：** 创建组合仓位 V1 最小纵向 Issue，优先定义需要由调用方提供的资金预算与失效/止损假设；保留现有账户未知语义和 What-if 契约。

**第三步：** 独立实现可替换但不过度抽象的确定性仓位策略，并接入 Portfolio 最终研究结果。直到 Gate B 满足前，不开始完整 Web 或 Memory 项目。

**成功的短期目标**：用户提供真实可用研究数据、账户和风控配置时，一次任务能够从多 Agent 事实研究走到有来源的结论和**可解释、不可执行**的组合仓位情景；缺失任何核心数据时，清楚解释不能计算的原因。随后立即进入 G0 和 Web V1.1，产品主要通过页面操作，再引入对话。
