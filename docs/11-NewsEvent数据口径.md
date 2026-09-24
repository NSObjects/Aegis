# V2.7 News/Event Analyst 数据与能力设计

> 状态：V2.7 当前设计约束
> 更新时间：2026-09-24

## 1. 目标

V2.7 新增 News/Event Analyst，用于回答：

```text
在 requested_as_of 之前，
与指定标的直接相关的哪些可验证事件已经发生或正式披露？
这些事实可能意味着什么？
```

News/Event Analyst 不负责技术指标、基本面、资金流或最终交易决策。

输出：

```text
EventContext
```

## 2. 从 BettaFish 借鉴的核心模式

参考 BettaFish / MindSpider 的两个阶段：

```text
广域发现
  ↓
热点 / 关键词
  ↓
定向深挖
```

以及 MediaEngine / QueryEngine 的 provider-neutral 搜索工具思想：

```text
Agent intent
  ↓
domain-oriented search capability
  ↓
Bocha / Anspire / Tavily / other provider
```

Aegis 不复制其实现，而只借鉴职责拆分。

Aegis 的事件研究要求更严格，因为还需要满足历史 as_of / PIT 语义。

## 3. Aegis 的正确分层

```text
                    Event Ingestion
                         │
      ┌──────────────────┼──────────────────┐
      │                  │                  │
Official Disclosure   Finance News      General / Social
      │                  │                  │
      └────────── Normalize / Archive ─────┘
                         │
                         ▼
                   Event Evidence Store
                         │
                         ▼
              stock_event_research Tool
                         │
                         ▼
                 News/Event Analyst
                         │
                         ▼
                    EventContext
```

其中 Official Disclosure 可以按历史日期直接查询。

热点 / 社媒 / NewsNow 这类“latest”数据不能在历史研究时实时回查来冒充过去信息，必须提前归档或由支持历史时间范围的 Provider 获取。

## 4. 采集与查询必须分开

错误：

```text
研究 2025-01-01
  ↓
请求今天的 NewsNow latest
  ↓
声称得到 2025-01-01 的 EventContext
```

这是未来数据泄漏。

正确：

```text
实时采集
  ↓
按 first_seen_at / published_at 落库
  ↓
历史研究只读取 <= requested_as_of 的归档数据
```

因此 V2.7 有两个独立职责：

```text
Ingestion
→ 多源抓取、归一化、存档

Research
→ 读取 requested_as_of 可见的事件证据
```

Agent 不直接承担定时采集。

## 5. Source 分层

### Tier A：Official Disclosure

用于高可信事实：

- 上市公司公告
- 交易所公告
- 监管披露
- 公司正式新闻稿

第一套实现：

```text
Tushare anns_d
```

### Tier B：Financial News

A 股优先考虑：

- 财联社
- 华尔街见闻
- 金十数据
- 格隆汇
- 其他财经媒体

### Tier C：General News

例如：

- 澎湃
- 今日头条
- 其他综合媒体

### Tier D：Social / Community Discovery

例如：

- 雪球
- 微博
- 知乎
- 抖音
- Bilibili
- 贴吧

该层默认用于：

- 事件发现
- 传播度
- 舆情线索

不能仅凭社交热度把某个未经证实的说法写成 Event.fact。

### Targeted Web Search

Bocha / Tavily / Anspire / Web Search 等属于定向搜索 Capability。

历史研究必须满足：

- 可限制时间范围；
- 能保留 published_at；
- 结果不使用 requested_as_of 之后内容。

若无法保证，应标记 coverage 不完整，不能假装 PIT 完整。

## 6. Source Capability

Event Agent 不感知具体 Provider。

```text
stock_event_research
  ↓
MultiSourceResearcher
  ├── OfficialDisclosureSource
  ├── LocalNewsArchiveSource
  ├── TargetedSearchSource
  └── ...
```

每个 Source 必须输出：

```text
source_id
source_name
kind
coverage status
item_count
raw items
```

Source 失败默认 fail-open，进入 coverage：

```text
FAILED
```

而不是让另一个成功来源被一并丢弃。

## 7. Coverage 是一级数据

多渠道系统不能只返回“有新闻 / 没新闻”。

EventContext 必须保存每个来源的覆盖情况：

```text
COVERED
EMPTY
FAILED
UNAVAILABLE
```

例如：

```json
{
  "source_id": "cls-hot",
  "source_name": "财联社热门",
  "kind": "FINANCIAL_NEWS",
  "status": "COVERED",
  "item_count": 20
}
```

只有在预期来源均成功查询、且均没有相关事件时，才可以高置信输出 NO_EVENT。

部分来源失败时，应使用 PARTIAL。

## 8. EventContext 状态

```text
NORMAL
→ 至少有一个可验证事件

NO_EVENT
→ 已完成足够覆盖，窗口内没有相关事件

PARTIAL
→ 部分来源缺失 / 失败，仍有可用研究结果

NO_DATA
→ 无法形成可靠事件研究
```

## 9. Fact 与 Interpretation 强制分离

每条 Event：

```text
Fact
→ 来源实际披露 / 报道了什么

Interpretation
→ 该事实可能对短线研究意味着什么
```

例如：

```text
Fact:
公司公告披露三季度业绩预告。

Interpretation:
盈利变化可能影响市场对未来盈利预期。
```

禁止：

```text
Fact:
公司明显利好，股价即将上涨。
```

## 10. 多来源去重与交叉验证

相同事件可能同时出现在：

```text
公司公告
财联社
华尔街见闻
雪球
微博
```

EventContext 不应简单产生五条重复 Event。

目标结构：

```text
Canonical Event
  ├── SourceRef A
  ├── SourceRef B
  └── SourceRef C
```

Event.sources 保留所有可验证来源。

官方披露与权威媒体可提高事实可信度，但“来源数量多”本身不等于事件一定重要。

## 11. BettaFish / NewsNow 给 Aegis 的渠道参考

BettaFish 的 BroadTopicExtraction 使用多热点源做广域发现。

Aegis 可以参考其中财经相关 Source：

```text
cls-hot
wallstreetcn-quick
xueqiu-hotstock
jin10
gelonghui
```

以及通用发现 Source：

```text
weibo
toutiao
zhihu
thepaper
douyin
bilibili-hot-search
```

但生产环境不应把公共 NewsNow 示例实例当成可靠基础设施。

更合适的方式：

```text
自建 NewsNow / 可控 Feed
        ↓
持续采集
        ↓
本地 Event Store
```

## 12. 与 Sentiment Analyst 的边界

News/Event Analyst 负责：

- 事件事实
- 来源
- 时间
- 基于事实的事件解释

Sentiment Analyst（未来）负责：

- 讨论热度
- 正负面情绪分布
- 社媒观点结构
- 情绪变化

因此：

```text
雪球 / 微博 / 知乎
```

既可以作为 Event discovery 来源，也可能成为未来 Sentiment Capability 的输入。

不能因为 V2.7 使用了社媒来源，就让 Event Agent 永久吞掉 Sentiment Role。

## 13. V2.7 实现顺序

```text
1. EventContext / Validate
2. MultiSource Event Capability contract
3. Official Disclosure Source
4. Event Agent
5. 独立 integration
6. Multi-source news ingestion / archive
7. LocalNewsArchiveSource
8. 再验证 Event Agent
9. 接入 Graph
10. Trading 消费 EventContext
```

在第 5 步之前不接 Graph。

在本地新闻归档能力可验证前，不把 NewsNow latest 直接放进历史 Research Graph。
