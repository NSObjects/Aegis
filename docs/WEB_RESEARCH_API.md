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
