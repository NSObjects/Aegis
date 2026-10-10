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
