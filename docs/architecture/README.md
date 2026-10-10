# Aegis 长期架构：稳定内核、官方能力与独立策略生态

> **状态：长期目标设计基线（2026-10-10），不是当前已实现功能或立即重构任务。**
> 当前代码、研究性/不接真实交易的产品边界，仍遵循 [现行 Multi-Agent 架构](../10-多Agent架构与职责边界.md)；唯一开发导航是 [DEVELOPMENT_ROADMAP](../DEVELOPMENT_ROADMAP.md)。R0/V1 验收后优先 R1 Web 工作台，不因本方案插队。

## 目标定位

**Aegis 提供稳定的 Strategy Host 和官方基础能力；策略自行组织数据获取、Agent、Tool 与 Workflow。** 当前 TradingAgents 式 Multi-Agent 研究是未来的一种**官方内置 Research Strategy**，而非强制所有策略采用的内核流程。选股、事件驱动、仓位研究可有完全不同的实现。

```text
Web Workbench / Conversation / CLI / Strategy Builder (future)
                           |
                     Aegis Host
   Strategy Registry / Runs / Isolation / Permission / Trace
                           |
                 Official Capabilities
  Data (scheduled collection + local DB + on-demand online)
  Eino / LLM / Tools / MCP / Skills / Evidence / Memory / Validation
                           |
       +-------------------+---------------------+
       |                   |                     |
 TradingAgents-style  Trend Pullback        Community Strategy
  Multi-Agent Graph  Deterministic Go       Own workflow/runtime
       |                   |                     |
     ResearchResult     ScreeningResult     Typed Result
```

图是**逻辑职责**，不要求每一层独立服务、微服务、仓库或现在建立对应目录。尤其 **Aegis Data 是官方内置 Capability，不能拆为单独产品/项目**。定时采集可以在 Aegis 内部以后台任务运行，不需要通过 LLM。

## 已形成的长期方向

1. **策略自治且相互独立**：每个策略拥有自己的执行过程、数据需求、配置、版本和结果；**策略之间不相互调用/依赖**。可以共享官方只读数据与基础 Capability。
2. **部分插件化**：扩展对象是策略包与确有必要的外部能力适配器；不是“一切皆插件”，不重造 Eino Agent Runtime，也不照搬 Cordis。
3. **官方维护基础数据资产**：交易日历、股票列表、日线/复权等按交易日批量采集、校验、存储与补采；动态信息按需查询。
4. **优先使用服务商已有 MCP 和 Skill**：避免为每家实时数据方重写插件；MCP 是 Tool 接入协议，Skill 是操作说明/流程，不能等同于数据源或数据真实性保证。
5. **Capability 不等于 Eino Tool**：同一能力可以给 Eino Agent Loop 通过语义 Tool 调用，也可以直接给 Go 策略做批量查询。
6. **统一运行协议、不强制统一业务 Result**：研究、全市场筛选、信号、仓位研究的输入输出存在本质差异。
7. **安全与时点由平台保障**：权限、PIT、时间、新鲜度、版本、错误、必需风险诊断，不能交给 LLM 随意决定。
8. **开放社区分阶段**：先证明两种异构官方策略共用 Host，再讨论第三方代码隔离、对话生成策略和 Marketplace。

## 与当前仓库的关系

- [现行权威架构](../10-多Agent架构与职责边界.md) 的 Team/Role/Graph 在当前阶段继续约束已实现的官方研究链；**长期不再要求所有未来策略照此组织**。其永久研究辅助、不接券商下单边界不变。
- `internal/research/workflow.go`、`full_portfolio_workflow.go` 及相关 Agent 保持现有链路，不立即迁移；后续可逐步封装为内置 TradingAgents-style 策略，通过兼容性测试证明不回归。
- `internal/runtime/runtime.go` 当前主要装配 Eino Middleware，**不是已实现的通用 Strategy Host**。
- `internal/capability/event` 的 Retriever、AcquisitionPlan 和 SnapshotRetriever，以及 `internal/capability/technical` 的 TuShare/技术计算是候选复用起点。
- 本文不更改当前 [DEVELOPMENT_ROADMAP](../DEVELOPMENT_ROADMAP.md) 的状态、顺序、V1 收口和 R1 Web 工作台优先级。

## 专题文档

| 文档 | 内容 |
|---|---|
| [01 Core 边界](01-core-and-boundaries.md) | Host/Capability/Strategy 职责，独立性与业务边界 |
| [02 官方能力与数据](02-official-capabilities-and-data.md) | 本地数据、每日采集、实时数据、MCP/Skill、PIT、混合查询 |
| [03 策略包与协议](03-strategy-package-and-runtime.md) | 策略类型、Manifest、Run Envelope、执行与版本 |
| [04 Eino 与工作流](04-eino-and-workflows.md) | Agent Loop、Tool、Graph、确定性批量逻辑与调度边界 |
| [05 社区与安全](05-community-builder-and-security.md) | Strategy Builder、第三方 Worker、权限、审查、Memory |
| [06 渐进落地验收](06-adoption-plan-and-acceptance.md) | 现状、PoC、验收、风险、待定 ADR |

## 设计状态

上述原则是**目标**；Manifest 的字段、Go/RPC SDK、沙箱技术、策略结果 Schema、数据源选择算法和社区审核均**未冻结**。每项需真实用例、测试和 ADR 才能进入正式契约。策略能运行不代表有投资价值；平台不执行真实交易。
