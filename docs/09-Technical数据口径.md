# Technical Agent 个股技术研究数据口径

> 状态：V2.8 当前规范
> 更新时间：2026-09-30
>
> 本文定义 Technical Agent / TechnicalContext 使用的个股行情与技术指标语义。数据源可以是 Skill、MCP、API、数据库或其他实现；本文不绑定具体 Provider。

## 1. 正确依赖方向

```text
Raw Data Provider(s)
  ↓
Normalize
  ↓
Time / Status Resolution
  ↓
PIT Adjustment
  ↓
Deterministic Indicators
  ↓
Technical Agent
  ↓
TechnicalContext
  ↓
Trading Agent
```

Technical Agent 负责解释个股技术状态。

Trading Agent 消费 TechnicalContext 做综合判断，不负责重新获取行情或重算指标。

本文中的确定性计算可以由 Go package、受控脚本、服务、MCP、API 或数据库 Capability 实现；具体 Provider 不是架构契约。

当前受控实现位于 `internal/capability/technical/`。Technical Agent 只依赖 `stock_technical_research` Tool；Capability 使用原生 Go HTTP client 调用 Tushare Pro HTTP API，并在 Go 内完成 PIT_QFQ 与确定性指标计算。

## 2. 时间语义

requested_as_of 由上游研究任务决定。进入 Market → Technical → Trading 链路时，默认与 MarketContext.AsOf 对齐。

effective_as_of 表示实际用于日线研究的数据日期。

规则：

1. requested_as_of 为正常交易日且标的有完整收盘数据时，effective_as_of = requested_as_of。
2. requested_as_of 为交易所非交易日时，可回退最近已完成交易日。
3. requested_as_of 为交易日但当日 final bar 尚未发布时，可使用 <= requested_as_of 的最近可用 bar，并显式令 effective_as_of 指向该日期，同时记录 staleness warning。
4. 缺少 requested_as_of 当日 daily row 本身不是停牌证据；只有显式停牌证据才允许标记 SUSPENDED。
5. 停牌必须显式标记，并保留 last_trade_date。
6. 日线研究使用 final bar；盘中未完成日线不得冒充完整收盘数据。
7. 不得使用 requested_as_of 之后的数据。

至少区分：

```text
NORMAL
SUSPENDED
NOT_LISTED
DELISTED
NO_DATA
```

周末/节假日、停牌、未上市、退市、错误代码、数据源暂时无数据不能全部压成“空数组”。

## 3. 原始价格与复权

目标日事实字段使用未复权价格：

```text
open
high
low
close
pre_close
pct_change
volume
amount
```

技术指标使用 PIT_QFQ，即以 effective_as_of 为锚点的点时前复权：

```text
adjusted_price(t)
  = raw_price(t)
  × adj_factor(t)
  / adj_factor(effective_as_of)
```

要求：

- 只读取 <= effective_as_of 的复权因子。
- 先做时间裁剪，再复权，再计算指标。
- 历史研究不得使用未来复权因子。
- 不得用今天看到的最新前复权历史序列回算过去。
- 因 PIT_QFQ 以 effective_as_of 为锚点，所以 adjusted_price(effective_as_of) = raw_price(effective_as_of)。
- 因此当前 snapshot.close 可以与同一 effective_as_of 锚定序列计算出的 MA5 / MA10 / MA20 / MA60 直接比较。
- 不得再把“当前未复权 close 与 PIT_QFQ 均线不能比较”写入 risk；该说法与本项目的锚定公式冲突。

## 4. 基础数据窗口

基础技术研究至少准备最近 120 个交易日，窗口按交易日计算，不按自然日计算。

基础事实：

```text
OHLC
pre_close
pct_change
volume
amount
turnover_rate（可获得时）
```

## 5. 确定性指标

收益率：

```text
return_1d
return_5d
return_10d
return_20d
return_60d
```

均线：

```text
MA5
MA10
MA20
MA60
```

动量：

```text
RSI14
├── 使用 Wilder smoothing
├── 初始 avg_gain / avg_loss = 前 14 个 price delta 的算术平均
└── 后续 = (前值 × 13 + 当前 gain/loss) / 14

MACD
├── EMA12 / EMA26 使用递推 EMA
├── EMA 初值 = 序列第一个有效价格
├── DIF  = EMA12 - EMA26
├── DEA  = EMA9(DIF)
└── HIST = 2 × (DIF - DEA)
```

收益率、MA、RSI、MACD 均基于同一条以 effective_as_of 锚定的 PIT_QFQ close 序列。snapshot.change_percent 仍保持 Tushare 原始日线事实口径，不用技术收益率字段覆盖。

量能：

```text
volume_ma5
volume_ma20
volume_vs_ma5
volume_vs_ma20

amount_ma5
amount_ma20
amount_vs_ma5
amount_vs_ma20
```

这些数值由确定性代码计算，不由 LLM 临场估算。

## 6. 可选增强技术数据

后续可按真实需求增加：

```text
ATR
Bollinger Bands
relative_strength
support / resistance
volatility
gap
limit state
```

加入前必须定义公式、时间窗口、数据口径和缺失语义。

## 7. 非技术数据不属于 TechnicalContext 主职责

```text
money_flow             → Capital Flow Agent
margin / margin_detail → Capital Flow Agent
announcement           → News/Event Agent
news                   → News/Event Agent
fundamental            → Fundamental Agent
financial statements   → Fundamental Agent
```

Technical Agent 可以引用必要元信息，但不能因为当前其他 Agent 尚未实现，就永久吞并这些职责。

## 8. 单位

所有 Context 中的单位必须明确。金额统一为 CNY yuan。

数据源若返回千元、万元、亿元，应在 Capability / normalize 层转换。volume 若数据源单位不是“股”，也应在进入 TechnicalContext 前归一化并记录语义。

## 9. 缺失与覆盖

必须区分：

```text
observed zero
not covered
permission denied
temporarily unavailable
no market data
```

推荐数据能力显式提供 observed、reason、source_time。Provider 名可以作为 provenance 元数据，但不得进入领域判断逻辑。

## 10. TechnicalContext 目标字段

第一版至少表达：

```text
symbol
requested_as_of
effective_as_of
status

snapshot
returns
moving_averages
momentum
volume

evidence
risk
confidence
```

当前 Go 契约已落在 `internal/agent/technical/context.go`，结构字段以该文件为实现事实来源。

Technical Agent 的 evidence / risk 是对技术事实的解释；确定性数值本身应保留结构化字段，不能只埋在自然语言里。

## 11. 与 Trading Agent 的边界

Trading Agent 可以读取 TechnicalContext，与 MarketContext / EventContext / FundamentalContext 比较，识别冲突并输出 Decision。

Trading Agent 不应：

- 重新获取 OHLCV；
- 重算 PIT_QFQ；
- 重算 MA / RSI / MACD；
- 再写临时 Python 重复 Technical Agent 的数据工作；
- 因具体 Provider 不可用而改变核心判断职责。

## 12. 当前实现策略

当前实现：

```text
Technical Agent
  ↓
stock_technical_research Tool
  ↓
Researcher interface
  ↓
HTTPResearcher (Go)
  ↓
TushareClient (Go HTTP)
  ↓
Tushare Pro HTTP API
```

Capability 在 Go 内负责时间/状态解析、最近可用 final bar 回退、PIT_QFQ、MA / RSI / MACD / Return / Volume 等确定性计算。生产与 CI 均不再依赖 Python、pandas 或 Tushare Python SDK。

未来增加 DuckDB、MCP 或其他数据源时，应继续保持 `Researcher` 契约与 TechnicalContext 语义稳定；只有出现真实重复模式后再抽象 Provider Registry。
