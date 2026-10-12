// CI-only loopback supervisor for REAL Go process restart testing.
// Never expose the control port outside 127.0.0.1.
import { createServer } from "node:http";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import { once } from "node:events";

const root = resolve(process.cwd(), "..");
const dir = mkdtempSync(join(tmpdir(), "aegis-browser-restart-"));
const binary = join(dir, "aegis-web");
const api = "http://127.0.0.1:18789/api/v1/research/runs";
const env = Object.fromEntries(Object.entries(process.env).filter(([k]) =>
  !/^(DEEPSEEK_|TUSHARE_|TAVILY_|AEGIS_WEB_|AEGIS_CREDENTIAL_MASTER_KEY$)/.test(k)));
execFileSync("go", ["build", "-o", binary, "./cmd/aegis-web"], { cwd: root, env, stdio: "inherit" });
let child;
let shuttingDown = false;
async function start() {
  child = spawn(binary, ["-dev", "-listen", "127.0.0.1:18789",
    "-store", join(dir, "runs"), "-credential-store", join(dir, "credentials.enc"),
    "-evidence-store", join(dir, "evidence")], { cwd: root, env, stdio: "inherit" });
  const current = child;
  const end = Date.now() + 15000;
  while (Date.now() < end) {
    if (current.exitCode !== null) throw new Error("Go service exited during startup");
    try {
      const response = await fetch(api, { signal: AbortSignal.timeout(800) });
      if (response.ok) return;
    } catch {}
    await new Promise(r => setTimeout(r, 80));
  }
  throw new Error("Go service did not start");
}
async function stop() {
  const prior = child;
  if (!prior || prior.exitCode !== null) return;
  prior.kill("SIGKILL");
  await once(prior, "exit");
}
let operation = Promise.resolve();
const server = createServer((req, res) => {
  if (req.url === "/health" && req.method === "GET") {
    res.writeHead(200); res.end("ready"); return;
  }
  if (req.url !== "/restart" || req.method !== "POST") {
    res.writeHead(404); res.end(); return;
  }
  operation = operation.then(async () => {
    await stop();
    await start();
    res.writeHead(200); res.end("restarted");
  }).catch(err => {
    res.writeHead(500); res.end(String(err));
  });
});
async function shutdown() {
  if (shuttingDown) return;
  shuttingDown = true;
  server.close();
  await stop();
  rmSync(dir, { recursive: true, force: true });
}
process.on("SIGTERM", () => { shutdown().finally(() => process.exit(0)); });
process.on("SIGINT", () => { shutdown().finally(() => process.exit(0)); });
await start();
server.listen(18788, "127.0.0.1");
