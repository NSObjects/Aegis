> **状态说明（2026-09-24）**  
> 本文保留为设计与演进记录，不作为当前实现状态或 Multi-Agent 职责边界的权威来源。Agent / Context / Capability / Provider / Graph 的职责划分以 [10-多Agent架构与职责边界.md](10-多Agent架构与职责边界.md) 为最高优先级；当前代码结构与实现状态以 [README.md](README.md) 和 [08-演进路线与现状.md](08-演进路线与现状.md) 为准。文中出现的 legacy skillruntime、MCP、mock tools、多 Provider 等内容可能描述历史方案或未来方向。

# Aegis 的 Skill 体系与工具运行时

本文档说明 Aegis 的 Skill 体系如何建立在 Eino 的 Tool 与 Middleware 模型之上：Skill 的标准形态、为什么它必须有执行能力、工具能力集如何收敛，以及 `internal/runtime` 这一共享 Harness 层的边界与接口。

内容覆盖 Eino Tool 模型基础、Skill 标准形态、执行能力、落定的最小方案、安全边界的分寸、`internal/runtime` 的接口与文件构成、MCP 与 Skill 的并存关系，以及截至 2026-09-23 的实现状态。

---

## 1. Eino Tool 模型基础

Aegis 的普通工具采用 Eino 的 `tool.InvokableTool` / `tool.BaseTool` 抽象。`InvokableTool` 需要实现两个方法：

```go
Info(ctx context.Context) (*schema.ToolInfo, error)

InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error)
```

两条关键约定：

- **参数以 JSON string 传入**。模型生成的调用参数序列化后通过 `argumentsInJSON` 交给工具，工具自行决定是否 `json.Unmarshal` 解析。
- **返回值是 string**。工具把结果序列化成 string 返回，模型再把这段文本当作工具结果读入上下文。

`ToolInfo` 实际使用的字段是 `Name` / `Desc` / `ParamsOneOf`，它们决定模型看到的「这个工具有什么能力、什么时候该用」。因此 **Tool 描述本身就是 Agent 能力设计的一部分**。

参数 schema 用参数表形式构建（`schema.NewParamsOneOfByParams`，条目字段为 `Type` / `Desc` / `Required`）。**无参数工具的 `ToolInfo.ParamsOneOf` 当前允许直接为 `nil`**，即不强制提供参数 schema。

最小 Tool 实现形态：

```go
package market

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type SnapshotTool struct{}

func (t *SnapshotTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "market_snapshot",
		Desc: "获取指定股票当前的市场快照，包括价格、涨跌幅、成交额等信息。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"symbol": {
				Type:     schema.DataTypeString,
				Desc:     "股票代码，例如 600519",
				Required: true,
			},
		}),
	}, nil
}

func (t *SnapshotTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	return `{"symbol":"600519","price":1450.20,"change_percent":2.31,"turnover":1850000000}`, nil
}
```

需要真正使用参数时，在 `InvokableRun` 内 `json.Unmarshal([]byte(argumentsInJSON), &args)`，失败直接 `return "", err`。

工具只返回原始事实，不返回加工好的判断结论。返回值应是 `{"ma5":1482.30,"rsi14":58.6}` 这类原始字段，而不是 `{"trend":"bullish","recommendation":"buy"}`。分工是「Tool → 事实 → Agent → 解释」。

工具通过 `adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{...}}}` 注册进 Agent。

类型上必须注意：`agenticdeepseek.Model` 实现的是 `model.BaseModel[*schema.AgenticMessage]`，而 `adk.ChatModelAgentConfig` 固定使用 `*schema.Message`。两者是不同的 Go 泛型实例，不能互相赋值，必须走 `adk.NewTypedChatModelAgent[*schema.AgenticMessage]` + `adk.TypedChatModelAgentConfig[*schema.AgenticMessage]`。

---

## 2. Skill 的标准形态

**Skill 是「模型可决策加载的能力包」**：它是一个可安装、可发现的目录单元，而不是数据库里的一段 Prompt 或一段系统提示词。

标准目录结构：

```text
<skill-name>/
├── SKILL.md
├── scripts/
├── references/
└── assets/
```

`SKILL.md` 是入口，由 YAML front matter 与正文组成：

```yaml
---
name: tushare-data
description: 面向中文自然语言的 Tushare 数据研究技能。用于把“看看这只股票最近怎么样”“帮我查财报趋势”“最近哪个板块最强”“北向资金在买什么”“给我导出一份行情数据”这类请求，转成可执行的数据获取、清洗、对比、筛选、导出与简要分析流程。适用于 A 股、指数、ETF/基金、财务、估值、资金流、公告新闻、板块概念与宏观数据等研究场景。
author: tushare.pro
version: 1.1.10
credentials:
  - name: TUSHARE_TOKEN
    description: Tushare Token，用于认证和授权访问Tushare数据服务。
    how_to_get: "https://tushare.pro/register"
requirements:
  python: 3.7+
  packages:
    - name: tushare
  environment_variables:
    - name: TUSHARE_TOKEN
      required: false
      sensitive: true
  network_access: true
---

# tushare-data

把自然语言财经数据请求，转成可执行的 Tushare 数据工作流。
```

Eino 侧解析 front matter 的结构体是：

```go
type FrontMatter struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Context     ContextMode `yaml:"context"`
	Agent       string      `yaml:"agent"`
	Model       string      `yaml:"model"`
}
```

因此 front matter 分两层：

- **Runtime Metadata（Eino 真正读取）**：`name` 是 Skill 唯一标识，也是 `skill` 工具的调用参数；`description` 是发现阶段的唯一依据；`context` / `agent` / `model` 由 Eino 解释。`name` 与 `description` 是最低必需字段。
- **Installation Metadata（仅作元数据保留）**：`author` / `version` / `credentials` / `requirements` 等字段会被 `yaml.Unmarshal` 丢弃，Eino 不消费它们。它们原本应由 `skill_install` 负责校验与处理，而该工具未实现，因此**最终设计中没有任何组件读取这些字段**。

纯方法论 Skill 可以只有 `SKILL.md`，不带 `scripts/`。

Skill 遵循三级 progressive disclosure：

```text
Tier 1  name + description           ← 启动时可见，仅用于匹配
Tier 2  SKILL.md                     ← 命中后通过 skill(name) 加载
Tier 3  scripts/ references/ assets/ ← Skill 指示 Agent 按需读取或执行
```

Eino 的 filesystem Skill Backend 要求传入一个 `BaseDir`，并且只扫描该目录下的**一级子目录**中的 `SKILL.md`：

```text
BaseDir
  └── 一级子目录
       └── SKILL.md
```

它的 `List()` / `Get()` 每次都重新扫描文件系统，而不是启动时一次性缓存。因此运行期间新建的 Skill 目录，在后续 `List` / `Get` 时即可被发现，不一定需要重建整个 Agent。

Skill Middleware 会自动注入一个 `skill` 工具，**不需要手工加入 `Tools`**：

- `Info()` 调用 Backend 的 `List(ctx)`，把每个 Skill 的 `name` 与 `description` 写入工具描述，模型由此得知当前有哪些 Skill 可用。
- 调用 `skill({"skill": "tushare-data"})` 时执行 `Get(ctx, name)`，读取完整 `SKILL.md`。
- `BeforeAgent()` 执行 `runCtx.Instruction += skillInstruction` 与 `runCtx.Tools = append(runCtx.Tools, skillTool)`，把 Skill 指导语与 `skill` 工具注入本次运行上下文。

因此不需要自行实现 `SkillRegistry`。

---

## 3. 为什么 Skill 必须有执行能力

`SKILL.md` 往往只是入口，真正可执行的能力在 `scripts/` 里。「Skill 能被加载」与「Skill 里的脚本能被执行」是两件事：

- 只接 Skill Middleware，只能读到 `SKILL.md`，实现的是 **Skill 的发现与指导层**。
- 常规执行型 Skill 会自带 `scripts/`（例如 `query.py`、`export.py`），需要通过 Shell 真正运行这些代码才能拿到数据。

所以完整的 Skill Runtime 需要三块能力：

```text
Skill Middleware   →  加载 SKILL.md
Filesystem         →  读取 references / scripts
Execute            →  执行 Skill 里的脚本
```

分层语义是：

```text
SKILL.md   →  instruction
scripts/   →  executable resources
execute    →  execution capability
```

Eino 的 filesystem middleware 本身可以注入 `ls` / `read_file` / `write_file` / `edit_file` / `glob` / `grep` / `execute`，但 `execute` 有额外前提：**必须为它配置 `Shell` 或 `StreamingShell`**；`read_file` 只需要 Backend。当前使用的 `local.Backend` 本身实现了 `Execute()`，底层默认走 `/bin/sh`，因此同一个 Backend 可以同时充当文件系统 Backend 与 Shell。

必须坚持的一条设计点：**不要让 Skill Middleware 自己自动执行脚本**。职责划分是「Skill 告诉 Agent 怎么做 / Agent 决定是否需要执行 / Execute 负责真正执行」。

脚本能否跑通还取决于运行时前置条件，与 Skill 是否加载成功无关：需要脚本存在、对应解释器存在（Python / Node / bash）、依赖已安装、环境变量与凭证存在、网络与文件权限允许。

`local.Backend.Execute()` 等价于 `/bin/sh -c <command>`，**不会切换到 Skill 目录**。因此脚本路径必须写成绝对路径（或由 Skill 显式切到自己的 base directory）：

```bash
python3 /path/to/skills/tushare-data/scripts/query.py ...
```

另外，shell 的非零退出码**不会**变成 Go error，而是作为 `ExecuteResponse` 正常返回：

```text
command exited with non-zero code 1
[stderr]: ...
```

因此判断脚本是否真的执行成功不能只看 `[tool:end]`，必须读取 `output` 内容。

按这个模型，Skill 可以是纯知识型（只有 `SKILL.md`）、执行型（带 `scripts/`）、或混合型（有现成 MCP 就走 MCP，需要本地处理就跑脚本）。

---

## 4. 落定的最小方案

### 4.1 固定单一 Skill 根目录

Skill 不散落在各处，也不由 Agent 自行猜路径。Aegis 只使用一个 canonical Skill Root：

```text
~/.aegis/skills/
└── <skill-name>/
    ├── SKILL.md
    ├── scripts/
    ├── references/
    └── assets/
```

规则统一为：所有 Skill 安装到这里、从这里发现、`scripts/` 都从这里执行；Skill 内的相对路径一律以各自 Skill 目录为基准，再由 Agent 据此构造实际执行的绝对路径。**路径规则只有一份。**

不做多级 scope（project skills / user skills / global skills / trust system / package precedence / collision priority），这些对 Aegis 当前没有实际价值。

### 4.2 只开放的能力集

```text
skill       → Eino Skill Middleware
ls          → Eino Filesystem Middleware
read_file   → Eino Filesystem Middleware
glob        → Eino Filesystem Middleware
grep        → Eino Filesystem Middleware
execute     → Eino Filesystem Middleware + local Shell
```

这套能力已足够支持 `SKILL.md` + `scripts/*` + `references/*` 形态的 Skill。

### 4.3 禁用 `write_file` 与 `edit_file`

当前目标只是运行已安装的 Skill，不需要让 Agent 任意修改文件，因此显式关闭这两个工具：

```go
WriteFileToolConfig: &filesystem.ToolConfig{
	Disable: true,
},
EditFileToolConfig: &filesystem.ToolConfig{
	Disable: true,
},
```

### 4.4 Skill Middleware 与 Filesystem Middleware 装配

权威形态是当前可用的 `internal/skillruntime/runtime.go`：Skill 与 Filesystem 共用同一个 `local.Backend`，Skill 根目录指向固定的 Skill Root，`CustomSystemPrompt` 注入通用的 Skill-first 规则。

```go
package skillruntime

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
)

func New(ctx context.Context) ([]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], error) {
	root := "./skills"
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create root dir: %w", err)
	}

	backend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		return nil, fmt.Errorf("create backend: %w", err)
	}

	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: backend,
		BaseDir: root,
	})
	if err != nil {
		return nil, fmt.Errorf("create skill backend: %w", err)
	}

	skillMW, err := skill.NewTyped[*schema.AgenticMessage](
		ctx,
		&skill.TypedConfig[*schema.AgenticMessage]{
			Backend: skillBackend,
			CustomSystemPrompt: func(ctx context.Context, toolName string) string {
				return fmt.Sprintf(`# Skill Runtime

在执行用户任务前，先检查可用 Skill 是否与当前任务匹配。

规则：

1. 如果存在明显匹配当前任务的 Skill，必须首先调用 %q 加载该 Skill。
2. 在相关 Skill 加载完成之前，不要直接使用其他领域工具绕过该 Skill。
3. Skill 加载后，按照 Skill 中定义的工作流、前置检查、脚本、参考资料和执行要求完成任务。
4. Skill 可以使用 filesystem、execute 以及其他可用工具完成任务。
5. 不要重复加载已经加载过的 Skill。

# Skill 运行依赖

当已加载的 Skill 缺少运行依赖时：

1. 先检查当前环境。
2. 如果依赖可以通过现有 execute 能力自行安装，则自行安装。
3. 安装后重新验证，并继续原任务。
4. 不要因为普通依赖缺失就停止任务。
5. 只有凭证、账号权限、付费权限或系统权限等无法自行获得的条件，
   才将其视为外部阻塞。`, toolName)
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create skill middleware: %w", err)
	}

	fsMW, err := filesystem.NewTyped[*schema.AgenticMessage](ctx, &filesystem.MiddlewareConfig{
		Backend: backend,
		Shell:   backend,
		WriteFileToolConfig: &filesystem.ToolConfig{
			Disable: true,
		},
		EditFileToolConfig: &filesystem.ToolConfig{
			Disable: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create filesystem middleware: %w", err)
	}

	return []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
		skillMW,
		fsMW,
	}, nil
}
```

要点：

- **共用一个 `local.Backend`**：`backend` 同时作为 `MiddlewareConfig.Backend` 与 `MiddlewareConfig.Shell`。`local` backend 要求绝对路径的 Skill 根目录。
- **`CustomSystemPrompt`**：注入通用的 Skill-first 规则，不针对任何具体 Skill，因此不需要在 Agent 的 Instruction 里写「必须先调用某个 Skill」。规则涵盖两件事：任务匹配时先加载 Skill；Skill 缺少普通运行依赖时通过 `execute` 自行安装并继续，只有凭证、账号权限、付费权限或系统权限这类无法自行获得的条件才算外部阻塞。
- **`skillBackend.List(ctx)`**：装配阶段可调用一次并打印 `[skill:discovered] name=... description=...`，用于确认发现结果。
- **禁用 `write_file` / `edit_file`**：见 4.3。

### 4.5 挂到 Agent 上的 `Handlers`

```go
agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
	ctx,
	&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        "aegis",
		Description: "Aegis trading research agent",
		Instruction: instruction,
		Model:       dp,

		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},

		Handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
			skillMW,
			fsMW,
		},
	},
)
```

最终 Agent 可见的能力：

```text
skill
ls
read_file
glob
grep
execute

+ MCP Tools
```

### 4.6 验收标准

```text
1. Skill Root 下的 Skill 能被动态发现。
2. Agent 能通过 skill 加载对应 SKILL.md。
3. Skill 引用 scripts/ 时，Agent 能通过 execute 真正执行脚本并拿到 stdout/stderr。
```

期望轨迹：

```text
[tool:start] skill      input={"skill":"tushare-data"}
[tool:start] ls
[tool:start] execute    python3 ...（环境检查）
[tool:start] read_file  SKILL.md / references
[tool:start] execute    python3 ...（真实取数）
[tool:end]   execute    output=真实数据
→ 最终结构化输出
```

该验证之前不做 `skill_install`、不做多目录、不做 Sandbox、不做额外抽象。

---

## 5. 安全边界的分寸

Aegis 的定位是**交易研究 Agent，不是 coding agent**，因此不照搬 coding agent 的防护体系，只保留与定位匹配的最小边界。明确**不做**的三件事：

**不做 `ValidateCommand` 白名单。** 曾设计过在 `local.Config` 上设置 `ValidateCommand`，用回调拒绝空命令以及包含 `;`、`&&`、`||`、`|`、`>`、`<` 等 token 的命令，以禁止 shell 组合操作：

```go
// 已废弃的设计，仅作对照
backend, err := local.NewBackend(ctx, &local.Config{
	ValidateCommand: validateCommand(root),
})
```

这套限制最终被放弃，装配时使用 `&local.Config{}`，不设置任何命令白名单。

**不做 `SkillExecutor` 抽象。** 不引入 `SkillRuntime`、`SkillExecutor`、`SkillRegistry`、`SkillScriptRunner`、`CapabilityResolver`，也不做 `execute_skill_script`。Skill 侧只需要 `Skill loader + read + bash`，即 `Skill = Instructions + Resources`、`Tools = Generic capabilities`，而不是把 Skill 做成一个特殊的可执行插件系统。

**不做 `skill_install` / `skill_create` 工具。** 这两者属于 Skill Lifecycle Management，不属于 Skill Runtime。运行任务只需要 `skill`；`skill_install` / `skill_create` 是额外的自我扩展能力，当前阶段不实现，也不参与 Skill 的发现与执行——发现和执行完全由 Eino Skill Middleware 负责。

安全边界收敛为一句话：**固定目录 + 固定能力集**：

- 只有一个 Skill Root，所有 Skill 从这一处发现、在这一处执行；
- 只开放 `skill` / `ls` / `read_file` / `glob` / `grep` / `execute`，并显式禁用 `write_file` / `edit_file`；
- 不限制 `execute` 只能运行 Python / bash / node——bash 是 Agent 的基础能力，Skill 只是使用它。

**执行隔离属于 Execution Environment / Sandbox 层的问题，不应通过把 Skill Runtime 设计得越来越特殊来解决。** 当前阶段不做 Sandbox。

---

## 6. `internal/runtime` 的边界与接口

### 6.1 抽取的时机与理由

`internal/runtime` 不是在设计阶段预先铺出来的。Market Agent 与 Trading Agent 的组装都需要同一组依赖：

```text
Model
Skill Middleware
Filesystem Middleware
Callback
```

Market Agent 的集成测试里因此出现了一份重复的 Model + Skill + Filesystem bootstrap。**抽象由真实的重复触发**：先在 `internal/agent/market` 内部用临时 bootstrap 把集成测试跑通，确认两个 Agent 都真实依赖同一组能力，再把这段逻辑抽成共享的 Harness 层。

```text
V2.4.1  internal/agent/market          → 独立跑通
V2.4.2  观察 Market / Trading 重复依赖  → 抽 internal/runtime
V2.5    Market Agent → MarketContext → Trading Agent
```

原则是**先跑通，再从真实重复里抽象 Harness**，不是「为了架构而架构」。相应地，`internal/agent/market` 只声明一个很薄的依赖入口，不自己初始化 Runtime：

```go
type Config struct {
	Model model.BaseModel[*schema.AgenticMessage]

	Handlers []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]

	MaxIterations int
}
```

### 6.2 定型接口

```go
rt, err := runtime.New(ctx, runtime.Config{
	SkillsDir: skillsDir,
	TimeZone:  "Asia/Shanghai",
})
handlers := rt.Handlers()
```

对应类型：

```go
package runtime

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	SkillsDir string
	TimeZone  string
}

type Runtime struct {
	handlers []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]
}
```

调用方只做两件事：用 `Config{SkillsDir, TimeZone}` 构造 Runtime，再取 `Handlers()` 交给 Agent 的 `Handlers` 字段。`SkillsDir` 取代硬编码的 Skill 根目录，`TimeZone` 驱动时间上下文注入。`Handlers()` 返回内部 slice 的**副本**，而不是直接把内部 slice 暴露出去。

`New` 的校验契约：`SkillsDir` 为空返回 `"runtime skills directory is required"`；`TimeZone` 为空返回 `"runtime timezone is required"`；`SkillsDir` 经 `filepath.Abs` 归一化（失败返回 `"resolve skills directory: %w"`），`TimeZone` 经 `time.LoadLocation` 解析（失败返回 `"load runtime timezone %q: %w"`）。

`Handlers()` 返回的 middleware 顺序固定为 **Runtime Context → Skill → Filesystem**：

```go
handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
	contextMW,
	skillMW,
	fsMW,
},
```

### 6.3 方法名从 `AgentHandlers()` 更正为 `Handlers()`

早期草稿使用 `runtime.AgentHandlers()`，定型时改为 `Handlers()`，理由是：

> Runtime 本来就是给 Agent 使用的，没必要多一个词。

即 `Agent` 前缀在 `internal/runtime` 包内是冗余信息——Runtime 的消费者本来就是 Agent，方法名不需要重复这个语境。

### 6.4 文件构成与职责

```text
internal/runtime/
├── runtime.go       组装入口，对外只暴露 New + Handlers
├── context.go       运行时上下文注入（时间、目录约定）
├── skill.go         Skill Middleware 装配
├── filesystem.go    Filesystem + Shell 装配
├── time_context.go  TimeZone → 注入「现在是什么时候」
└── mcp/             MCP Server 配置加载 / 连接 / 工具聚合
```

`time_context.go` 以 Middleware 形式实现，暴露 `NewContextMiddleware[M adk.MessageType](location *time.Location)`，内部持有 `location *time.Location` 与可替换的 `now func() time.Time`（后者是测试用 Clock 接缝，让测试无需依赖真实时间）。

其 `BeforeAgent` 把运行时时钟与规则注入 `runCtx.Instruction`：

```text
# Runtime Context

The following values are runtime facts provided by the Aegis Harness.

- current_time: <RFC3339>
- timezone: <location>
```

关键语义：`current_time` 是**运行时时钟**，不是研究请求的目标时间；目标时间由任务意图决定；「当前」「今天」「最近」「昨天」这类相对表达可以它作为参照；显式的历史日期不得被替换为当前时间；交易日、盘中时段、数据完整性这类领域时间判断交给 Skill 或工具。

### 6.5 Runtime 不持有 Model

**Model 属于每个 Agent 的组装配置，Runtime 是共享的 Harness 能力。** Runtime 的字段只有 `handlers`，没有 model 字段；模型由调用方构造后通过 Agent 自己的配置传入。这样 Market / Trading / Reflection 才能各自选择不同的模型供应商，而不是被固定成单一 Provider。

与之配套的是模型 profile 解析：配置给出 `default_model` 与若干 `models`，Agent 可以覆盖；`ModelProfileForAgent(agentName)` 只负责选出 profile，不关心 Provider 差异。

---

## 7. MCP 与 Skill 的并存

Skill 与 MCP 是两条并列的能力来源，语义不同：

```text
Skill = 怎么做        （工作流、前置检查、脚本、参考资料、执行要求）
MCP   = 用什么能力做  （外部服务提供的数据与工具）
```

一个 Skill 可以只用脚本，也可以只用 MCP，也可以两者组合。Skill 激活后继续调用 MCP 工具是允许的——目标不是「Skill 取代 MCP」，而是 Skill 负责指导工作流、Tool 负责执行能力。因此**不写「优先 MCP」或「优先 Skill」的硬编码路由，也不针对具体 Skill 写特殊规则**，只通过 `CustomSystemPrompt` 注入通用的 Skill-first 约束。

根因是「相关 Skill 必须先调用」这条强约束主要写在 `skill` 工具的描述里，而注入 Agent Instruction 的 Skill system prompt 语义较弱；当旁边存在名字非常明确的 `tushareMcp_*` 工具时，模型容易直接选择具体工具。因此用 `CustomSystemPrompt` 显式补强这条通用规则，而不是为某个 Skill 写专用规则。

在把 Skill 链路跑通的阶段，MCP 被**暂时**从测试 Runner 中移除：

- 目的不是废弃 MCP，而是让 smoke test 只保留 `skill` / `ls` / `read_file` / `glob` / `grep` / `execute`，防止 Agent 绕过 Skill 自身的数据执行路径去走更省事的 MCP 捷径。
- 只有当 MCP 不参与时，才能判断 Skill 是否真的通过 `execute` 完成了「环境检查 → 读取资源 → 执行脚本 → 访问外部 API → 处理错误 → 继续推理」这条完整链路。
- 同一阶段还必须移除同为 Mock 的工具（`market_overview` / `technical_indicators` / `news_search`）：它们会返回与 Skill 实际计算结果严重冲突的数据（例如 MA5 = 1482 对 MA5 = 1261.48），即使 Agent 识别出冲突，也仍然污染了上下文。
- 干净的验证条件是：没有 Mock 数据、没有 MCP、数据全部来自 Skill 通过 `execute` 自行获取、单个接口无权限时 Agent 继续使用已有数据而不是整轮退出、最后仍能产出结构化结果。

Skill 链路验证通过后，再把 MCP 放回来研究两者的协同。

---

## 8. 当前状态（截至 2026-09-23）

**可用的 Skill 运行时在 `internal/skillruntime/runtime.go`**（107 行），由 `cmd/aegis/main.go` 使用。它就是本文 4.4 节的权威形态：`local.NewBackend` → `skill.NewBackendFromFilesystem` → `skillBackend.List(ctx)` 打印 `[skill:discovered]` → `skill.NewTyped`（带 `CustomSystemPrompt`）→ `filesystem.NewTyped`（`Shell: backend`，禁用 `write_file` / `edit_file`），返回 `[]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{skillMW, fsMW}`。

该实现的 Skill 根目录目前**硬编码为 `root := "./skills"`**；基于 `~/.aegis/skills` 的 `os.UserHomeDir()` 版本在源码中被注释保留。这正是 `internal/runtime` 需要收口的地方。

`internal/runtime` 的状态：

| 文件 | 状态 |
|---|---|
| `internal/runtime/runtime.go` | 15 行。已声明 `Config{SkillsDir, TimeZone}` 与 `Runtime{handlers}`；**没有 `New()` 构造函数，也没有 `Handlers()` 方法** |
| `internal/runtime/context.go` | 仅一行 `package runtime`，空壳 |
| `internal/runtime/skill.go` | 仅一行 `package runtime`，空壳 |
| `internal/runtime/filesystem.go` | 仅一行 `package runtime`，空壳 |
| `internal/runtime/time_context.go` | 51 行，已实现。`NewContextMiddleware(location)` 与 `BeforeAgent` 可用 |
| `internal/runtime/mcp/` | 已实现：`config.go` 29 行 / `server.go` 85 行 / `manager.go` 52 行 |

即：**Runtime 文件已建、类型已定义，构造函数与对外方法尚未实现**；`time_context` 与 `mcp` 两个子能力已经落地。

`internal/agent/market/integration_test.go` 仍带临时 bootstrap，等待接入 `rt.Handlers()`；`internal/agent/trading/agent.go` 仍是空壳。

`internal/runtime` 的目标是吸收 `internal/skillruntime` 中已经跑通的装配逻辑，同时去掉 `./skills` 硬编码，改为走 `Config{SkillsDir, TimeZone}`。

---

[← 返回文档索引](README.md)
