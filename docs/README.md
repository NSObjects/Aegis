# Aegis 文档

> 当前实现状态更新时间：2026-09-24

Aegis 当前已经完成一条可运行的 V2.5 A 股研究主链路：

```text
Config
  ↓
LLM Manager
  ↓
Runtime
  ├── Skill
  ├── Filesystem / execute
  └── Context
  ↓
Market Agent
  ↓
MarketContext
  ↓
Research Workflow / Eino Graph
  ↓
Trading Agent
  ↓
Decision
```

这只是**当前已实现链路**，不是最终职责划分。

V2.6 起，长期架构调整为**组织级 Multi-Agent**，目标是逐步形成真实投资研究公司的分工：

```text
Analyst Team
├── Market Analyst
├── Technical Analyst
├── News/Event Analyst
├── Fundamental Analyst
└── Capital Flow Analyst
        ↓
Research Team
├── Bull Researcher
├── Bear Researcher
└── Research Manager
        ↓
Trading Team
└── Trader
        ↓
Risk / Portfolio Team
├── Risk Analyst
└── Portfolio Manager

Offline Evolution
├── Strategy Team
└── Backtest / Validation Team
```

其中 Strategy Team 负责提出和升级策略，Backtest / Validation Team 独立验证，形成版本化 ApprovedStrategy，再供在线决策流程选择使用。

当前不会一次性实现这些角色。V2.6 只先完成 Technical Agent，并继续验证 typed Context + Graph 协作。策略/回测部门属于长期并行演进方向，尚未实现。

## 当前权威代码

| 模块 | 状态 | 说明 |
|---|---|---|
| `cmd/aegis` | 已实现 | 正式应用入口，负责配置加载与组件组装 |
| `internal/config` | 已实现 | YAML 配置、默认模型与 per-agent profile 选择 |
| `internal/llm` | 已实现 | 当前仅支持 DeepSeek；包含 DSML → FunctionToolCall 兼容层 |
| `internal/middleware` | 已实现 | Context / Skill / Filesystem |
| `internal/runtime` | 已实现 | 创建共享 local backend 并装配 middleware |
| `internal/agent/market` | 已实现 | 真实市场环境研究，输出 `MarketContext` |
| `internal/agent/trading` | 已实现（待 V2.6 收窄职责） | 当前消费 `MarketContext` 并自行补个股研究；目标是只综合各 Domain Context |
| `internal/agent/technical` | V2.6 进行中 | TechnicalContext + Validate + Technical Agent 单测与真实 integration 已通过；下一步验证确定性 Capability 调用路径后接 Graph |
| `internal/research` | 已实现 V2.5 | 当前 Graph：Market Agent → Trading Agent；V2.6 将升级为 Market → Technical → Trading |
| `internal/decision` | 已实现 | WATCH / AVOID / CANDIDATE 输出契约 |

## 当前明确不包含

以下能力已经从当前运行路径删除，后续需要时再重新引入：

- V1 mock tools（market / technical / news / overview）
- `internal/skillruntime`
- MCP runtime 与 `configs/mcp.json`
- scenario mock
- 旧 MarketContext / 旧 Market Prompt
- OpenAI / Claude / Qwen / Gemini / Ark provider adapter

当前唯一实际支持的模型供应商是 **DeepSeek**。

## Runtime

Runtime 只负责共享运行资源和 middleware 装配：

```text
Runtime
├── Skill Middleware
├── Filesystem Middleware
└── Context Middleware
```

Model 不属于 Runtime。不同 Agent 通过配置选择 profile，再由 `llm.Manager` 获取模型实例。

## Multi-Agent 架构边界

后续新增 Agent / Tool / 数据源前，必须先阅读：

- `docs/10-多Agent架构与职责边界.md`

核心规则：

```text
Team        = 共同完成某类组织目标的一组 Role
Role        = 稳定组织职责
Agent       = Role 的具体运行实例
Context     = Role / Team 间结构化交接物
Capability  = Agent 完成职责所需能力
Provider    = Capability 的具体来源
Graph       = 公司的工作流
```

长期有两条主要工作流：

```text
Online Decision Loop
→ Analyst / Research / Trading / Risk

Offline Evolution Loop
→ Strategy / Backtest / Validation / ApprovedStrategy
```

Skill / MCP / API / Crawler / DB / Tushare 都不是 Agent 职责分类。

**重要：Team / Role 当前首先是架构概念，不代表已经存在 TeamRegistry / RoleRegistry 等运行时抽象。只有真实重复出现后才实现框架层类型。

## Skill

Skill 根目录由运行参数指定，默认：

```text
./skills
```

`skills/` 被 Git 忽略，真实 Tushare Skill 和凭证不进入仓库。

普通依赖可以由 Skill 通过 `execute` 安装；Token、账号、付费权限和系统权限属于外部条件。

## Model

当前配置：

```yaml
default_model: default

models:
  default:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY

  strong:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY

agents:
  trading:
    model: strong
```

`default` 与 `strong` 是 profile，不代表必须是不同 Provider。

## DeepSeek DSML

真实集成测试中出现过 DeepSeek 把工具调用作为 DSML 文本返回，而不是标准结构化 tool call。

当前在 `internal/llm/dsml.go` 做 Provider 边界兼容：

```text
DeepSeek DSML
    ↓
DSML Normalizer
    ↓
schema.FunctionToolCall
    ↓
Eino ADK ReAct / ToolsNode
```

Agent 业务层不感知 DSML。

## 真实链路验证

已实际跑通：

```text
Technical Agent
  ↓
Runtime / tushare-data Skill
  ↓
真实行情与技术数据
  ↓
TechnicalContext
  ↓
Validate
```

以及 V2.5 主链路：

```text
Market Agent
  ↓
Tushare Skill
  ↓
MarketContext
  ↓
Eino Graph
  ↓
Trading Agent
  ↓
Tushare Skill
  ↓
Decision
```

端到端测试：

```bash
AEGIS_INTEGRATION=1 \
go test ./internal/research \
  -run '^TestWorkflow_Run_Integration$' \
  -v \
  -count=1
```

## 正式运行

环境变量：

```bash
export DEEPSEEK_API_KEY="..."
export TUSHARE_TOKEN="..."
```

执行：

```bash
go run ./cmd/aegis -symbol 600519.SH
```

历史市场环境：

```bash
go run ./cmd/aegis \
  -symbol 600519.SH \
  -market-query "分析 2026-09-18 A 股市场环境。"
```

## 文档说明

- `docs/10-多Agent架构与职责边界.md`：**最高优先级架构约束**，定义 Agent / Context / Capability / Provider / Graph 边界。
- `docs/08-演进路线与现状.md`：当前实现、已知过渡状态与下一阶段。
- `docs/09-Technical数据口径.md`：Technical Agent / TechnicalContext 的 A 股技术数据语义。
- `docs/01-*.md` ~ `docs/07-*.md`：保留的设计/演进记录，其中部分内容描述历史实现，不作为当前代码状态的权威来源。
- `docs/research/tradingagents/`：外部项目研究与架构取舍记录。

发生冲突时，优先级为：

```text
10-多Agent架构与职责边界
  ↓
README / 08-演进路线与现状
  ↓
09-Technical数据口径
  ↓
历史设计文档
```
