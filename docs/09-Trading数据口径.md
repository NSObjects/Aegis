# Trading Agent 个股研究数据口径

> 状态：V2.6 当前规范
> 目标：固定 Trading Agent 的数据语义，再实现确定性计算脚本。本阶段不新增 Agent，不修改 Market → Trading Graph。

## 1. 核心流水线

```text
原始数据
  ↓
时间裁剪 / 状态判定
  ↓
PIT 复权
  ↓
确定性指标计算
  ↓
Trading Agent
  ↓
Decision
```

Trading Agent 负责解释和判断，不负责发明数据口径，也不应临场手算可确定指标。

## 2. 时间语义

- `requested_as_of` 固定等于 `market_context.as_of`，不得改成运行时“今天”。
- `effective_as_of` 是实际用于日线研究的数据日期。
- requested_as_of 为交易所非交易日时，可回退到最近已完成交易日。
- 交易所正常开市但个股停牌时，不得把上一交易日伪装成当日数据；必须显式标记 `SUSPENDED`，并保留 `last_trade_date`。
- 日线研究默认要求 final bar；盘中未完成日线不能当完整收盘数据。

至少区分以下状态：

```text
NORMAL
SUSPENDED
NOT_LISTED
DELISTED
NO_DATA
```

周末/节假日、停牌、未上市、退市、错误代码、数据源暂时无数据不能全部压成“空数组”。

## 3. 价格与复权

目标日真实行情字段使用未复权价格：

```text
open / high / low / close / pre_close / pct_change / volume / amount
```

技术指标统一使用 `PIT_QFQ`：以 `effective_as_of` 为锚点的点时前复权。

```text
adjusted_price(t)
  = raw_price(t)
  × adj_factor(t)
  / adj_factor(effective_as_of)
```

约束：

- 只读取 `<= effective_as_of` 的复权因子。
- 先裁剪时间，再做复权，再算指标。
- 不得使用当前最新前复权历史序列回算历史研究。
- 不得让未来除权除息反向改写历史研究结果。

## 4. 基础必需数据

历史窗口至少准备最近 `120` 个交易日，按交易日而不是自然日计算。

基础行情：

```text
OHLC
pre_close
pct_change
volume
amount
turnover_rate（来源可提供时）
```

确定性收益率：

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
MACD.DIF = EMA12 - EMA26
MACD.DEA = EMA9(DIF)
MACD.HIST = 2 × (DIF - DEA)
```

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

这些值由脚本固定计算，LLM 不负责公式计算。

## 5. 可选增强数据

以下不是基础研究成功的硬前置：

```text
money_flow
margin / margin_detail
daily_basic
fundamental
announcement
news
sector / industry
relative_strength
limit_up / limit_down
dragon_tiger
```

可选数据获取失败时，如果基础行情完整，仍可产生 Decision；缺失项写入 `risk`，必要时降低 `confidence`。无权限不等于“没有事件”。

## 6. 相对强弱

若计算相对强弱：

1. 个股与基准使用相同起止交易日。
2. 两侧数据都不得超过 `effective_as_of`。
3. 必须声明基准。
4. 不允许比较不同窗口的收益率。

第一版推荐沪深300，但它属于增强项。

## 7. 单位与覆盖

金额统一在数据能力层归一化成：

```text
CNY yuan
```

千元、万元、亿元不能留给 Trading Agent 猜。

数据能力还必须区分：

```text
实际观测为 0
vs
未覆盖 / 无权限 / 无数据
```

推荐显式提供 `observed` 与 `reason`，避免把缺失值当 0。

## 8. Trading Agent 边界

Trading Agent 可以：解释趋势、比较证据、识别冲突、结合 MarketContext、输出 WATCH/AVOID/CANDIDATE、调整 confidence。

Trading Agent 不应：手算 MA/RSI/MACD、自行决定复权方式、用自然日代替交易日、停牌时静默回退、把缺失当 0、使用 as_of 之后的数据、伪造无权限接口结果。

## 9. 下一实现步骤

下一步只实现 Tushare Skill 的确定性 stock research 脚本：时间裁剪、状态判断、PIT_QFQ、收益率、MA、RSI、MACD、量能。先让脚本独立测试通过，再让 Trading Agent 使用其结果。