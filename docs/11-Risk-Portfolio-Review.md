# V2.10 Risk Debate 决策与 Portfolio Review 边界

> 范围：Issue #75，V2.10.5 评估结论与 V2.10.6 最小实现。

## V2.10.5：Risk Debate 暂缓

TradingAgents 的 Aggressive / Conservative / Neutral 是围绕交易方案的多视角风险辩论。Aegis 当前只有：
- TradeIntent：标的、论点、证据、风险、候选状态、置信度；
- PortfolioContext：NOT_PROVIDED / EMPTY / POSITIONS，已观测时只有 as_of、symbol、quantity；
- RiskContext：risk assessment、finding、constraint、unresolved、typed references。

缺少现金、估值、资产净值、风险预算和限制条件，因此无法量化集中度、相对敞口、可买数量、可用资金或组合风险上限。

现阶段创建三个不同态度的 Risk Agent，只会让它们重复消费同一份信息。**不实现三方 Risk Debate，也不创建无真实职责的空 Agent。**

恢复 Risk Debate 的前置条件：
1. 引入真实观察到的、时间点一致的账户金额/风险预算/约束；
2. 至少识别出两个彼此独立、能够以不同证据与规则验证的风险职责；
3. 单独测试每种角色，证明产生的信息增量，而不是相互复述；
4. 再考虑配置有界轮数、冲突停止规则的 runtime。

## V2.10.6：有界的 Portfolio Decision

Portfolio Manager 当前仅作为**确定性、不可执行的审查边界**，不调用 LLM，不生成下单意图。

| TradeIntent | RiskContext | PortfolioDecision | Reason |
|---|---|---|---|
| MONITOR | 任意合法评估 | NO_ACTION | MONITOR_ONLY |
| REJECTED | 任意合法评估 | NO_ACTION | REJECTED_UPSTREAM |
| CANDIDATE | HIGH_RISK | BLOCKED | HIGH_RISK |
| CANDIDATE | CAUTION | REVIEW_REQUIRED | RISK_CAUTION |
| CANDIDATE | INSUFFICIENT_CONTEXT | REVIEW_REQUIRED | RISK_UNCERTAIN |
| CANDIDATE | ACCEPTABLE | REVIEW_REQUIRED | ACCOUNT_DATA_ABSENT |

无论 PortfolioContext 是 EMPTY 还是 POSITIONS，**它都没有现金、总资产和可执行的风险预算**。因此即使 RiskContext = ACCEPTABLE，也只能继续审查，不能代表批准交易。NOT_PROVIDED 必须配合 INSUFFICIENT_CONTEXT，否则 upstream validator 直接拒绝。

PortfolioDecision 不包含 BUY/SELL、下单数量、交易价格、资金、杠杆或仓位建议。所有状态、原因和摘要由可审计的确定性策略生成并二次验证。

## 后续

- V2.10.7 验证 Research/Trading → TradeIntent → Risk Analyst → Portfolio Review 的完整链路。
- 真实账户与风险预算可用后，再引入更强的 Portfolio Manager 判断与可能的 Risk Debate。
- Memory / Checkpoint、订单执行仍不属于本阶段。
