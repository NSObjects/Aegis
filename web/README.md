# Aegis Web · Research Workspace (mock-first)

Standalone Next.js App Router UI living in `web/`, intentionally isolated from the Go/Eino core.

## Run locally

Requires Node.js >= 20.9 and npm. From the repository root:

```bash
cd web
npm install
npm run dev
# http://localhost:3000
```

Static checks after installing dependencies:

```bash
npm run typecheck
npm run lint
npm run test
npm run build
```

**This PR ships a mock frontend only**. All price paths, stock information, research opinions, research stages, evidence and history are **synthetic demo data** — not market quotes, not an LLM output and not financial advice. The app prominently labels the mode in its UI. It only persists dummy demo task state in localStorage, not financial or account data. Clicking "开始演示研究" simulates progress locally and does not start Go agents or query APIs. Each simulated task has its own status/Run ID, and can be cancelled, retried, or opened from history. Switch the scenario picker to simulate an Event provider failure; the UI deliberately hides reports for failed/cancelled runs. Run history is stored in browser localStorage (demo-only, last 50 entries); unfinished timers are marked failed on refresh to avoid implying server persistence.

## Visual & component foundation

- Next.js App Router, React, TypeScript, Tailwind CSS v4.
- Accessible, high-contrast CSS tokens designed as a financial research terminal; Lucide icons and Recharts charts.
- `components.json` is prepared for importing official `shadcn/ui` components (`npx shadcn@latest add ...`) as real interaction needs emerge; this first slice does not require a component registry download.
- Responsive shell, usable research view, interactive demo run, stock switching, research stage inspection, evidence panel, history table, provider settings status, chart range switch and JSON export.

## Go integration hand-off (not implemented yet)

`lib/research-contract.ts` describes the frontend-facing boundary. When the Go API is ready, introduce an HTTP/SSE adapter behind `ResearchGateway` without replacing UI components. Server-side Go/Eino remains the only source of validated financial artifacts.

Recommended minimal operations:

- `startResearch(input) → {run_id, status}` (idempotent async request)
- `getResearchRun(run_id) → ResearchRun`
- `listResearchRuns(filters) → ResearchRun[]`
- `getResearchReport(run_id) → ResearchReport`
- `subscribeRunEvents(run_id, cursor)` using SSE, resumable by event cursor

`research_artifact`, `portfolio_review` and `final_risk_gate` are existing Go JSON fields; they are **not** reinterpreted as trade instructions. Account state `NOT_PROVIDED` must render as unknown, never an empty portfolio. Event and task DTOs in this UI are proposed frontend contracts, not representations of currently implemented Go endpoints.

## Follow-up implementation PRs

1. Review/freeze minimal HTTP DTOs against current Go typed artifacts.
2. Expose a shared Go application service and task/event persistence.
3. Replace the mock gateway and simulated timers with HTTP + SSE events.
4. Add real identity/authorization and browser E2E coverage.

No changes in this PR to the Go agent graph, CLI, CI secrets, execution permissions or broker integration.

## Demo run lifecycle

- Create a new demo run from the workbench. The independent local timer advances all running mock tasks, even after switching stocks.
- Select **数据源失败** to simulate a fail-closed Event Analyst. Failed and cancelled runs do not receive a report or allow JSON export.
- Cancel a running demo, open older run IDs from the archive, and filter history by status. Refresh recovers history; any unfinished local-only run is marked failed with `DEMO_SESSION_INTERRUPTED`.
- Browser localStorage holds at most 50 mock records; nothing is submitted to Aegis Go. Clearing site storage resets the demo. Real HTTP/SSE task storage is a future backend slice.

## Research trace & evidence navigator (mock-only)

- Completed demo runs expose a linked research reading view with four synthetic
  claims (Bull, Bear, Research Manager and Risk), sample debate rounds and a
  non-executable handoff. Select a claim to highlight its linked evidence and
  Agent stage; select evidence to see which claims reference it.
- These relationships are deliberately generated from static per-stock fixtures.
  **Opening multiple historical Demo runs for the same stock does not create
  genuine historical evidence or independent Agent outputs.** Each report is
  clearly identified by its mock run ID, and does not claim live provenance.
- A failed, cancelled, pending or still-running demo run has no completed report,
  research trace or evidence inspection. The JSON export includes a trace only
  for a completed demo run. The Go `EvidenceLedger` and typed Debate/Manager
  artifacts will replace this fixture mapping when real APIs are available.
- `npm test` includes fail-closed trace tests, missing-reference checks and
  claim/evidence/stage referential-integrity assertions.

## Live Go adapter (first integrated slice)

The Web now supports two **mutually exclusive** modes. Default **demo** remains
synthetic, clearly labelled and never contacts Go. With `AEGIS_WEB_MODE=live`
the home page switches to a separate, real-data-only research console, not a
mixture of demo stock prices and live research results.

1. Start the Go service with `go run ./cmd/aegis-web -listen 127.0.0.1:8686 -portfolio-review=true` and `AEGIS_WEB_API_TOKEN` (24+ random bytes), as documented in `docs/WEB_RESEARCH_API.md`.
2. Set these variables **in the Next.js server environment only**, not in client-side code: `AEGIS_WEB_MODE=live`, `AEGIS_WEB_API_URL=http://127.0.0.1:8686`, `AEGIS_WEB_API_TOKEN` (matching Go), `AEGIS_WEB_ACCESS_PASSWORD` (independent 16+ char password), `AEGIS_WEB_SESSION_SECRET` (32+ random characters), `AEGIS_WEB_RESEARCH_MODE=PORTFOLIO_REVIEW` (must match Go; `DECISION` is supported when Go starts in decision mode).
3. `cd web && npm install && npm run dev`; open `http://localhost:3000`, enter the Web access password. Only a short-lived HttpOnly, SameSite=Strict signed session cookie is placed in the browser. The Go API token stays exclusively in the server runtime and is injected only by the Next.js BFF.
4. Enter an A-share symbol (e.g. `600519.SH`), start a real Run, inspect persisted stages via SSE and status polling, cancel work, open past runs, then read/download the **exact typed** completed JSON artifact. In portfolio review mode, the frontend explicitly passes `{ "state": "NOT_PROVIDED" }` meaning account state **unknown** — not zero holdings.

The BFF is an **allowlisted, same-origin** `/api/workbench/runs/...` adapter.
It never redirects to an arbitrary URL or forwards user-supplied auth headers.
It supports Last-Event-ID forwarding and uses a streaming `Response` instead of
buffering SSE. Backend is deliberately limited to one process and one operator;
all runs are visible to anyone with the Web session. **Do not expose the Next.js
or Go service publicly without additional TLS/reverse-proxy access controls,
rate limits and a full user authentication/authorization design.** The simple
login endpoint is not a complete Internet-facing identity provider. The Go
backend has no live market chart endpoint yet; live mode therefore **does not
show synthetic prices, scores or evidence citations**. Do not mistake the
absence of those views for unavailable or negative market data.

## Browser contract E2E (no real provider credentials)

The Web CI includes a separate Chromium Playwright job. It starts a **synthetic
protocol fixture** at `127.0.0.1:18686` implementing only the subset of Go
ResearchRun API exercised by the app, then starts the **production Next.js
server** in Live mode at `127.0.0.1:3100`. Run from `web/`:

```bash
npm install
npx playwright install chromium
npm run build
npm run e2e
```

These tests cover browser login, 401 for anonymous requests, explicit unknown
portfolio input, creation of a real HTTP Run, SSE stage updates, `Last-Event-ID`
replay, completed artifact retrieval, history after reload, cancellation and
provider failure with no artifact. The mock's text includes
`PROTOCOL_FIXTURE_ONLY` so it **cannot be mistaken for Go/Eino output or a
market-data source**. These tests verify browser↔BFF↔HTTP protocol wiring,
**not** real Go/Eino research, provider credentials, point-in-time finance
accuracy or multi-user production deployment. Real integrated Hosted E2E
remains separately required before claiming the workstation is production ready.


## Live settings: provider credentials

With `AEGIS_WEB_MODE=live`, sign in and open **凭证设置** from the sidebar.
The page groups third-party credentials by purpose: **AI models**
(DeepSeek, `DEEPSEEK_API_KEY`), **financial data** (TuShare,
`TUSHARE_TOKEN`) and **information search** (optional Tavily,
`TAVILY_API_KEY`). Each group displays configured/total counts and
each provider displays its configured state. One Save operation supports
updates across groups; blank fields preserve their prior values. Clearing
one provider override also preserves unsaved edits in other groups.
Inputs are password fields and only submitted/cleared inputs are reset
after a successful request. The UI receives only availability booleans,
never stored secret values, partial masks or decrypted responses.
Additional vendors must be wired into the Go provider runtime and
server allowlist before they can be offered as functional settings.

To activate editing, set `AEGIS_CREDENTIAL_MASTER_KEY` (64 hex characters,
generate with `openssl rand -hex 32`) **on the Go server** before it starts.
The Go API stores secrets encrypted at `.aegis/credentials.enc` or
`-credential-store`. The settings API can be read with environment-only
credentials even when no master key is configured, but cannot write without
one. If model/provider keys are absent at Go startup, the server starts and
new runs fail closed until keys are provided; the UI must not claim successful
research in that case.

The browser never receives AEGIS_WEB_API_TOKEN,
AEGIS_WEB_ACCESS_PASSWORD, AEGIS_WEB_SESSION_SECRET or the encryption key.
Those deployment secrets must be installed separately; this page manages
**provider** keys only. Already-running research jobs retain their existing
provider configuration; newly started jobs read updated overrides.

## 本地免鉴权联调（单机开发专用）

使用两个终端，无需设置 `AEGIS_WEB_API_TOKEN`、网页登录密码、Session Secret 或加密主密钥：

```bash
# Aegis 仓库根目录
go run ./cmd/aegis-web -dev

# 第二个终端
cd web
npm run dev:live
```

访问 http://127.0.0.1:3000 ，直接进入 Live 研究工作台。点击侧栏「凭证设置」填写 DeepSeek / TuShare / Tavily 密钥。
Go 第一次运行时生成 `.aegis/.local-dev-master.key`（已被 Git 忽略）并在下次运行复用；密钥与密文存放在同一个本地目录，因此这只是调试便利功能，不是针对主机失陷的安全防护。

只有显式 `-dev` 加上 literal loopback IP 时 Go 才免鉴权；Web 只有 `dev:live` 且 Next.js `development` runtime 才免登录。生产模式不变。不要通过外网端口转发、反向代理、隧道或共享电脑暴露该模式。

## Live 研究任务固定链接（R1-05 增量）

Live 工作台从 URL 恢复当前页面和服务端 Run：

- `/?run=<32 位 run_id>`：打开指定研究运行的详情、执行事件与已完成的后端工件；刷新后仍保留。
- `/?view=history`：研究历史；`/?view=settings`：凭证设置。
- 侧栏/历史表点击会更新浏览器 URL；前进、后退恢复相同工作区。任务详情提供「任务固定链接」。
- 如果某个 Run 已不在最近 100 条列表中，Web 用精确 ID 向 Go API 查询；若 ID 不存在/无访问权限，则明确提示，不会替换成另一任务的报告。
- 固定链接只包含不透明 Run ID，不包含密钥或 Artifact；访问时仍必须通过现有会话验证。Demo 模式不适用上述 Live URL 约定。

此改动只解决 URL 导航与恢复；不表示整个 R1-05（完整页面路由）或 R1 验收完成。


## Live 结构化研究报告（R1-08 第一批）

在 `AEGIS_WEB_MODE=live` 下，Go 返回的 **COMPLETED** 原始 Typed Artifact 除 JSON 导出外，现按真实 Go 字段分区展示：`research_artifact.synthesis/market/technical/event/fundamental`、`portfolio_review.debate_state/research_plan/trade_intent/risk_context/portfolio_decision/evidence_ledger`、`final_risk_gate`。

- 原始结果仍是唯一真源。Web 不重新运行模型、不编写投资结论、不把 `NO_DATA` 转换成零值，不生成来源链接；仅显示确实存在的源字段。
- 报告显式展示 `requested_as_of`，以及后端提供的市场数据日、技术有效交易日、事件证据观察截止和财报实际公告日；不把历史价格误称为最新行情。
- `NOT_PROVIDED` 是账户未知，不是 EMPTY/0 持仓。可选 Fundamental、观察敞口和独立 Limits 未返回时不展示伪数值。
- Bull/Bear 的回合、论点 ID、反驳关系与 EvidenceLedger 分类、引用域、事件 ID 原样展示。单纯 MARKET/TECHNICAL/EVENT 领域引用不等于存在可核查的具体文档。
- 保留「高级：查看后端原始 JSON 工件」及完整 JSON 下载，便于与 Go 输出逐字核对。
- Chromium 协议测试使用 `PROTOCOL_FIXTURE_ONLY` 模拟数据；它只验证 UI 与 HTTP 契约，**不是**真实金融研究 E2E 或供应商数据正确性验收。

后续 R1-10 再实现证据/论点 ID 的交互跳转和原文审计，R1-11 再做跨运行比较。当前阅读器只负责只读报告。

## R1-06：研究任务再次研究（独立 Run）

在 Live 工作台中，打开已完成 / 失败 / 已取消的研究任务，选择「再次研究该标的」。Web **只预填股票代码**，不会假装能从原 Run 恢复研究关注点、账户状态、历史数据或模型配置。用户需要重新核实本次关注点，点击「启动真实研究」才会请求模型和数据源。

- 新的 POST 将可选 `rerun_of`（原 Run ID）与显式新参数发送给 Go；服务端校验原任务存在、属于同标的/模式且已到终态。
- 新任务使用全新 `Idempotency-Key` 和研究截止时间。用户重复提交同一次因网络响应丢失而不确定的请求时，可以安全复用该 key；主动再次研究生成不同 key。
- Go 在私有 RunStore 中持久化原任务 ID，并通过新 Run 的元数据返回 `rerun_of`；历史刷新/重启后仍可从新任务回到来源任务。来源任务内容不被覆盖。
- 任务未到终态不能再次研究。失败/取消任务不显示伪造报告；新任务是否成功只由新的 Go 运行状态决定。
- 这不是历史回放功能，也不是“精确复用原参数”：原始研究请求内容和旧账户事实不会由 Web 复制。当前接口仅研究当前时点；历史 PIT 重放属于后续独立工作。

## Live EvidenceLedger 交互式导航（R1-10 第一批）

仅在已完成的 Go `PORTFOLIO_REVIEW` 原始工件中存在 EvidenceLedger 的论点/观点时展示「论点与事件证据导航」：

- 点选 Bull/Bear 论点或 Research Manager Insight，可查看它的 ID、阵营、回合、接受/未决状态、结构化来源 Reference 与显式针对该论点的反驳。
- **仅**当引用是 `domain: EVENT` 且具有 `event_id`，并且此 ID 精确存在于**同一次 Run** 的 `research_artifact.event.events` 中，才允许继续打开事件详情；内容仅包含 Go 保存的 `fact_summary`、时间与 `evidence_document_ids`。
- 领域引用（例如 `MARKET` 无 EventID）、缺失/无效的 EventID、没有文档 ID 时给出明确差异说明，绝不构造 URL 或把来源 ID 说成已核实原文。
- `TradeIntent.Evidence` 是自由文本，不得被解析为引用关系。完整证据原文/URL 审计依赖后续服务端能力，当前不提供。
- Run 的证据导航组件以 run_id 隔离选择状态，换 Run 不复用上一次选择；FAILED/CANCELLED 不显示成功工件或证据导航。
- 单元测试断言严格引用边界；浏览器测试使用独立的 `PROTOCOL_FIXTURE_ONLY` 人工 JSON 截取 BFF 的 Artifact 响应，仅测试 UI，不证明实时数据来源可靠或真实 Hosted 模型结果。

## R1-07 Live Agent 执行轨迹（持久化事实源）

工作台右侧「Agent 执行轨迹」读取 **Go RunEvent** 持久化事件（`seq`, `stage`, `status`, `at`, 可选 `duration_ms`），按实际观测到的阶段和 sequence 渲染。新的 UI **不假设图谱中所有 Agent 均已执行**：

- START → 执行中；DONE → 成功；FAIL → 该阶段明确失败；若 Run 已进入终态而某阶段仅留下 START，则展示「未记录完成」，**不是**自动判成功或失败。
- 原始状态不认识时标记「未知状态」并保留状态原文；未出现的 Fundamental 或其他阶段，不判定为「跳过」或「成功」。
- `Run` 生命周期与 Agent 阶段分开展示；失败且没有阶段 FAIL 时，只展示 Run 自身 `failure_code`，不会猜测哪个 Agent 失败。
- 仅在 **同一个已完成 Run** 的 Typed Artifact 确实存在相应报告分区时，允许从阶段点击定位报告；失败/取消运行没有虚构结果链接。
- EventSource 用于低延迟展示；浏览器会对活动 Run 周期性查询服务端 `/events`，切换为终态再补读一次。历史 Run 直接恢复持久化日志；SSE 中断不是状态成功的证据。
- 目前 Go Stage Events **不含脱敏工具调用、来源信息、模型 prompts、链式推理**。UI 明确告知不可用；R1-07 工具级追踪仍需先设计、验证并接入受保护的 Go 事件契约。此 PR 不声称这些字段已经存在。

E2E 仅验证固定的 Go 协议 fixture；不是实盘、行情或真实模型数据验证。


## R1-03: submitted research request snapshot

For new Live runs, the task detail panel has an expandable **提交时研究配置快照**
section. It shows only server-saved request constraints: evidence window and PIT
requirement, preferred (not necessarily contacted) source types, minimum
document/source constraints, whether a research question was provided (not its
text), and an optional explicit account state. `NOT_PROVIDED` always means
unknown, never a zero portfolio.

This is persisted by Go in the single-process RunStore, survives restarts,
and is returned as optional `request_snapshot` metadata on new Run records.
Older runs display **旧任务未记录**. This metadata is not a model/version
capture and does not imply any source successfully returned data.
The browser does not receive provider secrets or stored research-query text.
