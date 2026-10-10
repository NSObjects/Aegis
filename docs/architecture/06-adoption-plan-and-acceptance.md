# 06. 可行性论证、渐进迁移与验收

> **本文件是长期架构验证门槛，不是第二份开发路线。** 继续开发的唯一入口仍是 [DEVELOPMENT_ROADMAP](../DEVELOPMENT_ROADMAP.md)：当前 R0/V1 收尾，之后 R1 Web 研究工作台优先；后续仓位管理属于大型独立阶段。返回 [索引](README.md)。

## 1. 当前代码映射（2026-10-10）

| 已有目录/逻辑 | 未来可复用价值 | 当前仍缺失 |
|---|---|---|
| `internal/research/workflow.go` | 官方 TradingAgents-style 策略的 Research Graph | 不能当成通用 Strategy Host |
| `internal/research/full_portfolio_workflow.go` | 现有研究→Portfolio Review、风险门禁与结果交接 | 不等于所有策略必须使用的全局 Graph |
| `internal/runtime/runtime.go` | Eino Skill、Filesystem、Context Middleware 的装配 | 通用策略注册、第三方隔离、Run 管理 |
| `internal/capability/event` | Retriever、AcquisitionPlan、Snapshot、来源/时间/证据校验 | 通用 Capability Catalog 需验证再抽 |
| `internal/capability/technical` | TuShare HTTP、日线/复权/技术计算 | 日常全市场本地 DB 采集和批量标准查询尚不能据此视为完成 |
| `internal/research/planning` | 受约束的 Plan/Tool 执行 | 不能变成所有策略的必选 Planner |
| `internal/portfolio`、`portfoliodecision` | 研究性账户/风险诊断基础 | 不等于完整动态仓位策略或真实下单 |

未来应保留现有证据链、PIT、失败语义与 JSON 契约，不为满足新架构而推翻已经通过的 CLI/CI 结果。当前研发的现实重点是先让 R0 闭环可验证，再做真正可用的 R1 Web；不因长期目标改变路线优先级。

## 2. 可行的落地方法：用真实异构策略验证，而非先设计大框架

| 验证单元（未来安排） | 最小交付 | Exit Gate |
|---|---|---|
| A. 共用 Run Service | CLI/Web 共用启动、取消、查询 Run/事件/产物 | 行为等价；取消失败可追踪；CLI 不回归 |
| B. Official Data 查询复用 | 同一行情/事件能力供 SDK 和 Eino Tool 使用 | 数据、来源、PIT、权限和错误一致 |
| C. 两个独立策略 | TradingAgents-style Research 与纯 Go Trend Pullback | 不改 Host 业务 Graph 可独立运行、输出不同 typed results |
| D. Strategy 协议与版本 | Manifest、内容 digest、类型化结果、capability grants | 拒绝缺能力/非法权限，版本/回放可审计 |
| E. Worker 隔离 PoC | 第三方代码独立安全执行 | 隔离密钥、宿主 FS、任意网络与资源开销 |
| F. Builder PoC | 对话生成候选策略并运行验证 | 未验证/未授权不得发布 |
| G. 社区发布 | 安装/版本/回滚/下线治理 | 安全、版权/许可证、数据许可及发布审核可执行 |

这些属于在原开发路线适当阶段插入的技术验证；**不在本 PR 修改原有 R0–R8 的阶段和下一项任务**。当达到真实需求时再发独立 ADR / Issue / PR。

## 3. 必须证明的异构策略场景

```text
Aegis Host
  +-- TradingAgents-style Research
  |      Market / Technical / News Tools
  |      Multiple Eino Agents + bounded Debate
  |      -> ResearchResult
  |
  +-- Trend Pullback Screening
         DB batch OHLCV + Go deterministic indicators/rules
         -> ScreeningResult
```

**通过条件**：
- 策略之间无直接调用、运行状态或错误传播；不同输入和业务 Schema 正常共存。
- 共享 Host 的超时取消、版本与 provenance 记录，运行 Web/CLI 均可检索。
- 日常本地行情查询和在线检索的时间/质量语义一致；纯 Go 策略不被迫使用 LLM。
- 趋势回踩扫描几千只股票走批量查询，无逐股 LLM 工具风暴。
- TradingAgents 式当前路径迁移后不会重复运行研究、破坏 EvidenceLedger/Portfolio Review 校验。
- 数据来源失败会产生显式状态，不能被改成静默成功。

## 4. 故障验证矩阵

| 故障注入 | 期望 |
|---|---|
| 本地 DB 只到前一日但请求盘中最新 | 在线补取或 STALE，禁止以昨收冒充实时 |
| 历史 as_of 早于公告/财报修订 available_at | PIT Gate 拒绝 |
| 一路 MCP 超时、另一路提供互补证据 | 分别保留状态，不能伪造多源一致 |
| 数据源切换后价格/复权口径不同 | 拒绝混用或显式转换与来源记录 |
| 采集任务重复或中断 | 幂等写入与完整性检测，可补采 |
| 策略未授权 Tool/网络/FS | 明确拒绝与记录，不能绕过 Broker |
| Skill 要求 shell 安装或上传密钥 | 不自动提权/执行 |
| LLM 无限重复检索/超预算 | 有界结束，保留 BUDGET/INCOMPLETE 状态 |
| 正在运行的策略被升级 | 当前 Run 仍锁定旧包 digest |
| Worker 崩溃 | 该策略失败，其他策略正常 |
| LLM 重新研究同标的结果措辞不同 | 历史结果可审计，但不宣称逐 token 复现 |
| 账户 NOT_PROVIDED / 证据不足 | 不能伪造成空仓或风控通过 |
| Builder 回测未通过仍尝试发布 | 必须阻断，保留错误证据 |

## 5. 特别需要避免的失败路线

- 在 R0/R1 之前重构全部 `internal/research`，破坏已经跑通的研究链。
- 将 Aegis Data 单独拆库/拆项目，或为每个数据源重写原本已有的 MCP。
- 为了“全部插件化”创建 Registry、Lifecycle、Reflection 等无真实消费者的元框架。
- 把所有 Capability 强行变成 Eino Tool，导致确定性批量任务走 LLM。
- 强制所有 Strategy 输出一个统一的 TradingAgents Decision。
- 安全未验证就允许第三方 Go 插件进入主进程或 Skill 执行任意脚本。
- Strategy Builder 生成即可投产，忽略 PIT、成本、样本外和独立审核。
- 为社区功能提前挤占 Web 工作台和真实研究闭环的优先级。

## 6. 进入开发阶段后再做的 ADR

Manifest 和 RPC 协议、运行状态/typed result schema、首批官方采集数据范围和存储方案、Provider/实时查询策略、Worker 技术选型和跨语言、权限/多租户、历史数据与研究 Memory PIT、Marketplace 许可与审核。

**最终可行性验收问题**：能否让一个新策略作者不改 Core、不复刻全市场数据采集、不复制 TradingAgents 研究 Graph，仅声明能力并实现自己的流程，即可独立完成可追踪、可验证、受控的研究 Run？这个问题由两个异构策略和失败测试回答，不由架构图回答。
