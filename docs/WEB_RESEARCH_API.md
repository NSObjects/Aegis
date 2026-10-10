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
