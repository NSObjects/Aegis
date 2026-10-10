# 02. 官方能力、日常采集与混合数据查询

> **Aegis Data 是官方内置 Capability，不拆成独立项目、仓库或产品**。以下仅定义职责，不要求独立部署。返回 [索引](README.md)。

## 1. Capability、Provider、MCP、Skill 要严格分层

| 概念 | 回答的问题 | 例子 |
|---|---|---|
| Capability | 需要完成什么 | `market.bars`、`news.search`、`disclosure.search`、`technical.indicators` |
| Provider | 谁能提供数据/计算 | 本地 DB、TuShare、巨潮资讯、服务商新闻接口 |
| Adapter | 怎么接入 | SQL、HTTP API、MCP、Crawler、Web Search |
| Skill | Agent 如何使用/组合能力 | 公告查找与证据核验步骤、外部服务商的操作指南 |

**MCP 是工具协议**，不是数据真实性保证；**Skill 是操作流程/说明**，可能要求 MCP/API/脚本等执行环境，不能当成数据源本身。同一 Capability 可以有多个 Provider，Provider 可以通过不同 Adapter 接入。

服务商有可用 MCP/Skill 时优先评估复用，避免为每个实时源重复开发自定义插件；有成熟 API 或本地 DB 时也不必强造 MCP。爬虫作为最后的合规备选，而不是默认首选。

## 2. 官方维护两条获取路径

```text
 OFFLINE: Scheduler -> TuShare / official APIs -> Raw Archive
            -> Normalize / Validate / Version -> Local Database
                                                    |
 Strategy/Agent -> Semantic Data Capability --------+--->
                                                    |
 ONLINE: Provider Connector -> MCP / API / Search / Crawler
            -> Provenance / PIT / Quality Gate -----+---> Typed Result
```

**离线官方采集**：按交易日批量采集全市场基础资料、交易日历、历史日线、成交量、复权因子等必要数据；增量同步、断点重试、幂等写入、缺口扫描、补采和批次审计。后续有明确用途时再扩充财务、行业、交易事件等数据。不将基础采集工作交给社区，也不要求每个策略下载一份全市场数据。

**在线按需获取**：实时行情、最新公告、新闻、政策、专题信息，按新鲜度和研究需求调用第三方现成 MCP/Skill/API；并按数据性质决定是否归档。Agent 可以发起语义请求，数据是否走本地或在线应由确定性 Resolver 判断。

**这两条都属于 Aegis 官方能力**。后台采集并不需要通过 Eino Loop 调度；运行进程的物理部署方式留到负载实测后再决定。

## 3. Local-first 不等于永远返回数据库

| 研究情形 | 正确行为 |
|---|---|
| 截止某历史交易日的 120 根日 K | 本地满足时点、口径、覆盖与完整性后，优先本地 |
| 今日盘中价格 | 昨日历史日线 + 获许可的最新行情；盘中日 K 标明未完成 |
| 当天最新公告 | 读已归档快照，不足时再调用在线来源；记录来源与版本 |
| 全市场短线扫描 | 批量 SQL/SDK 查询，不能对 5000 股票逐个 LLM Tool Call |
| 历史回测/重放 | 严格 PIT；不得补入目标研究时点之后才可知的事实 |
| 没有满足要求的数据 | 返回 STALE/PARTIAL/UNAVAILABLE/INSUFFICIENT 等显式语义，不能假装成功 |

建议语义请求（**示意，非最终 Schema**）：

```json
{
  "capability": "market.bars",
  "symbols": ["600519.SH"],
  "interval": "1d",
  "lookback": 120,
  "requested_as_of": "2026-10-09T14:00:00+08:00",
  "freshness": "INTRADAY_ALLOWED",
  "price_basis": "PIT_QFQ",
  "allow_partial": false
}
```

结果需包含：requested/effective time、data status、完整性、是否 provisional、price basis、Provider/Adapter、来源与版本、数据可用/采集时间、必要的 raw document 或 dataset 引用。时间、来源和数据完整性不是给 LLM 口头解释就结束，必须在类型和校验中体现。

## 4. PIT、数据质量与存储

- `trade_date` / `period_end` 是数据归属日，不表示该事实在当时已公开。
- `published_at` 是发布时间；`available_at` / 有证据的 `first_seen_at` 才是严格时点可见性的核心依据；`ingested_at` 是 Aegis 实际采集时间。
- 复权/财报重述/更正公告均需版本、修订时间与历史可见性；不得使用未来复权事件或后发公告污染过去的回测。
- 交易日历、停牌、上市/退市、数据缺失、盘中未完成日 K 不能默默按正常收盘处理。
- 缺乏历史发布/可见性证据的数据源，在严格回测时应拒绝或标记“不具备 PIT 证明”。
- 保留原始获取产物、标准化数据与可重算衍生指标的边界；不建议只每天覆盖一份最新表。

## 5. 多来源选择、缓存和熔断

**替代**：同一事实/同口径源可优先使用本地、在线作为回退；记录切换原因。**互补**：公告和新闻信息不同，需组合而非首成功返回。**交叉核验**：多网站转载同一原始新闻不能算独立来源。

默认需 Provider 健康、超时、速率/成本限制、请求合并、有限重试、证据去重、缓存版本/时点隔离、错误与来源记录。不能认为接上 MCP 就自动解决数据质量和许可；调用权限、保存权和社区再分发权须分别核验。

## 6. 与现有代码的实际关系

- `internal/capability/technical/tushare.go`、`http_researcher.go` 已有 TuShare HTTP 和单标的技术计算，但**不能据此宣称已完成全市场本地定时采集**；大量标的逐只在线调用不合适。
- `internal/capability/event/retriever.go` 和 `acquisition_plan.go` 已有 leaf Retriever 与多渠道执行、规范化、交叉核验。
- `internal/capability/event/snapshot_retriever.go` 已有本地证据快照查询，可作为混合查询的起点。
- `internal/middleware/skill.go` 使用 Eino Skill Middleware；其中开发环境宽泛的 execute/依赖安装提示不能原样赋给第三方社区策略。

先在两个不同策略中验证本地和在线数据契约复用，再抽象统一 Capability Catalog，而不是现在重写全部实现。

## 7. 验收样例

1. 本地覆盖完整时查询历史日线：不需要任何外部 API。
2. 本地最后一条是昨日，要求今天盘中：不能把昨日收盘当最新；在线不可用时明确 STALE。
3. 过去某天回测：未来公告、复权、数据修订一律不能进入可见集合。
4. 一路 MCP 超时：记录超时，互补来源可继续，不能制造“已交叉验证”结论。
5. 数据定时采集重试/重复：幂等、可补采、来源与质量可审计。
6. 批量模拟 5000 标的：走 DB 批量路径，无 5000 次 LLM 工具调用。
