# Research Evidence Ledger — 最小确定性证据追溯清单

> 增量分支：`codex/research-evidence-ledger`，最初从 V2.11.3 的 `main` 创建。PR #86（V2.11.4 组合情景推演）与 PR #87（当前能力文档更新）现已合并至 `main`；本分支尚未对齐最新 `main`，必须在合并前重新验证兼容性。
>
> **Aegis 永久是研究与决策辅助系统，不会增加券商下单、执行或审批能力。**

## 目的与真实输入契约

在现有 `PortfolioReview` 通过 `ResearchArtifact → DebateState → ResearchPlan` 验证后，输出可审计的 `evidence_ledger`。使用已经存在的结构化对象，不发起新数据检索、不调用新 LLM、不修改任何研究原文或原有决策。

信息链与边界：

- `ResearchArtifact`：Market / Technical / Event Context 均经过 `synthesis.Validate`。`synthesis.Reference` 只能是 `MARKET`、`TECHNICAL` 或 `EVENT`。
- `DebateState.Turns[].Arguments[]`：每条记录有全局唯一 `id`、Bull/Bear、轮次、论点文本、引用列表；`Rebuttals` 仅可指向**之前的论点 ID**。重用 `debate.ValidateState` 的规则。
- `ResearchPlan`：`accepted_argument_ids`、`unresolved_argument_ids`、`opportunities/risks[].references`。重用 `manager.Validate`：ID 必须存在，重复和跨分组重复必须拒绝，引用的事件 ID 必须存在于 `ResearchArtifact.Event.Events`。
- `TradeIntent.Evidence` 是**自由文本**，没有结构化 ID/引用绑定；本 Ledger **完全不读取该字段**，更不会解析字符串作为证据。

**不新增领域输入 Schema。** 唯一增加的正式输出是 `PortfolioReviewResult.evidence_ledger`；其生成与检验位于 Research Manager 校验之后、调用 Trading Agent 之前，执行失败即 fail-closed。

## Ledger 数据契约

`evidence_ledger` 包含：

- `symbol`、`conclusion`：直接来自经验证的研究对象；
- `arguments[]`：按照 Debate Turn → Argument 原始顺序，保留 `id`、`round`、`side`、`claim`、`references`；
- `arguments[].disposition`：
  - `ACCEPTED`：ID 出现在 `accepted_argument_ids`；
  - `UNRESOLVED`：ID 出现在 `unresolved_argument_ids`；
  - `UNCLASSIFIED`：存在于 Debate，但未被 Manager 放进上述两组。**不是 REJECTED，也不是无证据。**
- `arguments[].challenges[]`：按原始辩论顺序列出后续反驳，关联到真实目标 ID，记录反驳人 `side`、`round`、`claim`、`references`。没有制造反驳 ID。
- `insights[]`：按 ResearchPlan Opportunities 顺序、再按 Risks 顺序，展示独立的 `OPPORTUNITY / RISK`、`claim` 和原始引用。`argument_ids` 为可选字段：只复制 Research Manager **显式提供、已通过 Debate ID 校验**的论点关联；旧版未提供时不输出该字段，更不凭文本相似性猜测关联。关联 ID 只能证明所引用论点存在，不能独立证明 insight 语义正确。

### 结构示例（说明形状，非实际市场研究）

```json
{
  "symbol": "600519.SH",
  "conclusion": "BALANCED",
  "arguments": [
    {
      "id": "bull-1", "side": "BULL", "round": 1,
      "claim": "技术面相对改善",
      "references": [{"domain": "TECHNICAL"}],
      "disposition": "ACCEPTED",
      "challenges": [
        {
          "side": "BEAR", "round": 1,
          "claim": "市场整体风险仍需考虑",
          "references": [{"domain": "MARKET"}]
        }
      ]
    },
    {
      "id": "bear-1", "side": "BEAR", "round": 1,
      "claim": "市场偏谨慎",
      "references": [{"domain": "MARKET"}],
      "disposition": "UNRESOLVED"
    }
  ],
  "insights": [
    {
      "kind": "RISK", "claim": "市场方向尚未确认",
      "references": [{"domain": "MARKET"}]
    }
  ]
}
```

这是说明字段形状的**虚构示例**，不能当成真实研究、实际证券现状或 CI 运行结论。

## 验证与失败规则

| 输入情况 | 结果 |
|---|---|
| 合法且有采纳/未解决 ID | 原样追溯具体 Bull/Bear 论点 |
| 合法但未被 Manager 提及的论点 | `UNCLASSIFIED`，不推断负面结论 |
| 未知 `accepted` / `unresolved` ID | 拒绝，`does not exist in debate` |
| 组内重复 ID、跨组重复 ID | 拒绝；不默默去重或任选一种状态 |
| Debate 不同轮重复 Argument ID | 拒绝；不能覆盖已有论点 |
| 不存在或不是前序论点的反驳目标 ID | 拒绝 |
| 引用不存在的 Event ID | 拒绝；不能编造事件 |
| `MARKET/TECHNICAL` 携带 Event ID | 拒绝；领域引用类型必须匹配 |
| 研究标的在 Artifact / Debate / Plan 不一致 | 拒绝 |
| 通过所有校验、但没有任何辩论轮次 | 拒绝；不输出空 Ledger 冒充研究 |

与现有 Validator 完全一致：Argument ID 在校验时按 `TrimSpace` 比较，Ledger 展示归一化 ID，但不修改原始状态。

`EVENT` 引用允许不填 `event_id`，表示**仅引用 Event 领域上下文**；若填了 `event_id`，必须存在于 EventContext 中。没有具体 ID 时不声称已溯源至某条事件、Document 或 Fact。Market / Technical 的引用也只有**领域粒度**，并不是单条数值、新闻原文或源 URL 的索引。

## 稳定性与边界

- 按已有有序 Slice 遍历；内部 Map 仅用于 ID 查找、从不决定输出顺序。
- Build 不改变 `ResearchArtifact`、`DebateState` 或 `ResearchPlan`；复制引用 Slice，避免下游修改污染原始数据。
- 同输入可重复得到完全相同的 JSON 字节；无时间戳、随机 ID、I/O 或额外模型调用。
- Ledger 不是独立的事实核验引擎，只能重现现有 Validator 允许的证据颗粒度；Research Manager 本身的推理正确性依然需要人工审查。

## 验收

- 单测覆盖三种状态、后续反驳链接、Manager Opportunity/Risk 结构化引用、稳定排序/相同 JSON、原始输入不可变及引用 Slice 不共享。
- 异常测试覆盖未知/重复/冲突 ID、目标不是前序论点、标的不匹配、事件引用不存在及领域错用；失败时不返回 Ledger。
- Workflow 测试确认新的 `evidence_ledger` 在实际 `PortfolioReviewResult` 中出现，不依赖或信任 `TradeIntent.Evidence` 的自由文本。
- 普通 CI：`go vet ./...`、`go test ./...`、CLI 黑盒检查。**不会重复运行 V2.11.4 已验收且未更改的 Hosted Integration**，也不能以正常工作流 smoke 替代本增量的确定性测试。
