// Test-only implementation of docs/WEB_RESEARCH_API.md. NEVER runs an Agent,
// contacts a market-data provider, or serves real financial research.
import { createServer } from "node:http";
import { randomBytes } from "node:crypto";

const host = "127.0.0.1";
const port = 18686;
const secret = "ci-go-api-fixture-only-secret-32-characters";
const runs = new Map();
const requestKeys = new Map();
const prefix = "/api/v1/research/runs";
const providerSecrets = new Map();
const providerNames = ["DEEPSEEK_API_KEY", "TUSHARE_TOKEN", "TAVILY_API_KEY"];
const providerStatus = () => ({ editable: true, configured: Object.fromEntries(providerNames.map(n => [n, providerSecrets.has(n)])) });
const asOf = () => new Date().toISOString();
const json = (res, code, value) => {
  res.writeHead(code, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  res.end(JSON.stringify(value));
};
function append(run, stage, status, duration_ms) {
  run.events.push({ seq: run.events.length + 1, stage, status, at: asOf(), ...(duration_ms ? { duration_ms } : {}) });
}
function evolve(run) {
  if (run.state === "CANCELLED" || run.state === "FAILED" || run.state === "COMPLETED") return;
  const ms = Date.now() - run.createdMs;
  if (ms >= 250 && run.state === "PENDING") {
    run.state = "RUNNING";
    run.started_at = asOf();
    append(run, "Run", "RUNNING");
  }
  if (run.state !== "RUNNING") return;
  if (ms >= 800 && !run.stage1) { run.stage1 = true; append(run, "Market", "START"); }
  if (ms >= 1200 && !run.stage2) { run.stage2 = true; append(run, "Market", "DONE", 400); }
  if (ms >= 1550 && !run.stage3) { run.stage3 = true; append(run, "Event", "START"); }
  if (run.scenario !== "hold" && ms >= 2200) {
    if (!run.stage4) { run.stage4 = true; append(run, "Event", run.scenario === "fail" ? "FAIL" : "DONE", 650); }
    run.state = run.scenario === "fail" ? "FAILED" : "COMPLETED";
    if (run.state === "FAILED") run.failure_code = "RESEARCH_FAILED";
    run.finished_at = asOf();
    append(run, "Run", run.state);
  }
}
function metadata(run) {
  const { run_id, symbol, mode, requested_as_of, state, created_at, started_at, finished_at, failure_code, rerun_of } = run;
  return { run_id, symbol, mode, requested_as_of, state, created_at, ...(started_at ? { started_at } : {}), ...(finished_at ? { finished_at } : {}), ...(failure_code ? { failure_code } : {}), ...(rerun_of ? { rerun_of } : {}) };
}
function report(run) {
  // Synthetic protocol values only: verify UI maps Go JSON paths without
  // implying that any finance provider, model or actual account was queried.
  return {
    research_artifact: {
      symbol: run.symbol,
      synthesis: {
        summary: "PROTOCOL_FIXTURE_ONLY — synthetic research synthesis for browser contract testing",
        agreements: [{ claim: "FIXTURE_MARKET_AGREEMENT", references: [{ domain: "MARKET" }] }],
        conflicts: [],
        risks: ["FIXTURE_NO_LIVE_MARKET_QUOTE"],
      },
      market: { as_of: "20261009", regime: "MIXED", advance_count: 0, decline_count: 2,
        turnover: 0, evidence: ["FIXTURE_MARKET_ONLY"], risk: ["no provider"] },
      technical: { symbol: run.symbol, requested_as_of: "20261011", effective_as_of: "20261009",
        last_trade_date: "20261009", status: "NO_DATA", returns: {}, moving_averages: {},
        momentum: { macd: {} }, volume: {}, evidence: [], risk: [] },
      event: { symbol: run.symbol, requested_as_of: "2026-10-11T00:00:00Z",
        evidence_as_of: "2026-10-11T00:05:00Z", status: "NO_EVENT", events: [] },
    },
    portfolio_review: {
      research_plan: { symbol: run.symbol, conclusion: "INSUFFICIENT_EVIDENCE",
        summary: "FIXTURE_ONLY_MANAGER_NO_RESEARCH",
        opportunities: [], risks: [], unresolved_argument_ids: ["fixture-a1"], trading_constraints: [] },
      debate_state: { symbol: run.symbol, turns: [{ round: 1, side: "BULL",
        thesis: "FIXTURE_BULL_ONLY", arguments: [{
          id: "fixture-a1", claim: "FIXTURE_UNRESOLVED_ARGUMENT", references: [{ domain: "MARKET" }],
        }], rebuttals: [] }] },
      evidence_ledger: { symbol: run.symbol, conclusion: "INSUFFICIENT_EVIDENCE",
        arguments: [{ id: "fixture-a1", side: "BULL", round: 1, claim: "FIXTURE_UNRESOLVED_ARGUMENT",
          references: [{ domain: "MARKET" }], disposition: "UNRESOLVED" }], insights: [] },
      trade_intent: { symbol: run.symbol, status: "MONITOR", thesis: "FIXTURE_NOT_A_TRADE",
        evidence: [], risk: [] },
      risk_context: { symbol: run.symbol, assessment: "INSUFFICIENT_CONTEXT",
        summary: "FIXTURE_MISSING_ACCOUNT_INFORMATION", findings: [], constraints: [],
        unresolved: [{ claim: "FIXTURE_ACCOUNT_UNKNOWN" }] },
      portfolio_decision: { symbol: run.symbol, disposition: "REVIEW_REQUIRED",
        reason_code: "ACCOUNT_DATA_ABSENT", summary: "FIXTURE_ACCOUNT_UNKNOWN",
        portfolio_state: "NOT_PROVIDED", risk_assessment: "INSUFFICIENT_CONTEXT" },
    },
    final_risk_gate: { disposition: "REVIEW_REQUIRED", reason: "FIXTURE_NO_REAL_ACCOUNT" },
  };
}
async function readBody(req) {
  const parts = [];
  for await (const p of req) { parts.push(p); if (parts.reduce((n, x) => n + x.length, 0) > 16384) throw Error("oversize"); }
  return JSON.parse(Buffer.concat(parts).toString("utf8"));
}
const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://${host}:${port}`);
  if (url.pathname === "/healthz") return json(res, 200, { fixture: true });
  if (process.env.AEGIS_TEST_FIXTURE_LOCAL_DEV !== "1" && req.headers.authorization !== `Bearer ${secret}`) return json(res, 401, { error: "unauthorized" });
  if (url.pathname === "/api/v1/settings/credentials") {
    if (req.method === "GET") return json(res, 200, providerStatus());
    if (req.method === "PUT") {
      let body;
      try { body = await readBody(req); } catch { return json(res, 400, { error: "invalid" }); }
      for (const [key, value] of Object.entries(body.changes ?? {})) {
        if (!providerNames.includes(key) || typeof value !== "string" || value.length < 8) return json(res, 400, { error: "invalid" });
        providerSecrets.set(key, value);
      }
      for (const key of body.clear ?? []) {
        if (!providerNames.includes(key)) return json(res, 400, { error: "invalid" });
        providerSecrets.delete(key);
      }
      return json(res, 200, providerStatus());
    }
    return json(res, 405, { error: "method not allowed" });
  }
  if (url.pathname === prefix && req.method === "GET") {
    const list = [...runs.values()].reverse().map(r => { evolve(r); return metadata(r); });
    return json(res, 200, { runs: list });
  }
  if (url.pathname === prefix && req.method === "POST") {
    let body;
    try { body = await readBody(req); } catch { return json(res, 400, { error: "bad JSON" }); }
    if (!/^[0-9]{6}\.(SH|SZ)$/.test(body.symbol ?? "") || body.mode !== "PORTFOLIO_REVIEW" || body.portfolio?.state !== "NOT_PROVIDED") return json(res, 400, { error: "invalid fixture request" });
    const key = req.headers["idempotency-key"];
    if (key && requestKeys.has(key)) {
      const previous = requestKeys.get(key);
      if (previous.body !== JSON.stringify(body)) return json(res, 409, { error: "idempotency conflict" });
      return json(res, 202, metadata(runs.get(previous.run_id)));
    }
    let origin;
    if (body.rerun_of !== undefined) {
      if (typeof body.rerun_of !== "string" || !/^[a-f0-9]{32}$/.test(body.rerun_of))
        return json(res, 400, { error: "invalid origin id" });
      origin = runs.get(body.rerun_of);
      if (!origin) return json(res, 400, { error: "origin not found" });
      evolve(origin);
      if (!["COMPLETED", "FAILED", "CANCELLED"].includes(origin.state) || origin.symbol !== body.symbol || origin.mode !== body.mode)
        return json(res, 400, { error: "invalid rerun origin" });
    }
    const run_id = randomBytes(16).toString("hex");
    const created_at = asOf();
    const scenario = body.market_query?.includes("hold") ? "hold" : body.market_query?.includes("fail") ? "fail" : "complete";
    const run = { run_id, symbol: body.symbol, mode: body.mode, requested_as_of: created_at, created_at, createdMs: Date.now(), state: "PENDING", scenario, events: [], ...(origin ? { rerun_of: origin.run_id } : {}) };
    append(run, "Run", "PENDING");
    runs.set(run_id, run);
    if (key) requestKeys.set(key, { body: JSON.stringify(body), run_id });
    res.setHeader("Location", `${prefix}/${run_id}`);
    return json(res, 202, metadata(run));
  }
  const match = url.pathname.match(/^\/api\/v1\/research\/runs\/([a-f0-9]{32})(?:\/(artifact|events|stream|cancel))?$/);
  if (!match) return json(res, 404, { error: "not found" });
  const run = runs.get(match[1]);
  if (!run) return json(res, 404, { error: "not found" });
  evolve(run);
  const action = match[2];
  if (!action && req.method === "GET") return json(res, 200, metadata(run));
  if (action === "cancel" && req.method === "POST") {
    if (run.state === "CANCELLED" || run.state === "FAILED" || run.state === "COMPLETED") return json(res, 409, { error: "not active" });
    run.state = "CANCELLED"; run.finished_at = asOf(); append(run, "Run", "CANCELLED");
    return json(res, 202, { status: "CANCELLATION_REQUESTED" });
  }
  if (action === "artifact" && req.method === "GET") return run.state === "COMPLETED" ? json(res, 200, report(run)) : json(res, 409, { error: "artifact not ready" });
  if (action === "events" && req.method === "GET") return json(res, 200, { events: run.events });
  if (action === "stream" && req.method === "GET") {
    const raw = req.headers["last-event-id"] || "0";
    if (!/^\d{1,20}$/.test(raw)) return json(res, 400, { error: "invalid cursor" });
    let cursor = Number(raw);
    res.writeHead(200, { "Content-Type": "text/event-stream; charset=utf-8", "Cache-Control": "no-store", "Connection": "keep-alive" });
    const pump = () => {
      evolve(run);
      for (const event of run.events) {
        if (event.seq <= cursor) continue;
        res.write(`id: ${event.seq}\nevent: research_stage\ndata: ${JSON.stringify(event)}\n\n`);
        cursor = event.seq;
      }
      if (["COMPLETED", "FAILED", "CANCELLED"].includes(run.state)) { clearInterval(timer); res.end(); }
    };
    const timer = setInterval(pump, 125);
    res.on("close", () => clearInterval(timer));
    pump();
    return;
  }
  return json(res, 405, { error: "method not allowed" });
});
server.listen(port, host, () => console.log(`Fixture Go API protocol server at http://${host}:${port}`));
