# Aegis 文档

> 当前实现状态更新时间：2026-09-24

Aegis 当前已经完成一条可运行的 A 股研究主链路：

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

## 当前权威代码

| 模块 | 状态 | 说明 |
|---|---|---|
| `cmd/aegis` | 已实现 | 正式应用入口，负责配置加载与组件组装 |
| `internal/config` | 已实现 | YAML 配置、默认模型与 per-agent profile 选择 |
| `internal/llm` | 已实现 | 当前仅支持 DeepSeek；包含 DSML → FunctionToolCall 兼容层 |
| `internal/middleware` | 已实现 | Context / Skill / Filesystem |
| `internal/runtime` | 已实现 | 创建共享 local backend 并装配 middleware |
| `internal/agent/market` | 已实现 | 真实市场环境研究，输出 `MarketContext` |
| `internal/agent/trading` | 已实现 | 消费 `MarketContext` + 个股数据，输出 `Decision` |
| `internal/research` | 已实现 | Eino Graph：Market Agent → Trading Agent |
| `internal/decision` | 已实现 | WATCH / AVOID / CANDIDATE 输出契约 |

## 当前明确不包含

以下能力已经从当前运行路径删除，后续需要时再重新引入：

- V1 mock tools（market / technical / news / overview）
- `internal/skillruntime`
- MCP runtime 与 `configs/mcp.json`
- scenario mock
- 旧 MarketContext / 旧 Market Prompt
- OpenAI / Claude / Qwen / Gemini / Ark provider adapter
- JetBrains `.idea` 项目文件

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

- `docs/08-演进路线与现状.md`：当前实现与下一阶段。
- `docs/01-*.md` ~ `docs/07-*.md`：保留的设计/演进记录，其中部分内容描述历史实现，不作为当前代码状态的权威来源。
- `docs/research/tradingagents/`：外部项目研究与架构取舍记录。

当前代码状态以本文件和 `docs/08-演进路线与现状.md` 为准。
