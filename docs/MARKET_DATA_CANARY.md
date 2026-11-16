# TuShare 主板行情小样本验收（不启动扫描）

使用 `aegis-market-canary` 在真实全市场扫描前检查 TuShare 接口、日期横截面、PIT 股票范围与磁盘快照大小。**必须由操作者主动开启联网；CI 全部使用模拟 HTTP，不调用真实行情。**

## GitHub Actions 手动执行（不需要本地终端）

仓库已有 `Integration` 工作流会同时提供多个付费研究测试选项；**不要通过 Integration 来尝试行情 Canary**。使用独立工作流即可只执行受限 TuShare 行情验收，不会运行 Agent、LLM、全市场股票扫描或定时任务。

1. 打开 [Aegis Actions → Market Data Canary (manual, no LLM)](https://github.com/NSObjects/Aegis/actions/workflows/market-data-canary.yml)。
2. 选择 **Run workflow**，分支选 `main`，填写已结束的上海市场日期 `as_of`（例如 `20261113`）、`days=1`。在 **`mode` 下拉菜单**里，默认是 **`PREVIEW - no TuShare requests`**：不联网、不读取密钥。**绿色成功只表示预览完成，绝不表示行情验收通过。**
3. 确认仓库 GitHub Actions Secret 已配置 `TUSHARE_TOKEN`。GitHub Secret 名称必须准确，**不要在日志、Issue、PR 或聊天里粘贴 Token**。
4. 再次手动运行相同日期和天数；这次在 **`mode` 下拉菜单中明确选 `LIVE - call TuShare (up to 10 requests)`**。只有选择 LIVE 才会向 TuShare 发出请求（1 日上限计划 6 次、3 日上限计划 10 次）。
5. 查看对应运行的 `Bounded TuShare market data canary` Job 及 Summary。它会输出聚合覆盖率、复权因子配对量、快照大小估算与异常原因；若未成功或 `ready_for_full_pool=false`，不要启动完整主板任务。

该工作流**仅支持手动触发**，不会在 Push、PR 或定时器中运行。此前的 #43797900901 和 #43821653733 都是 Preview，真实 TuShare 步骤被跳过；看到 workflow 的绿色状态不代表 LIVE 成功。GitHub Actions 不需要、也不会通过模型获取 Token。日志中的汇总结果可能对仓库读者可见；请不要把原始数据或密钥添加到汇总里。若仓库没有这个 Secret，执行模式会在第一条行情请求之前报错并退出。

## 连续 3 日 LIVE 结果与后续 90 日验收

2026-11-16 [3 日 LIVE 运行 #43844650357](https://github.com/NSObjects/Aegis/actions/runs/43844650357) 实测（截止日 20261113）通过：

| 交易日 | 冻结主板 | 有效日线 | 同日因子 | 低覆盖警报 |
| --- | ---: | ---: | ---: | --- |
| 20261111 | 3194 | 3191 | 3194 | 否 |
| 20261112 | 3194 | 3190 | 3194 | 否 |
| 20261113 | 3194 | 3189 | 3194 | 否 |

3 天 JSON 快照合计约 3.12 MB，线性外推 90 日约 93.5 MB。**这只是行情日期横截面缓存的大小，不是 Aegis 数据库或历史扫描结果总量。** 3 日 LIVE 不能证明 90 日全部数据可用，因此下一道单独的人工门禁是：

1. 打开 [Full Market Cache Prewarm（manual, no scans）](https://github.com/NSObjects/Aegis/actions/workflows/market-full-prewarm.yml)。
2. **Run workflow** 选择 `main`、`as_of=20261113`。默认 `mode=PREVIEW - no TuShare requests`：不会联网、不会读取密钥；绿色只代表预览成功。
3. 认可额度后，新开一次手动运行，明确选择 `mode=LIVE - full 90-day TuShare prewarm (up to 184 requests)`。需要预先配置仓库 `TUSHARE_TOKEN` GitHub Secret。CLI 通过私有临时目录执行生产 `MarketDayCache.Prepare`，上限计划 3 次 stock_basic、1 次 trade_cal、90 次 daily 和 90 次 adj_factor。
4. 查看日志的 `90-session prewarm: n/90 complete` 进度、日历首末日、数据源覆盖率下限、全量实际 JSON 磁盘字节数以及 `contiguous_65_with_factors`。`ready_for_pool_review=false` 或红色执行失败必须先查明原因，不得直接启动股票池。
5. **即使 LIVE 成功，本工作流也不会启动/存储生产股票池**，不会调用 LLM，不会保留约 90 日原始行情缓存。临时数据在任务结束后删除。正式股票池运行和约 3,194 只证券的历史信号验收是**下一道独立门禁**。

这项 90 日验收复用了生产代码路径，验证数据可读与 65 日连续性，但不能证明 TuShare 历史复权因子的当时真实发布版本或回测 PIT 无前视。这项风险在严格历史回测前仍需单独处理。

## 操作

指定**已结束的上海市场日期**（格式 `YYYYMMDD`）。当天即使已经收盘也不接受，以免取得尚未完整发布的数据。建议先验证最近一个已结束的交易日，再扩大到 3 日。

```bash
# 1. 默认只输出计划：没有任何网络请求，也不读取密钥
go run ./cmd/aegis-market-canary --as-of YYYYMMDD --days 1

# 2. 操作者在本地环境中预先设置 TUSHARE_TOKEN（不要粘贴到命令行或 GitHub）
# 3. 只有显式指定 --execute 才会使用 TuShare 额度
go run ./cmd/aegis-market-canary --as-of YYYYMMDD --days 1 --execute

# 4. 一日验证可靠后，才考虑最多 3 个交易日
go run ./cmd/aegis-market-canary --as-of YYYYMMDD --days 3 --execute
```

**调用上限（按应用发出的 TuShare API 请求计）：** 3 次 `stock_basic` 上市状态、1 次 SSE `trade_cal`、每个交易日 1 次 `daily` 和 1 次 `adj_factor`。所以 1/2/3 天分别最多预期发出 6/8/10 次请求；无重试。HTTP 重定向被拒绝，避免额外请求及 body 中的 Token 泄露。

命令输出 JSON，包含 `main_board_symbols`、每日 `daily_rows`、`factor_rows`、与冻结主板范围匹配的 `main_board_daily`、`main_board_factors`、`paired_rows`、日期快照磁盘字节数，以及 90 日缓存大小**估算**。不包含股票代码全表或凭证。

## 停止条件

若日期不可用、供应商报错、日线或复权因子为空、响应达到单次 6000 行上限、日期/证券污染、重复记录、OHLCV 不合法或缓存校验失败，立即停止；不会退回逐股大规模请求。

若样本覆盖率触发主板任务的保守阈值（当前：有效交易日主板日线低于 80%，复权因子低于 90%），`ready_for_full_pool=false`，命令退出非零状态。其余轻微缺因子会在 `warnings` 中列出，应人工核实。

**注意：** “样本通过”仅表示有限日期和当前供应商数据满足这些检查，不能证明 90 日全部齐全、历史复权因子的真实 PIT 可见性或未来行情质量。尤其不能据此宣称已经跑通整个市场或计算出真实投资收益。

## 数据与权限边界

- 通过 `--execute` 显式联网；代码不会提交、打印或持久化 Token。
- 暂存使用独立的私有临时目录，结束或失败后删除；不会写入生产 `market-day-cache/`、`scans/`、`pools/`。
- 不调用 LLM，不产生 ResearchRun，不交易，不向 Web 传递 Token。
- JSON 中的 `projected_90_day_bytes` 是按样本平均值线性外推的**空间预估**，不包括扫描快照与其他目录，也不是磁盘保留政策。
- 缓存中的复权因子可能被供应商后续修订；严格 PIT 回测需要独立的历史发布版本证据。

## 下一道验收

在本地真实 Token 的小样本验收通过后，保留**脱敏 JSON 输出**（不要上传密钥、完整响应或本地绝对路径），核对 1 日和 3 日的覆盖、供应商额度、实际耗时、内存与存储量。只有在这些结果满足预期后，才由操作者主动决定是否启动主板范围完整扫描。
