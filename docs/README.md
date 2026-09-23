# Aegis 文档

Aegis 是一个用 Go + [CloudWeGo Eino](https://github.com/cloudwego/eino) 构建的 **A 股交易研究 Agent**。

本目录是从项目早期设计过程中沉淀下来的技术文档，说明 Aegis **怎么设计、为什么这样设计**，以及**当前做到哪一步**。

> 代码是现状的唯一事实来源；文档解释决策与理由。两者冲突时以代码为准。

---

## 文档地图

| 文档 | 内容 |
|---|---|
| [01-设计原则.md](01-设计原则.md) | 项目定位、Eino 的分层位置、Graph 与 ADK 的分工、目录演进策略、增量实现纪律 |
| [02-数据能力层.md](02-数据能力层.md) | Capability / Binding 模型、多实现降级、数据契约与失败判定、MCP 运行时 |
| [03-Skill与工具运行时.md](03-Skill与工具运行时.md) | Skill 标准形态、执行能力与安全边界、`internal/runtime` 的职责与接口 |
| [04-Agent设计与组装.md](04-Agent设计与组装.md) | Agent 组装原则、Market Agent 契约与 Prompt、Runner 选型、Decision 输出 |
| [05-模型配置与供应商.md](05-模型配置与供应商.md) | `default_model` + per-agent override、Provider 中立、LLM Manager |
| [06-时间语义与输出契约.md](06-时间语义与输出契约.md) | `as_of` 语义时间规则、时间注入、结构化 JSON 输出要求 |
| [07-工程实践与踩坑.md](07-工程实践与踩坑.md) | Eino API 踩坑、Callback 可观测性、JSON 输出调试、测试策略 |
| [08-演进路线与现状.md](08-演进路线与现状.md) | 演进路线（V1→V10 版本主线、V2.x 里程碑）、长期目标、当前实现状态快照、待办 |

**新手建议阅读顺序**：01 → 02 → 03 → 04，再按需查 05–08。

## 研究

对 Aegis 要对标的**外部框架**的源码级研究，与上面的 Aegis 自身设计文档分开存放。

| 研究 | 版本 | 内容 |
|---|---|---|
| [TradingAgents](research/tradingagents/README.md) | v0.5.0（`2d17df8`） | 多 Agent 协作、数据时效性、结果结算与反思、多供应商接入；含「吸收 / 改造 / 不搬」清单与差距分析 |

---

## 核心决策速览

| # | 决策 | 一句话理由 |
|---|---|---|
| 1 | Aegis 做**交易研究**，不做交易执行 | 先验证分析链路，不把"交易决策"和"Agent 架构"耦死 |
| 2 | Eino 是 **Agent 运行基础设施**，不是业务架构 | Agent 是业务角色，Graph / ADK 是执行机制，不能反过来设计 |
| 3 | 业务流程**不全 Graph 化** | 流程确定处用 Graph，自主判断处用 ADK；全 Graph 太死 |
| 4 | 不包 `internal/eino/` | Eino 是基础设施，代码应表达业务概念 |
| 5 | 目录**随能力生长**，不提前铺空目录 | 目录结构本身就是项目演进的记录 |
| 6 | 每个 Agent **在自己包内**组装与测试 | Agent 数量与能力变化不应迫使入口层大改 |
| 7 | 外部数据用 **Capability / Binding**，一个能力绑多个实现 | 数据源可能是 MCP / Skill / DB，且单一来源会失效 |
| 8 | 降级判定**不只看报错** | HTTP 200 但空数据、旧数据、字段缺失同样是失败 |
| 9 | `Validate` 只校验**结构合法性**，不校验数据完整性 | 数据缺失写入 `risk`，不能逼 Agent 编数据 |
| 10 | Skill 能力集固定为 `skill` / `read_file` / `execute` | 固定目录 + 固定能力集，不做 coding-agent 级防护 |
| 11 | **Runtime 不持有 Model** | 模型属于每个 Agent 的组装配置，才能 per-agent 选供应商 |
| 12 | 模型配置 = `default_model` + per-agent override | 不必为每个 Agent 都配模型 |
| 13 | `as_of` 取**实际用于判断的数据日期** | 测试适配语义，不是语义适配测试 |
| 14 | 长期目标是**可自成长的 Harness** | Agent 只是当前版本的 Composition，Prompt/Skill/契约需可版本化 |

---

## 工程纪律

本项目的实现节奏与架构决策同等重要，核心是 **先跑通、后收紧**：

1. **不提前抽象** —— 抽象必须由一次真实的重复触发（如 `internal/runtime` 是在集成测试出现重复 bootstrap 后才抽的）
2. **一次只动一小块** —— 改动前后都要验证"原本能跑，改完还能跑"
3. **不把目标架构当成当前代码** —— 测试只能引用已经存在的 API
4. **测试去适配语义** —— 不允许为了测试通过而写死业务语义（如时间）
5. **先跑通、后收紧** —— 新能力先用最小方案接通（如 Skill 先只接 Middleware + Execute，不做安装/创建），跑通后再加固

---

## 当前状态

截至 **2026-09-23**：

| 层 | 状态 |
|---|---|
| `cmd/aegis/main.go` | **V1 单体原型**：能跑通 Agent + Skill + Tool + Callback，但**未接入** `internal/config` / `internal/llm` / `internal/runtime` / `internal/agent/market`；密钥与模型名**硬编码**在源码里 |
| `internal/config`、`internal/llm` | 已实现；Provider 枚举列了 6 家，但**只有 DeepSeek 真正落地** |
| `internal/runtime/mcp`、`time_context.go` | 已实现 |
| `internal/runtime/{runtime,context,skill,filesystem}.go` | **未完成** —— `runtime.go` 只声明了 `Config` / `Runtime` 类型，无 `New()` / `Handlers()`；其余三个仅有 `package` 声明 |
| `internal/skillruntime/runtime.go` | **当前真正在用的 Skill 运行时**（`main.go` 调用），Skill 根目录硬编码 `./skills` |
| `internal/agent/market` | 已实现（含 Prompt / Validator / 单测）；但 `integration_test.go` 的**测试体已被整段注释**，真实链路处于「已停用」 |
| `internal/agent/trading/agent.go` | 空壳 |

构建状态：`go build ./...` 与 `go vet ./...` 通过。

### ⚠️ 已知凭据风险

| 位置 | 内容 | 状态 |
|---|---|---|
| `cmd/aegis/main.go:119` | 硬编码 DeepSeek API Key | **已进入 git 历史**（`b1d9e6e Initial commit`），需吊销并清理历史 |
| `configs/mcp.json` | Tushare token 明文写在 URL 查询串里 | 当前未被跟踪，但 `.gitignore` 第 4 行 `./configs` 是无效模式，实际**未被忽略**，有被误提交的风险 |

正确做法是把凭据放在环境变量里、由 `internal/config` + `internal/llm` 读取（这套代码已就绪）。

完整状态表、推进顺序与待办见 [08-演进路线与现状.md](08-演进路线与现状.md)。
