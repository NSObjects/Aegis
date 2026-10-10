# Live Go/Eino ResearchRun acceptance

This is an **opt-in paid** end-to-end verification of the real Go HTTP ResearchRun, SSE stage events and typed output. It does not use the browser's protocol fixture and never executes orders.

## Run locally after configuring providers

1. From the repository root, start `go run ./cmd/aegis-web -dev`.
2. In another terminal run `cd web && npm run dev:live`, then open http://127.0.0.1:3000 and configure DeepSeek, TuShare and Tavily keys in **凭证设置**. The next research run reads these Go-side encrypted credentials.
3. From the repository root run `go run ./cmd/aegis-live-check -symbol 600519.SH -timeout 25m`.

The verifier uses `PORTFOLIO_REVIEW` with an **explicit** account state of `NOT_PROVIDED`, not `EMPTY` or fictitious cash/positions. It listens for real persisted SSE events and requires Market, Technical, Event, Synthesis, Debate, ResearchManager, Trading, Risk, Portfolio and FinalRiskGate to finish successfully. It then checks the actual completed JSON artifact and consistency of the final risk disposition and unknown account state.

To test only the Decision workflow, start the Go server with `-portfolio-review=false` and invoke `go run ./cmd/aegis-live-check -mode DECISION`. The verifier accepts only loopback HTTP origins. An optional production Bearer can be supplied via the existing server-only `AEGIS_WEB_API_TOKEN` environment variable.

Errors, interrupted or cancelled runs, invalid stage sequence, missing research domains or contradictory financial state fail closed. The command prints stage names, statuses and a run ID, **not** raw provider responses, confidential keys, account snapshots or full research artifacts. It does not automatically retry costly calls.

## Hosted opt-in workflow

The existing GitHub Actions **Integration** workflow now includes `run_live_web_api`, **off by default**. If explicitly enabled, it uses configured repository secrets (`DEEPSEEK_API_KEY`, `TUSHARE_TOKEN`, `TAVILY_API_KEY`) to build and start the real Go service on a private loopback address, then triggers exactly one full live ResearchRun through HTTP/SSE.

Standard `CI`, `Web Research Workspace` CI and Chromium fixtures do **not** invoke paid providers. A passing synthetic-browser or mocked-HTTP test is not evidence of a successful live research run or source quality. Real financial evidence and model conclusions still require independent human review.
