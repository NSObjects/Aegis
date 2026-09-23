# Aegis 的 Agent 设计与组装

本文说明 Aegis 中一个 Agent 如何被定义、组装、运行与校验：先给出通用组装原则与依赖注入边界，再以 **Market Agent** 为主线给出包结构、`Context` 契约、`Validate` 规则、Prompt 正文、Runner 选型与组装代码，最后给出 **Trading Agent** 的 V1 输出契约。

结构体、函数与 Prompt 以仓库当前实现为唯一事实来源；仅存在于设计阶段、尚未落地的部分会在小节内显式标注。

---

## 1. Agent 的设计与组装原则

### 1.1 Agent 只是当前版本的一个 Agent Definition / Composition

`internal/agent/<name>/` 里的 Agent 不是运行时本身，而是**当前版本对某个业务角色的一次组装结果**：它声明「这个 Agent 是什么」，不承担「运行一个 Agent 需要什么」。

```text
Agent Definition / Composition        Shared Harness
├── identity   角色与名称              model adapter
├── prompt     职责与行为约束          skill runtime
├── contract   结构化输出契约          filesystem / execute
├── validator  结构合法性校验          callbacks
└── config     Model / Handlers        agent loop / session
```

### 1.2 Prompt、Skill、Strategy、Memory、输出契约保持可替换、可版本化

为了让后续的「成长机制」有接口可挂，这些层必须可替换、可版本化，而不是写死进运行流程：

```text
Execution → Outcome → Reflection → Proposal → Evaluation → Promotion
                                                          ↓
                              新的 Agent / Skill / Strategy 版本
```

具体到代码边界：`instruction`、`Handlers`、Skill、Strategy、Model 都应能由 Agent Definition 动态产生，而**不需要修改 `Analyze()` 这个业务契约**——变的是组装参数，不变的是 `Analyze(ctx, query)`。

### 1.3 `main` / Graph 只负责编排

`main` 或 Graph 负责「什么时候做什么」，Agent 负责「面对当前信息应该怎么判断」，两者不可互相侵入：

- `main` 只做**应用入口与顶层调度**：读配置、建共享 Runtime、按 Agent 组装、发起调用。
- Graph 只承载**需要显式编排的确定性流程**；流程固定处用 Graph，自主判断处用 ADK Agent，取数据用 Tool，只有多 Agent 自由协作才考虑 Supervisor / Host。
- Agent 自身逻辑（Prompt、解析、校验）不进入入口层；**Agent 数量与能力变化不应迫使入口层大改**。

对外因此只暴露一层最小契约，调用方看不到 `TypedRunner`、`AgenticMessage`、JSON 解析与 `Validate`：

```go
marketAgent, err := market.New(ctx, cfg)

marketCtx, err := marketAgent.Analyze(ctx, "分析当前 A 股市场环境。")
```

### 1.4 每个 Agent 在自己的包内完成组装与测试

一个 Agent 的完整生命周期都在自己的包内闭合：

```text
配置 → Prompt → Middleware → Agent 构建 → Runner → Parse → Validate → 测试
```

共享能力**不提前抽象**：只有真实的重复出现时才抽取，`internal/runtime` 的抽取时机就是集成测试里反复出现同一份 bootstrap 之后。Runtime 提供 Context / Skill / Filesystem / Execute，Agent 包只接收注入进来的依赖。

---

## 2. Market Agent 的包结构

```text
internal/agent/market/
├── agent.go
├── context.go
├── prompt.go
├── validator.go
├── agent_test.go
└── integration_test.go
```

| 文件 | 职责 |
|---|---|
| `agent.go` | `Config` / `Agent` 定义、`New()` 组装（ChatModelAgent + TypedRunner）、`Analyze()` 运行与输出收集、`parseContext()`、`assistantText()` |
| `context.go` | 输出契约：`Regime`、`IndexSnapshot`、`SectorPerformance`、`Context` |
| `prompt.go` | 职责边界、关注项目、市场状态定义、时间规则、JSON 输出格式（`instruction` 常量） |
| `validator.go` | `Validate(*Context) error`：只做结构合法性校验 |
| `agent_test.go` | 确定性单测：`fakeModel` 注入、正常解析、非法 JSON、模型错误、空 query、`Validate` 表驱动用例 |
| `integration_test.go` | 真实链路集成测试：真实 Model + Runtime + Skill + Tushare → 真实 `Context`；当前测试主体被整体注释（内部引用了尚未实现的 `runtime.New` / `rt.AgentHandlers()` 目标接口），保留 `requireEnv` / `envOrDefault` / `findSkillsDir` 三个 helper |

两条结构约定：

- **输出契约与 Agent 同包**。`Context` 就是 Market Agent 的输出契约，放在 `internal/agent/market` 内，而不是单独建 `internal/market`；包名已表达 `market`，类型直接叫 `Context`，外部写作 `*market.Context`。
- **Prompt 不重复 Harness 职责**。Skill-first、依赖自动修复等属于 Skill Runtime，写在 Runtime 的 system prompt 里，不写进 Market Agent 的 Prompt。

---

## 3. MarketContext 契约

`internal/agent/market/context.go` 全文：

```go
package market

type Regime string

const (
	RegimeRiskOn  Regime = "RISK_ON"
	RegimeMixed   Regime = "MIXED"
	RegimeRiskOff Regime = "RISK_OFF"
)

type IndexSnapshot struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type SectorPerformance struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type Context struct {
	AsOf string `json:"as_of"`

	Regime Regime `json:"regime"`

	Indexes []IndexSnapshot `json:"indexes"`

	AdvanceCount   int `json:"advance_count"`
	DeclineCount   int `json:"decline_count"`
	LimitUpCount   int `json:"limit_up_count"`
	LimitDownCount int `json:"limit_down_count"`

	Turnover float64 `json:"turnover"`

	LeadingSectors []SectorPerformance `json:"leading_sectors"`

	Evidence []string `json:"evidence"`
	Risk     []string `json:"risk"`

	Confidence float64 `json:"confidence"`
}
```

| 字段 | 类型 | JSON tag | 说明 |
|---|---|---|---|
| `AsOf` | `string` | `as_of` | 本次判断**实际采用的市场数据日期**，格式 `YYYYMMDD`（如 `"20260922"`），不是程序运行时间 |
| `Regime` | `Regime` | `regime` | 市场状态，取值 `RISK_ON` / `MIXED` / `RISK_OFF` |
| `Indexes` | `[]IndexSnapshot` | `indexes` | 主要指数快照，元素为 `name` + `change_percent` |
| `AdvanceCount` | `int` | `advance_count` | 上涨家数 |
| `DeclineCount` | `int` | `decline_count` | 下跌家数 |
| `LimitUpCount` | `int` | `limit_up_count` | 涨停家数 |
| `LimitDownCount` | `int` | `limit_down_count` | 跌停家数 |
| `Turnover` | `float64` | `turnover` | 全市场成交额，口径为 CNY 数值（集成输出为 `2153700000000` 量级），契约本身不带单位字段 |
| `LeadingSectors` | `[]SectorPerformance` | `leading_sectors` | 领涨行业 / 板块，元素为 `name` + `change_percent` |
| `Evidence` | `[]string` | `evidence` | 支持判断的主要证据 |
| `Risk` | `[]string` | `risk` | 当前主要风险；数据缺失、口径盲区、数据时间冲突都写在这里 |
| `Confidence` | `float64` | `confidence` | 判断置信度，取值 `[0,1]` |

`Regime` 只有三个合法取值，语义由 Prompt 约束、由 `Validate` 兜底：

```text
RISK_ON    市场整体风险偏好较高，多项市场证据整体偏强
MIXED      市场信号明显分化，指数、市场广度、成交、板块或情绪之间存在明显冲突
RISK_OFF   市场整体风险偏好较低，多项市场证据整体偏弱
```

---

## 4. Validator 规则

`internal/agent/market/validator.go` 全文：

```go
package market

import (
	"errors"
	"fmt"
)

func Validate(v *Context) error {
	if v == nil {
		return errors.New("market context is nil")
	}

	if v.AsOf == "" {
		return errors.New("as_of is required")
	}

	switch v.Regime {
	case RegimeRiskOn, RegimeMixed, RegimeRiskOff:
	default:
		return fmt.Errorf("invalid regime: %s", v.Regime)
	}

	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"confidence out of range: %f",
			v.Confidence,
		)
	}

	if len(v.Evidence) == 0 {
		return errors.New("evidence is empty")
	}

	return nil
}
```

| 校验项 | 判据 |
|---|---|
| Context 非 nil | `v == nil` → 报错 |
| `as_of` 非空 | `v.AsOf == ""` → 报错 |
| `regime` 是枚举值 | 不在 `RegimeRiskOn` / `RegimeMixed` / `RegimeRiskOff` 中 → 报错 |
| `confidence` 在区间内 | `< 0` 或 `> 1` → 报错 |
| `evidence` 非空 | `len(v.Evidence) == 0` → 报错 |

**明确不校验** `Turnover > 0`、`AdvanceCount > 0`、`len(Indexes) > 0`，因为两类错误的性质不同：

- **结构非法**（缺 `as_of`、`regime` 不是枚举、`confidence` 越界、没有 `evidence`）是 bug，必须让调用方立刻失败。
- **数据缺失**（拿不到成交额、拿不到指数列表）是业务事实：真实数据取不到时，Agent 应把缺口写进 `risk`，而不是为了通过 Validator 伪造数据。

同理，`Validate` **不承担时间语义判断**：不检查 `as_of` 距今天数、不检查它是否等于当前交易日、不检查它是否等于 `Now`。`as_of` 与任务目标时间是否一致属于 Agent 的语义职责；写成 `Now - AsOf <= N days` 等于替 Agent 做语义判断，会把合法的历史研究误判为错误。

---

## 5. Market Agent 的 Prompt

`internal/agent/market/prompt.go` 中 `instruction` 常量的正文（Market Agent 运行时的 system instruction）：

```text
你是 Aegis 的 Market Agent。

你的职责是研究指定时间点的整体市场环境，并输出结构化的市场上下文。

你只负责市场层面的研究，不负责：
- 分析单只股票
- 给出个股交易建议
- 输出 CANDIDATE / WATCH / AVOID
- 选择具体买卖点

根据任务需要，自主使用当前可用的 Skill 和工具获取真实数据。

分析市场环境时重点关注：

1. 主要指数表现
2. 上涨与下跌家数
3. 涨停与跌停数量
4. 市场成交额和活跃度
5. 行业、板块和主题强弱
6. 市场赚钱效应
7. 整体风险偏好
8. 不同数据之间是否存在冲突

不要根据单一指标判断市场环境。

市场状态只能是：

RISK_ON
市场整体风险偏好较高，多项市场证据整体偏强。

MIXED
市场信号明显分化，指数、市场广度、成交、板块或情绪之间存在明显冲突。

RISK_OFF
市场整体风险偏好较低，多项市场证据整体偏弱。

时间规则：

1. 首先确定本次研究对应的目标时间。
2. 所有具有时间属性的数据应尽量使用一致的时间基准。
3. 如果当前交易日数据尚未完整，应使用最近一个数据完整的交易日。
4. as_of 必须填写实际用于判断的数据日期。
5. 不得把明显过期的数据当成当前市场状态。
6. 如果关键数据无法获得，应在 risk 中明确说明，不得编造。

最终只输出一个 JSON object。

JSON 格式：

{
  "as_of": "YYYYMMDD",
  "regime": "RISK_ON | MIXED | RISK_OFF",
  "indexes": [
    {
      "name": "string",
      "change_percent": 0.0
    }
  ],
  "advance_count": 0,
  "decline_count": 0,
  "limit_up_count": 0,
  "limit_down_count": 0,
  "turnover": 0,
  "leading_sectors": [
    {
      "name": "string",
      "change_percent": 0.0
    }
  ],
  "evidence": ["string"],
  "risk": ["string"],
  "confidence": 0.0
}

要求：

- regime 只能是 RISK_ON、MIXED、RISK_OFF
- evidence 必须是 string[]
- risk 必须是 string[]
- confidence 必须在 0 到 1 之间
- 不要输出 Markdown
- 不要输出 JSON 之外的任何文字
```

设计要点：

- **职责边界**：只做市场层面研究；不做个股分析、不给个股交易建议、不输出 `CANDIDATE` / `WATCH` / `AVOID`、不选买卖点。
- **不靠单一指标下结论**：八项关注清单覆盖指数、广度、涨跌停、成交与活跃度、板块主题、赚钱效应、风险偏好与数据冲突。
- **三种市场状态**在 Prompt 内给出可判定定义，与 `Regime` 枚举一一对应。
- **时间规则**要求先由语义确定目标时间、时间基准一致、数据不完整时退到最近完整交易日、`as_of` 填实际采用的数据日期、不得用过期数据冒充当期状态、数据缺失写入 `risk`。
- **Prompt 不写具体 Skill / Tool 名称**：Skill 选择属 Harness Runtime，Agent 只被要求「自主使用当前可用的 Skill 和工具」。

`internal/agent/market_prompt.go` 是早期扁平 `agent` 包中的同义 Prompt 副本（`marketInstruction`），措辞略有差异；当前 Market Agent 的组装路径使用 `internal/agent/market/prompt.go`。

---

## 6. Runner 的选型

`internal/agent/market/agent.go` 使用 Eino `TypedRunner` 并显式关闭 streaming：

```go
runner := adk.NewTypedRunner(
	adk.TypedRunnerConfig[*schema.AgenticMessage]{
		Agent: a,

		// Market Agent 最终输出是结构化 JSON，
		// 当前没有必要使用 streaming。
		EnableStreaming: false,
	},
)
```

选型理由：

1. **最终结果是结构化 JSON，不是逐字输出。** 产物是一份 `MarketContext`，只有在 Run 结束后整体反序列化才有意义，流式增量对契约没有任何价值。
2. **开 streaming 会让 JSON 聚合复杂。** Eino 打开 streaming 后，`MessageOutput` 可能给出 `MessageStream` 而不是完整 `Message`，调用方要自己做增量拼接与边界处理，收益为零、复杂度上升。
3. **可观测性不受影响。** 关闭 streaming 只影响输出形态，Callback 日志照常保留。

`Analyze()` 的事件循环遵循两条规则：只取 **assistant 文本**，且只保留**最后一个非空 assistant 输出**。

```go
	var output string

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if event == nil {
			continue
		}

		if event.Err != nil {
			return nil, fmt.Errorf(
				"market agent run failed: %w",
				event.Err,
			)
		}

		if event.Output == nil ||
			event.Output.MessageOutput == nil ||
			event.Output.MessageOutput.Message == nil {
			continue
		}

		text := strings.TrimSpace(
			assistantText(event.Output.MessageOutput.Message),
		)

		if text != "" {
			// 只保存最后一个有效 assistant 输出。
			// Agent Loop 中间可能发生多轮 Tool Calling，
			// 最后一个 assistant message 才是需要解析的 MarketContext JSON。
			output = text
		}
	}
```

文本提取只认 `AssistantGenText` 块，忽略 Tool Calling 等其它块：

```go
func assistantText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}

	var b strings.Builder

	for _, block := range msg.ContentBlocks {
		if block == nil || block.AssistantGenText == nil {
			continue
		}

		b.WriteString(block.AssistantGenText.Text)
	}

	return b.String()
}
```

若不做「只取最后一个」，Agent Loop 中间步骤产生的文本会与最终 JSON 混在一起，导致 `json.Unmarshal` 失败。收集到输出后，`parseContext()` 先 `json.Unmarshal` 到 `Context`，再调用 `Validate()`，两步任一失败都返回错误。

---

## 7. Trading Agent 的输出契约（V1）

Trading Agent 的产物是结构化 `Decision`，不是一篇分析文章。契约定义在 `internal/decision/decision.go`：

```go
package decision

import "fmt"

type Action string

const (
	ActionWatch     Action = "WATCH"
	ActionAvoid     Action = "AVOID"
	ActionCandidate Action = "CANDIDATE"
)

type Decision struct {
	Symbol     string   `json:"symbol"`
	MarketView string   `json:"market_view"`
	Evidence   []string `json:"evidence"`
	Risk       []string `json:"risk"`
	Action     Action   `json:"action"`
	Confidence float64  `json:"confidence"`
}

func Validate(d *Decision) error {
	if d.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}

	if d.MarketView == "" {
		return fmt.Errorf("market view is required")
	}

	switch d.Action {
	case ActionWatch, ActionAvoid, ActionCandidate:
	default:
		return fmt.Errorf("invalid action: %s", d.Action)
	}

	if d.Confidence < 0 || d.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	if len(d.Evidence) == 0 {
		return fmt.Errorf("evidence is required")
	}

	return nil
}
```

| 约束 | 内容 |
|---|---|
| `action` 取值 | 只能是 `WATCH`（继续观察）/ `AVOID`（当前不考虑）/ `CANDIDATE`（进入短线候选池） |
| 禁止动作 | **禁止输出 `BUY` / `SELL`**；不因要求买卖就直接给出交易执行建议 |
| `confidence` | `[0,1]` 之间的小数 |
| `evidence` / `risk` | 必须是 JSON 字符串数组；**即使只有一项也必须使用数组** |
| 输出形态 | 合法 JSON，不得用 Markdown 代码块包裹，不得在 JSON 前后添加文字 |

V1 不把 `Action` 设计成 Buy / Sell，是为了先验证 Agent 的分析链路，避免一开始就把交易决策与 Agent 架构绑死；`Decision` 是后续 Strategy、Risk、Backtest、Memory、Portfolio 统一消费的结构化对象，系统最终产出不应是一段自由文本。

V1 Instruction 中与输出契约相关的要求片段：

```text
你是 Aegis，一个交易研究 Agent。

你的任务是进行交易研究，而不是执行交易。

分析个股短线状态时，必须结合整体市场环境。

整体市场环境包括指数表现、涨跌家数、涨跌停数量、
市场成交额和板块表现。

这些数据是客观事实，不是交易结论。
你需要自行判断整体市场环境如何影响当前个股的短线机会和风险。

不要简单复述市场数据，要解释市场环境与个股状态之间的关系。

当用户要求分析某个标的时：

1. 判断当前问题需要哪些信息。
2. 必要时调用可用工具获取信息。
3. 根据工具返回的数据进行综合分析。
4. 如果信息不足，继续调用必要的工具，不要猜测。
5. 如果不同工具返回的数据存在冲突，必须明确指出冲突，并降低判断置信度。
6. 最终必须输出一个结构化 Decision。

Decision 必须包含：

- symbol：股票代码，string
- market_view：当前市场/短线状态判断，string
- evidence：支持判断的主要证据，string[]
- risk：当前主要风险，string[]
- action：当前建议的状态，string
- confidence：对当前判断的置信度，number

action 只能使用：

- WATCH：继续观察
- AVOID：当前不考虑
- CANDIDATE：进入短线候选池

confidence 必须是 0 到 1 之间的小数。

evidence 和 risk 必须始终输出为 JSON 字符串数组。
即使只有一条 evidence 或 risk，也必须使用数组，不能输出为单个字符串。

正确示例：

{
  "symbol": "600519",
  "market_view": "当前短线状态判断",
  "evidence": [
    "证据1",
    "证据2"
  ],
  "risk": [
    "风险1",
    "风险2"
  ],
  "action": "WATCH",
  "confidence": 0.6
}

不要输出 BUY 或 SELL。
不要因为用户要求买入或卖出，就直接给出交易执行建议。

最终输出必须是合法 JSON，不要使用 Markdown 代码块，不要在 JSON 前后添加其他文字。
```

**实现状态**：`Decision` 契约与 `decision.Validate` 已实现；`internal/agent/trading/agent.go` 目前只有一行 `package trading`，Trading Agent 的组装（Prompt 常量、`New()`、Runner、JSON 解析）**设计已定、实现未落地**，该契约当前只有类型层面的约束，尚未接到运行时链路上。

---

## 8. 组装代码与依赖注入

### 8.1 Agent 侧：`Config` / `New` / `Analyze`

```go
type Config struct {
	Model model.BaseModel[*schema.AgenticMessage]

	Handlers []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]

	MaxIterations int
}

type Agent struct {
	runner *adk.TypedRunner[*schema.AgenticMessage]
}

func New(ctx context.Context, cfg Config) (*Agent, error) {
	if cfg.Model == nil {
		return nil, errors.New("market agent model is required")
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 20
	}

	a, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "market_agent",
			Description: "Researches the overall market environment and produces structured market context.",
			Instruction: instruction,
			Model:       cfg.Model,

			Handlers: cfg.Handlers,

			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create market agent: %w", err)
	}

	runner := adk.NewTypedRunner(
		adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent:           a,
			EnableStreaming: false,
		},
	)

	return &Agent{runner: runner}, nil
}

func (a *Agent) Analyze(
	ctx context.Context,
	query string,
	opts ...adk.AgentRunOption,
) (*Context, error) {
	// 校验 Agent 已初始化、query 非空
	// → runner.Query(ctx, query, opts...)
	// → 逐事件收集最后一个非空 assistant 文本
	// → parseContext(output)（json.Unmarshal + Validate）
}
```

要点：

- `Config` 只接收**依赖**：`Model`、`Handlers`、`MaxIterations`。Market 包不创建 Model、不初始化 Skill / Filesystem、不知道 skills 根目录、不知道 execute 用什么 shell。
- `MaxIterations <= 0` 时取默认值 **20**；集成测试显式传入 `30`。
- Agent 名固定为 `market_agent`，与包名、Prompt 职责一致。
- `Analyze(ctx, query string, opts ...adk.AgentRunOption) (*Context, error)` 是稳定的业务契约，运行选项（如 Callback）通过 `opts` 传入，不需要改签名。

### 8.2 调用侧：Model 与 Handlers 的注入

```go
marketAgent, err := market.New(
	ctx,
	market.Config{
		Model: dp,

		Handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
			skillMW,
			fsMW,
		},

		MaxIterations: 30,
	},
)
if err != nil {
	return err
}

marketCtx, err := marketAgent.Analyze(
	ctx,
	"分析当前 A 股市场环境。",
)
if err != nil {
	return err
}
```

两条注入路径的归属：

```text
Model     ← internal/llm（按 model profile 创建，Provider 差异收敛在 adapter 内）
          ← 面对 Agent 时统一为 model.BaseModel[*schema.AgenticMessage]

Handlers  ← shared Runtime（Runtime Context / Skill / Filesystem / Execute）
          ← Agent 只声明需要哪些 Harness capability
```

即 **Agent 是 provider-neutral 的**：Market 包不感知 DeepSeek / OpenAI / Claude 的差异，也不感知 Skill Runtime 的实现方式，只接收「一个 Model」和「一组 Middleware」。

### 8.3 目标形态与当前实现状态

组装链路的最终形态：

```text
App Config
    ↓
Runtime（Context / Skill / Filesystem / Execute）
    ↓
Handlers
    ↓
market.New(ctx, Config{Model, Handlers, MaxIterations})
    ↓
market.Agent.Analyze(ctx, query)
    ↓
*market.Context
```

需要区分「已实现」与「未实现」：

| 项 | 状态 |
|---|---|
| `market.New` / `market.Analyze` / `parseContext` / `Validate` | 已实现，单测覆盖 |
| `market/integration_test.go` 真实链路 | 已跑通过（默认模型 + Skill + Tushare）；测试主体当前被注释，保留 helper |
| `internal/config` 的 `ModelProfileForAgent` + `internal/llm` 的 Manager / DeepSeek | 已实现（`ModelConfig.APIKey` 存放环境变量名） |
| `internal/runtime` 的 `New(cfg{SkillsDir, TimeZone})` + `Handlers()` | **未实现**：`runtime.go` 只声明 `Config` / `Runtime` 类型，无构造函数与 `Handlers()`；`context.go` / `skill.go` / `filesystem.go` 仍只有 `package` 声明；`time_context.go` 已实现 `ContextMiddleware`，在 `BeforeAgent` 注入 `current_time` + `timezone` |

因此文中出现的 `rt.Handlers()` 属于**目标接口形态**；在 Runtime 落地前，集成测试仍以内联 bootstrap 组装 Skill / Filesystem middleware。

---

[← 返回文档索引](README.md)
