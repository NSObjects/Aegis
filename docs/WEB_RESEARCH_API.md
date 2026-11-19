# Aegis Web Research API — first single-node slice

This is a **single-operator, single-process** research API, not an order
execution service. It is built on the existing Go/Eino workflows via the
`internal/application` research factory. It does not accept real trading or
capital transfers.

## Run locally

Build from the PR branch, with Go **1.26**:

```sh
export AEGIS_WEB_API_TOKEN='<generate-a-strong-random-secret-of-at-least-24-bytes>'
go run ./cmd/aegis-web \
  -listen 127.0.0.1:8686 \
  -store .aegis/runs \
  -portfolio-review=true
```

The existing model and provider configuration/credentials are still required.
The server constructs production models/providers at startup. Keep its token,
model and data provider credentials **on the server**, not in Next.js client
JavaScript. The API deliberately does **not** send cross-origin authorization
headers: use a server-side Next.js adapter/proxy for browsers. The token must
be set explicitly; `NewHandler` rejects tokens shorter than 24 bytes. Default
network binding is loopback; TLS, authorization per user and tenant isolation
are **not** implemented. Do not expose this service directly to the Internet.

## HTTP endpoints

All requests require `Authorization: Bearer $AEGIS_WEB_API_TOKEN`.
All responses use `Cache-Control: no-store`.

| Method | Route | Result |
| --- | --- | --- |
| POST | `/api/v1/research/runs` | `202` with persisted `PENDING` Run, RunID and `Location` |
| GET | `/api/v1/research/runs` | most recent 100 Run metadata records (no artifacts or events) |
| GET | `/api/v1/research/runs/{run_id}` | Run metadata and terminal status (no artifact) |
| POST | `/api/v1/research/runs/{run_id}/cancel` | `202` cancellation requested (not yet terminal) |
| GET | `/api/v1/research/runs/{run_id}/artifact` | raw original typed JSON **only when COMPLETED**, otherwise `409` |
| GET | `/api/v1/research/runs/{run_id}/events` | persisted ordered `RunEvent[]` for HTTP polling/debugging |
| GET | `/api/v1/research/runs/{run_id}/stream` | `text/event-stream`, ordered `research_stage` events |

The server is configured for either `DECISION` or `PORTFOLIO_REVIEW` mode. The
POST `mode` is optional and defaults to the server's configured mode. A
request with a different mode fails rather than constructing another graph.

Example for full Portfolio Review:

```sh
curl -sS -X POST http://127.0.0.1:8686/api/v1/research/runs \
  -H "Authorization: Bearer $AEGIS_WEB_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"symbol":"600519.SH","portfolio":{"state":"NOT_PROVIDED"}}'
```

`NOT_PROVIDED` means the actual account/holdings are **unknown**, *not*
`EMPTY`, zero cash or zero positions. The server never invents portfolio
snapshots. In `DECISION` mode omit `portfolio`. The server establishes a
current live research cutoff and keeps the original CLI PIT evidence task
parameters (lookback is configured server-side). There is **no arbitrary
historical-as-of HTTP parameter** in this first slice.

### 监控提醒与研究摘要

配置服务进程的 `TUSHARE_TOKEN`，再通过 `POST /api/v1/monitors` 建立规则、通过 `POST /api/v1/schedules` 创建启用的 `MONITOR` 任务。监控在交易日当地 18 点起执行，默认时区为上海。公告可触发一项 `DECISION` 研究。价格和量能提醒记录行情变化。

`GET /api/v1/research-digest?period=daily` 返回当日提醒。`period=weekly` 返回截至指定日期的七个日历日。可用 `through=YYYYMMDD` 查询历史范围。响应包含记录的行情或公告变化、数据源故障数、关联触发状态、研究和此前报告的 ID。观察池呈现同一摘要。数据不可用会显示为告警。

### 研究假设

在已完成报告中展开「研究假设」，录入陈述、有效期、失效条件、验证规则与正反证据。浏览器按上海时间解释有效期。

`GET /api/v1/research/runs/{run_id}/hypotheses` 返回该 Run 的假设。向相同路径发送 `POST`，提交 `statement`、RFC3339 格式的 `expires_at`、`invalidation` 和 `verification_rule`。可选的 `supporting_evidence` 与 `contrary_evidence` 各接受最多八条 `{ "reference": "...", "detail": "..." }`。证据由操作者引用。可选的 `close_rule` 指定 `trade_date`（YYYYMMDD）、`operator`（GTE 或 LTE）和正数 `threshold`，用于核验指定交易日的未复权收盘价。也可改用 `document_rule`，指定 `document_id` 和 `on_match`（SUPPORTED 或 REFUTED）。公告须为报告所引用的原始来源，在原研究时点之后发布，并在有效期及后续报告时点内可用。来源 Run、标的、研究时点和报告 SHA-256 由服务端取得。相同内容重试返回原记录。每份报告最多保存 50 条假设。

通过 `GET /api/v1/research/runs/{run_id}/hypotheses/{hypothesis_id}/observations` 查看后验观察。向同一路径 POST 后续报告的 `run_id`，可为同一标的的原假设追加验证结果。服务端从后续报告读取行情或公告证据。缺失数据或文本规则在有效期内返回 `INCONCLUSIVE`，期满后仍缺可判定证据则返回 `EXPIRED`。每条观察保存后续报告的 ID、时点和指纹。

在后验观察下展开「错误复盘」可归档原因。通过 `GET /api/v1/research/runs/{run_id}/hypotheses/{hypothesis_id}/reviews` 读取复盘。向相同路径 POST，提交已关联观察的 `observation_run_id`、`category`、`reason`、至少一条 `supporting_evidence` 和可选 `counterexamples`。两类证据各最多八条，沿用假设的来源与说明字段。类别值为 `DATA_MISSING`、`CITATION_ERROR`、`REASONING_BIAS`、`CONTEXT_CHANGED` 或 `HINDSIGHT_BIAS`。复盘是操作者归因记录，每个假设最多保存 50 条。

### 研究记忆

从报告的后验观察选择「收入研究记忆」，可选填市场阶段或关联该观察的复盘。在「研究档案」展开「研究记忆」检索。列表中的原报告与后续报告可直接打开。

- `POST /api/v1/research/memory` 接受 `source_run_id`、`hypothesis_id`、`observation_run_id`，以及可选的 `review_id`、`market_phase`。标的、论点、结果和失败类别由已保存的来源记录派生。
- `GET /api/v1/research/memory` 支持 `symbol`、`q`（论点关键词）、`market_phase`、`category`、`outcome`、`offset`。每页最多 50 条，按入库时间倒序返回。
- `POST /api/v1/research/memory/{memory_id}/revoke` 保存撤销时间，并将样本从检索中移除。原始假设、观察和复盘继续保留。

对话的只读工具 `ReadResearchMemory` 按标的和可选论点关键词查询既有假设、最新观察和未撤销的记忆样本。结果包含原报告和后续报告的 ID、研究时点及人工复盘出处。历史观察仅用于回顾其记录时点。

定时研究和监控触发研究进入完成状态时，会对研究开始前已录入、在其研究时点仍有效的同标的结构化假设追加后验观察。收盘价规则仅在目标日收盘后评估，缺少合格证据时保留证据不足。同一后续报告重复轮询不会产生重复观察。组合审查的定时任务以显式未知账户启动。

### 策略定义与扫描回放

`go run ./cmd/aegis-pullback-scan --describe-strategy` 离线输出当前注册的策略定义与 SHA-256。新扫描的输入指纹同时绑定策略参数指纹。通过 `GET /api/v1/scans/{scan_id}/replay` 按原版本重算已保存的输入。响应包含 `strategy_definition`、`strategy_definition_sha256` 和 `strategy_definition_bound`。旧扫描按记录版本解析定义，此时 `strategy_definition_bound` 为 `false`。无法核实参数指纹时，回放返回错误。

### Retry-safe Run creation

The optional `Idempotency-Key` header on `POST /api/v1/research/runs` is
recommended for the real Web client. Use a freshly generated opaque 8–128
character ASCII key (letters, digits, `_`, `-`, `.`, `:`) for each
**intended new** research request. Preserve the same key if the HTTP response
is lost and the client retries the **same** request.

- Matching key and request identity return the original Run ID, even across
  process restart or while the original study is still running. No second
  provider workflow starts.
- A reused key with different symbol, mode, query, account facts or evidence
  lookback returns HTTP `409` without invoking providers.
- A new research attempt, including an intentional rerun of the same stock,
  **requires a new key**.
- Auto-generated research cutoff timestamps do not cause retry fingerprint
  mismatches. The original run retains its original cutoff, never a newer
  silently refreshed snapshot.
- Raw keys are not stored. Private RunStore files contain only SHA-256 key
  digests and request fingerprints; GET/list API responses remove both.
  This is **one-process** idempotency, not a cross-node guarantee.

```sh
curl -sS -X POST http://127.0.0.1:8686/api/v1/research/runs \
  -H "Authorization: Bearer $AEGIS_WEB_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: study-600519-20261011-001' \
  -d '{"symbol":"600519.SH","portfolio":{"state":"NOT_PROVIDED"}}'
```

### Stage stream / reconnect

The SSE stream replays persisted stage events from the beginning unless the
request includes `Last-Event-ID: <seq>`, in which case only events with a
strictly larger sequence are sent. Events include `seq`, `stage`, `status`,
`at` and optional `duration_ms`. A terminal Run closes the stream after
replaying remaining events. Clients may reconnect with the last received ID;
no event stream depends solely on in-memory subscribers.

The API uses a polling-backed SSE sender; it is **not** a distributed event
broker. `research.WithProgressEvents` provides the typed stage callback from
existing Workflow stages; it does **not** parse CLI text logs or expose
prompts, tool arguments, credentials, raw account data, model chain-of-thought,
or internal agent thoughts. Stage events are best-effort with a durable
snapshot at each accepted event; persistence failure results in a failed run,
not a fabricated successful artifact.

Example raw SSE request (for a Next.js server-side adapter, not browser JS):

```sh
curl -N http://127.0.0.1:8686/api/v1/research/runs/$RUN_ID/stream \
  -H "Authorization: Bearer $AEGIS_WEB_API_TOKEN" \
  -H 'Last-Event-ID: 0'
```

## Boundaries and next steps

- This slice depends on **PR #130** (shared live ResearchService) and
  **PR #131** (single-process RunManager/Store), in that order.
- The FileRunStore must be **owned by one process only**. Storage is private
  (0700 dir, 0600 JSON). No cross-process lock or distributed scheduling.
- No per-user authorization, CORS, reverse-proxy auth or multi-tenancy exists.
  `AEGIS_WEB_API_TOKEN` grants access to **every** stored research output;
  keep the endpoint loopback and token server-side.
- Research runs survive restarts only as saved **COMPLETED/FAILED/CANCELLED**
  records. Unfinished paid work is marked `PROCESS_INTERRUPTED` on restart,
  never automatically re-executed.
- This is the backend protocol **foundation**, not yet a wired Web UI. The
  next implementation can create a Next.js server adapter that maps the real
  Run/Artifact/Event objects to the existing workbench components. There are
  no trading or live portfolio-management commands here.


## Provider credentials from Web settings (single operator)

The Go server supports optional encrypted provider credentials. Generate a
persistent 32-byte random master key and provide its **64-character hex** value
as `AEGIS_CREDENTIAL_MASTER_KEY` to `cmd/aegis-web`, for example:
`openssl rand -hex 32`. Keep this master key in the deployment secret manager,
NOT in a browser, Web form, Git repository, research artifact, or logfile.
Start the Go server with its normal API token and this master key before
opening the Live Web settings page. No DeepSeek/TuShare/Tavily credentials are
needed to **start** the research server: model and provider dependencies are
built for each new Run using the most recently saved credential values.
Credentials changed while another Run is active are used only for new Runs.

The default encrypted file is `.aegis/credentials.enc` (customize with
`-credential-store`). AES-256-GCM is used with random nonces; the directory
must be 0700 and the file 0600. Back up the master key securely: if it is
lost, saved credentials are not recoverable. Keep the file on a trusted local
filesystem; do not mount the same credential store into multiple Go servers.
Do not expose these single-operator settings publicly.

Authenticated Go API operations:
- `GET /api/v1/settings/credentials` returns **only** booleans
  `editable` and `configured` for `DEEPSEEK_API_KEY`,
  `TUSHARE_TOKEN`, `TAVILY_API_KEY`; no key values or suffixes.
- `PUT /api/v1/settings/credentials` accepts a strict object such as
  `{"changes":{"TUSHARE_TOKEN":"..."}}`. Omitted keys are unchanged.
  To delete stored overrides use `{"clear":["TUSHARE_TOKEN"]}`.
  If an environment variable exists it remains the fallback when the
  encrypted override is deleted. No master key means read-only status.
  The API requires the same server Bearer token as ResearchRun operations.

Only provider credentials are managed here. Go/Next API tokens, Web access
password, session signing secret and encryption master key remain deployment
settings and cannot be edited over the network.

The Live Web settings page uses the existing HttpOnly session, same-origin
checks and Next.js server-side Bearer proxy. Neither GET nor PUT replies
include plaintext secrets. Secret entries are never persisted in localStorage.


### R1-03 submitted request snapshot (v1)

New ResearchRun records contain a durable `request_snapshot` in the usual
GET/list run metadata. It is frozen **before** asynchronous execution and
restored from the same private FileRunStore after restart. Fields are a
strict allowlist: `schema_version: 1`, `market_query_provided` (boolean),
`event_window_start/end`, `event_pit_safe`, requested
`preferred_source_types`, `min_documents`, `min_independent_sources`,
and optional explicit `portfolio_state` (e.g. `NOT_PROVIDED` = unknown).
There is **no** raw market_query, portfolio snapshot, provider API key, or
private model prompt in this projection. The request object is not persisted.

This is a **submitted-input** snapshot, not a claim about what external
sources were actually fetched or which model version was used at execution.
Model/profile/provider execution provenance and source document indexing
remain separate R1-03 work. Older records without `request_snapshot` are
still readable and must be shown as missing historical metadata instead of
being reconstructed from today's configuration. The Run artifact has its
independent version/checksum; do not confuse it with this snapshot schema.


### R1-03 executed model graph identity (schema v1)

New Run metadata can include `model_provenance` **after** the live Go
`NewLiveResearchService` successfully constructs its graph (not at Submit).
The RunManager persists it **before** the paid workflow begins; it remains
available for COMPLETED, FAILED or CANCELLED runs as long as graph
construction actually succeeded. Older runs and failures before
initialization omit the field without inventing a version.

`schema_version: 1`, `config_fingerprint`, and `bindings` each containing
`role`, `profile`, `provider`, `model` come from the **same in-memory model
configuration** used by that graph. The fingerprint is a deterministic SHA256
of precisely these ordered role bindings, **not** a whole-file hash. Raw API
keys, API key environment variable names, BaseURLs, header secrets, prompts,
and portfolio details are **not serialized** in model provenance.

Configured `model` is the API model ID, not an independently verified
provider revision. This metadata cannot prove which model artifact the
remote provider deployed or guarantee the LLM actually returned an answer.
The deterministic data-source/document provenance index remains an
independent R1-03 deliverable. API `GET /runs/:id` and `GET /runs`
include this metadata as optional fields, while artifact JSON remains
on the authorized completed-only artifact endpoint.


### R1-03 Event evidence identity index in completed Run metadata

New completed **PORTFOLIO_REVIEW** runs with a typed `research_artifact.event`
include `evidence_index`, projected deterministically from the exact result
serialized into the same Run record (the source of truth). Format:

- `schema_version: 1`, `status` (`NORMAL`, `NO_EVENT`,
  `INSUFFICIENT_EVIDENCE`), `requested_as_of`, optional `evidence_as_of`;
- `events`: stable sorted event IDs, occurrence timestamps where available,
  the EventContext's **reported** `source_count`, and deduplicated/sorted
  document IDs drawn from event/fact/grounding identities;
- `document_ids`: sorted union of all referenced documents in those events.

Index and Artifact are saved together on the terminal transition.
`FileRunStore` checks that any persisted index can be regenerated exactly
from its persisted Artifact, in addition to the existing Artifact SHA256
check. Restarted managers read the same index; historical pre-index records
retain no field, not a reconstructed guess. `NO_EVENT` is explicit and
different from absent metadata. Failed/CANCELLED/PENDING tasks cannot contain
an index. Decision mode does not contain a research `EventContext` and does
not gain a fictional index.

**Limitations:** Event IDs/document IDs and source_count here are from the
Go Typed Artifact. They are **not** raw source archives, independently
authenticated publishers, canonical URLs, document content hashes or proof
of semantic entailment. `evidence_as_of` is the EventContext observation
timestamp and can be later than the requested PIT cutoff as described in
the existing live capture contract. Per-document publication/availability
timestamps and independently verifiable source URLs are not represented
by this index. Original evidence snapshot archival and one-to-one source
linkage remain further R1-03 verification work.


### R1-03 cited canonical source metadata (same-Run capture)

For a **new successfully completed Portfolio Review** whose Event Agent produced
grounded events, `evidence_index.documents` can now contain canonical metadata
for each actually referenced document: `document_id`, `version_id`,
`content_sha256` (the canonicalizer's processed text, **not** a separately
verified web byte archive), `publisher`, safely publishable `url` when
available, `source_type`, `authority`, `is_primary`, `published_at`,
`available_at`, and `first_seen_at`.

The Event Agent takes these fields directly from the final research round's
PIT-filtered `CanonicalDocument` set, restricted to document IDs cited in
that run's grounded events. Metadata is stored as part of
`research_artifact.event.cited_document_sources` and deterministically
projected into the completed Run index, so FileRunStore can check it against
the exact saved Artifact after restart. **No current-day snapshot lookup** or
second crawler pass is used to fill old metadata. When no source metadata
exists (legacy or externally assembled Artifact), `documents` is absent.

`available_at` is eligibility at the Event Agent's evidence observation
cutoff (T1); it must not be presented as proof of availability at the original
user's submission timestamp T0. Unsafe original URLs (including HTTP,
userinfo, search/query tokens, fragments) are not exposed. An acceptable
HTTPS URL is a **recorded source pointer**, not proof that the page remains
reachable, that its contents are authentic, or that its claims are true.
This change does not expose raw retrieved document contents or create an
independent custody-verified original source archive.
