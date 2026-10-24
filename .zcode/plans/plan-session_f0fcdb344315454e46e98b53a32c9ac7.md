# R3-02 实施计划：策略扫描任务、解释结果与数据快照（#208）

## 架构决策（已与用户确认）

- **独立 `ScanManager`**，复刻 RunManager 模式（状态机、先落盘再执行、启动中断核对、终态互斥切换），不与 LLM RunManager 耦合——扫描是纯确定性 Go 计算，无 LLM（路线图禁令）
- **数据获取按交易日批量**：`trade_cal`（1 次）取 asOf 往前 90 个开市日 → `stock_basic list_status=L` 全量（1 次）→ 逐交易日 `daily?trade_date=`（90 次）。共约 92 次 Tushare 调用，远优于按只拉取的 ~8000 次
- **股票池**：缺省全市场在市股票（含 .BJ，前缀淘汰交给 Evaluate 记录 REJECTED）；请求可选 `symbols` 子集（显式模式下校验存在且在市，否则 400）
- **回看窗口 90 交易日**，作为参数固化进请求快照，保证可重放
- **asOf 必须是交易日**：启动时查 `trade_cal`，非开市日直接拒绝请求（避免全市场无意义 INSUFFICIENT）
- **存储**：沿用文件型原子 JSON（0700 目录、0600、fsync+rename、DisallowUnknownFields、schema_version）；per-symbol bars 快照分文件，主记录只存结果 + SHA256 引用（bars 总量 ~60MB 会撞 64MB 单文件上限）

## 新增文件

1. **`internal/application/scan_manager.go`**
   - `ScanState`：PENDING/RUNNING/COMPLETED/FAILED/CANCELLED（对齐架构文档 03 的 screening Run Envelope 草案）
   - `ScanRequestSnapshot{SchemaVersion:1, AsOf, StrategyVersion:"trend-pullback-v1", Symbols []string(nil=全市场), LookbackTradingDays:90, CreatedAt}`
   - `ScanResultItem{Symbol, Verdict, Reason, BarsCount, FirstBarDate, LastBarDate, SnapshotSHA256}`
   - `ScanRecord{ID, State, RequestSnapshot, Summary{Total,Selected,Rejected,Insufficient}, Progress{Evaluated,DataDays}, FailureCode, CreatedAt/StartedAt/FinishedAt}`
   - `ScanStore` 接口（Save/Get/List）+ 启动中断核对：遗留 PENDING/RUNNING → FAILED(PROCESS_INTERRUPTED)
   - 并发：slots=1（同时一个扫描）、maxActive=2、runTimeout flag、Cancel、CloseContext 排水

2. **`internal/application/file_scan_store.go`** — `FileScanStore`：主记录 `<dir>/scans/<id>.json`（每次读写全量重校验，仿 checkRunRecord）；快照 `<dir>/scans/<id>/snapshots/<symbol>.json`（bars + 校验和验证）

3. **`internal/capability/technical/market_scanner.go`**
   - `BatchMarketSource` 接口：`TradingDays(ctx, asOf, n)` / `ListedSymbols(ctx)` / `DailyBars(ctx, tradeDate)` → 生产实现包装 `TushareClient.query`
   - 按标的分组构造 `[]HistoricalBar`，单位转换与 HTTPResearcher 一致（vol×100 股、amount×1000 元），按日期排序

4. **`internal/httpapi/scan_handler.go`** — `ScanController` 接口 + 路由分支 `/api/v1/scans`（复用 Bearer 鉴权 + 限流）：
   - `POST /api/v1/scans`（202 + Location，Idempotency-Key，16KB 上限，DisallowUnknownFields）
   - `GET /api/v1/scans`（state/limit/offset 过滤，信封格式）
   - `GET /api/v1/scans/{id}`（状态 + 汇总 + results）
   - `GET /api/v1/scans/{id}/results/{symbol}`（单标的判定与原因）
   - `GET /api/v1/scans/{id}/results/{symbol}/snapshot`（当时 bars，可复核输入；仅 COMPLETED 可取，409 语义对齐 artifact 端点）
   - `POST /api/v1/scans/{id}/cancel`

5. **`cmd/aegis-web/main.go`** — flags（scan-timeout、scan 并发参数）+ `NewFileScanStore` + `NewScanManager` + `handler.WithScanController(...)`；Tushare token 从现有 credentialStore 运行时获取

6. **`docs/WEB_RESEARCH_API.md`** — 增补 scans 端点契约

## 执行流程

校验冻结 → 随机 ID → Save PENDING → goroutine：trade_cal 校验 asOf 开市 → stock_basic → 逐日拉 daily（串行，每步检查 ctx）→ 分组 → 逐只 `pullback.Evaluate`（任何硬错误按 fail-closed 记为该标的 FAILED 结果不中断整体？**否**——Evaluate 硬错误仅来自输入畸形，扫描侧数据已校验，出现即记该标的 `Verdict:"ERROR"` 单独条目不吞；tushare 调用失败 → 整体 FAILED(DATA_UNAVAILABLE)，已完成的分文件快照保留）→ per-symbol 快照落盘 → 汇总 → COMPLETED。

## 测试（针对性自动测试，对齐验收）

- `scan_manager_test.go`：正常全流程（fake source）、单标的缺数据 → INSUFFICIENT_DATA 不当满足、tushare 中途失败 → FAILED 且保留部分快照、取消 → CANCELLED、重启中断核对、同请求重放结果一致（可重放验收）
- `file_scan_store_test.go`：round-trip、坏记录/未知字段拒绝、快照 SHA256 不匹配拒绝
- `scan_handler_test.go`：202+Location、非交易日 400、symbols 不存在 400、list 信封、快照 409、cancel
- `market_scanner_test.go`：分组、单位转换、停牌缺口（某日无行 → 该标的窗口内 bar 缺失）

## 提交

单 PR（对齐仓库惯例），commit `feat(r3-02): async strategy scan jobs with per-symbol reasons and input snapshots`；`go vet ./...` + `go test ./...` 全绿；PR 描述记录固定 SHA 的 CI 证据，合并后在 #208 关闭评论记录合并 SHA。