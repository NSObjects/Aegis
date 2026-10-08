# Event Hosted E2E：语义匹配与结构化输出的稳定性边界

> 2026-10-08；PR #81。本文是实现记录，不取代 `docs/10-多Agent架构与职责边界.md`。

## 1. 观察到的故障

- Integration #37712842612：旧 Stateless Trading 阶段的 JSON 含多余引号。
- Integration #37714418364：Event 事实提取 JSON 格式错误，以及 Event Materiality 多出响应字段。
- Integration #37715426256：Risk/Portfolio E2E 的 Semantic Evidence Matcher 少返回一个候选 ID（`cls_news_2498111`）。
- 以上是不同真实模型输出失约案例，不能因为普通 `go test ./...` 成功就宣称 Hosted E2E 通过。

这些运行均依赖不断变化的实时新闻、远端模型与外部数据。构造出的 JSON 是需要验证的输入，不是可信业务事实。

## 2. 官方文档与接口事实

- DeepSeek JSON Output：https://api-docs.deepseek.com/guides/json_mode/
  - 设置 `response_format={"type":"json_object"}`，Prompt 中包含 JSON 和输出示例；
  - 需合理设置 max_tokens，并考虑空输出、截断；
  - **JSON 语法正确不等于字段齐全、证据正确或结果可信**。
- DeepSeek Chat Completions：https://api-docs.deepseek.com/api/create-chat-completion/
  - `finish_reason=length` 可能表示生成截断。
- Eino AgenticDeepSeek：https://github.com/cloudwego/eino-ext/blob/main/components/model/agenticdeepseek/README.md
  - `Config.ResponseFormatType`、`Config.MaxTokens` 已有支持；
  - `ResponseMetaExtension.FinishReason` 提供结束原因；
  - 模型接口接受完整的 `[]*schema.AgenticMessage`，应用侧仍必须执行严格契约校验。

## 3. Semantic Evidence Matcher 运行契约

`HybridEvidenceSelector` 先使用股票代码/名称等确定性直接关联信息判定；
不满足确定性证据关联的候选交给 `AgenticSemanticEvidenceMatcher`。

为了降低一次输出需覆盖的候选数量：

1. 按原始候选顺序，每批最多 4 个原始文档。
2. 每个模型响应通过 `validateSemanticMatches` 检查 ID、重复、置信度。
3. 如果缺失一些候选，只对**缺失候选**补判一次；不重复已成功匹配的新闻。
4. 若补判后仍遗漏，`HybridEvidenceSelector` 为遗漏项生成 `UNREVIEWED` 记录、`selected=false`、`confidence=0`。
5. `applyEvidenceSelection` 再次拒绝任何将 `UNREVIEWED` 伪装成选中的行为。
6. 虚构/未知 ID、重复 ID、非法置信度仍报错，不接受错误匹配结果。

`UNREVIEWED` **不代表已经确认该新闻无关**，而是**未得到可用的相关性判断**。它必须留在审计结果中，不得作为正向证据、事实或经济影响进入 EventContext。

## 3.1 生成长度配置

模型配置 `ModelConfig.MaxTokens` 对应可选 YAML `models.<profile>.max_tokens`，直接传给 Eino `agenticdeepseek.Config.MaxTokens`。不填写时保留供应商默认值，非正数拒绝创建模型。

Hosted Full Stateless / Full Multi-Agent E2E 使用显式 `max_tokens=16384`，因为这些测试有较长的事件提取输出；此设置是上限而不是要求模型一定输出 16384 tokens，不代表 JSON 或领域契约可以省略校验。

批处理 / 补判针对的是**漏 ID**问题；增加输出预算针对的是**生成过长或可能截断**问题。这两种错误要分别记录和测试。

## 4. 其他结构化输出

- Event Analyst 的 `generateStrictAgenticJSON` 允许最多两次完整生成，但每次都使用独立解码目标，保留未知字段和多 JSON 值的严格拒绝。
- Trading Agent 仅对 JSON 语法失败做有界重新生成，不自动修补字段。
- 语义相关性、原文 grounding、PIT 时间、事实关系和 Risk/Portfolio 契约仍需各自的确定性 validator。
- 重试耗尽不等于模型验证通过，不允许生成假结果。

## 5. 正确的验收分层

- 单元测试：固定输入 + 脚本化模型输出，覆盖批次边界、漏项补判、持续遗漏、伪造 ID 和拒绝未审查证据；`go test ./...` 只证明这些确定性测试成功。
- Hosted 测试：明确记录当前 branch SHA、GitHub Actions run ID、失败阶段和数据有效时间；一次失败应当先查日志，不应推测与上次同因。
- Risk/Portfolio E2E：使用**显式测试用 Portfolio fixture**，绝不推导真实账户数据；最终 `PortfolioDecision` 仍不批准可执行交易。
- 当前 workflow_dispatch 默认仅运行 `run_risk_portfolio`，如需单独诊断 Stateless/Multi-Agent/Technical/Trading，再显式开启对应选项。

**验收结论必须同时区分：代码可编译、确定性测试通过、真实 Hosted E2E 通过。三者不能互相替代。**

## 6. 后续但不在本次 PR 内

若仍频繁受新闻实时变化影响，应把 live acquisition smoke 与基于持久化 canonical evidence snapshot 的 agent-chain regression 分开，并在测试输出中记录 snapshot ID、版本、PIT as_of 和覆盖率。这能区分数据源变化、模型不稳定、领域 validator 拒绝，防止纯随机重跑掩盖问题。
