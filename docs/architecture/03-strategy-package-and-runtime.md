# 03. 独立 Strategy Package 与 Runtime 契约

> 以下代码、Manifest、状态字段是**待验证的设计原型**，不是当前已发布的 API/SDK。返回 [索引](README.md)。

## 1. 为什么不能只有一个 Strategy.Run() → Decision

TradingAgents 式研究回答“如何判断给定标的”，趋势回踩通常“扫描股票池有哪些候选”，仓位策略需要“给定账户状态如何进行研究性风险预算”。它们的流程、输入、结果完全不同。

建议使用**统一 Run Envelope + 分类型业务契约**：
- `research`：标的/主题研究、论点、证据、风险、未知项；
- `screening`：股票池扫描、候选、过滤原因、覆盖率；
- `signal`：研究性触发信号、失效条件、来源；
- `position_research`：显式账户上下文、风险约束与非执行性仓位情景。

类型名和最终 Schema 应通过两个真实实现测试后再冻结。

## 2. Strategy Manifest（概念示例）

```json
{
  "api_version": "aegis.strategy/v1alpha1",
  "id": "official.tradingagents-research",
  "version": "0.1.0",
  "kind": "research",
  "entrypoint": {"type": "builtin", "name": "tradingagents-research"},
  "input_schema": "schemas/research-input.json",
  "output_schema": "schemas/research-result.json",
  "required_capabilities": [
    {"name": "market.bars", "range": ">=1.0 <2.0"},
    {"name": "news.search", "optional": true},
    {"name": "llm.chat", "range": ">=1.0 <2.0"}
  ],
  "permissions": {"network": [], "filesystem": "none", "credentials": "capability-scoped"},
  "limits": {"timeout_seconds": 600}
}
```

以上只是候选配置，不要在未实施时作为已支持的策略安装格式宣传。社区代码包可能还需 Worker 协议、运行平台与依赖锁信息。

## 3. Run Envelope（协议草案）

```text
RunRequest
  run_id / idempotency_key / caller
  strategy_id / immutable package_version / digest / kind
  requested_as_of / timezone / mode (current, replay, backtest)
  typed input + config fingerprint + granted capabilities

RunRecord
  state: PENDING -> RUNNING -> COMPLETED | FAILED | CANCELLED
  timestamps / stage events / Tool events / sanitized errors
  resolved providers, models, data versions, execution fingerprint
  typed result OR insufficiency / unavailable diagnostics
  artifact references / evidence references / trace references
```

运行状态与研究可信度不同：`COMPLETED` 仅表示任务按协议完成，**不保证收益或研究正确**。对于部分数据/模型失败，结果要能表达 `PARTIAL/INSUFFICIENT/REVIEW_REQUIRED`；不能为了让 UI 显示成功而吞掉缺口。取消和失败不能伪造完整成功产物。

### 生命周期

1. Host 校验调用方、版本与 RunRequest；解析允许的 Capability 和 Provider。
2. 冻结包 digest、输入/config、运行截止时间和预算，创建运行记录。
3. 根据执行级别启动可信内置执行器或隔离 Worker。
4. 策略自行执行独立 Workflow 和 Tool/SDK 调用，产生可追踪事件。
5. Host 校验结果 Schema、权限、时间语义和必需安全诊断。
6. 原子提交终态、Artifact、Evidence 与错误信息，可供历史对比与审计。

策略版本升级不能修改运行中实例或历史结果。多个策略同股票运行，由 Web/Host 独立发起、独立展示，不让它们相互调用。

## 4. 执行接口（示意，不固定 Go SDK）

```go
type Executor interface {
    Run(ctx context.Context, req RunRequest,
        caps CapabilityClient, events EventSink) (TypedOutcome, error)
}
```

此接口说明的是**平台协议边界**，不是要求 Python 第三方作者实现 Go interface。Go builtin 可以进程内执行；第三方代码应通过受限 Worker + 明确的 RPC 协议实现等价语义。CapabilityClient 必须是有权限和预算约束的访问入口，而不是全权限 DB/OS handle。

## 5. 回放、可复现与兼容性

- 每次 Run 固定策略包 digest、配置、模型/Provider 版本与有效时点、数据来源/版本、执行图分支与工具返回引用。
- 区分 **audit replay**（展示原始决策与证据）和 **recompute**（重新运行）；LLM 天然非确定性，再调用不能保证逐字结果相同。
- 对确定性策略可要求固定输入/版本下结果一致；对 LLM 策略要求记录证据和差异，不伪称 bit-for-bit 复现。
- 必须有拒绝类错误：PERMISSION_DENIED、MISSING_CAPABILITY、STALE_DATA、PIT_VIOLATION、BUDGET_EXCEEDED、INVALID_RESULT、PROVIDER_UNAVAILABLE 等候选码，具体命名待确定。
- 不把“所有策略输出同一个 Decision”作为兼容性目标；只统一运行管理和共有研究元数据。

## 6. 独立性与隔离

- 一种策略内部可以通过 Eino Graph 调用多个 Agent，也可以全程用 Go 算法。
- 策略之间不能 `Strategy A -> Strategy B` 直接运行依赖或互相修改状态。
- 可使用共享**官方基础数据及能力**，但访问必须按权限隔离。
- 官方可信 builtin 先可进程内运行；不可信第三方代码默认隔离执行，不能依赖 Go 的原生 `plugin` 动态加载确保安全。

## 7. 关键 PoC

在不把交易研究固定 Graph 塞到 Host、不强制相同输入输出 Schema 的前提下，同时支持：
- `official.tradingagents-research`：Market/News/Technical、多 Agent Debate、ResearchResult；
- `official.trend-pullback`：批量本地行情、指标计算/回踩规则、ScreeningResult。

检验二者是否可以复用 Run 记录、权限、取消、错误与来源追踪；不满足前不得宣称 SDK/Runtime 完整。
