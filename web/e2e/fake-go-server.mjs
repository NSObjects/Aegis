// Test-only implementation of docs/WEB_RESEARCH_API.md. NEVER runs an Agent,
// contacts a market-data provider, or serves real financial research.
import { createServer } from "node:http";
import { randomBytes } from "node:crypto";

const host = "127.0.0.1";
const port = 18686;
const secret = "ci-go-api-fixture-only-secret-32-characters";
const runs = new Map();
const prefix = "/api/v1/research/runs";
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
  const { run_id, symbol, mode, requested_as_of, state, created_at, started_at, finished_at, failure_code } = run;
  return { run_id, symbol, mode, requested_as_of, state, created_at, ...(started_at ? { started_at } : {}), ...(finished_at ? { finished_at } : {}), ...(failure_code ? { failure_code } : {}) };
}
function report(run) {
  return {
    research_artifact: { symbol: run.symbol, synthesis: { summary: "PROTOCOL_FIXTURE_ONLY — simulated research artifact for browser contract testing", agreements: [], conflicts: [], risks: [] } },
    portfolio_review: { trade_intent: { status: "MONITOR" }, evidence_ledger: { entries: [] } },
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
  if (req.headers.authorization !== `Bearer ${secret}`) return json(res, 401, { error: "unauthorized" });
  if (url.pathname === prefix && req.method === "GET") {
    const list = [...runs.values()].reverse().map(r => { evolve(r); return metadata(r); });
    return json(res, 200, { runs: list });
  }
  if (url.pathname === prefix && req.method === "POST") {
    let body;
    try { body = await readBody(req); } catch { return json(res, 400, { error: "bad JSON" }); }
    if (!/^[0-9]{6}\.(SH|SZ)$/.test(body.symbol ?? "") || body.mode !== "PORTFOLIO_REVIEW" || body.portfolio?.state !== "NOT_PROVIDED") return json(res, 400, { error: "invalid fixture request" });
    const run_id = randomBytes(16).toString("hex");
    const created_at = asOf();
    const scenario = body.market_query?.includes("hold") ? "hold" : body.market_query?.includes("fail") ? "fail" : "complete";
    const run = { run_id, symbol: body.symbol, mode: body.mode, requested_as_of: created_at, created_at, createdMs: Date.now(), state: "PENDING", scenario, events: [] };
    append(run, "Run", "PENDING");
    runs.set(run_id, run);
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
