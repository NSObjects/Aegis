import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET, POST } from "./route";
import { COOKIE_NAME, liveConfig, newSession } from "@/lib/server/live-auth";

const origin = "http://localhost:3000";
const originalEnv = { ...process.env };
let fetchSpy: ReturnType<typeof vi.fn>;

function setup() {
  process.env.AEGIS_WEB_MODE = "live";
  process.env.AEGIS_WEB_API_URL = "http://127.0.0.1:8686";
  process.env.AEGIS_WEB_API_TOKEN = "backend-access-secret-at-least-24-characters";
  process.env.AEGIS_WEB_ACCESS_PASSWORD = "frontend-secret-passphrase";
  process.env.AEGIS_WEB_SESSION_SECRET = "session-signing-secret-very-long-and-random";
  const config = liveConfig();
  if (!config) throw Error("invalid unit test env");
  return config;
}
function request(path: string, { method = "GET", authenticated = true, originHeader = true, body, lastEventId }: { method?: string; authenticated?: boolean; originHeader?: boolean; body?: string; lastEventId?: string } = {}) {
  const headers = new Headers();
  if (authenticated) headers.set("cookie", `${COOKIE_NAME}=${newSession(setup())}`);
  if (method !== "GET" && originHeader) {
    headers.set("origin", origin);
    headers.set("sec-fetch-site", "same-origin");
  }
  if (lastEventId !== undefined) headers.set("last-event-id", lastEventId);
  if (body) headers.set("content-type", "application/json");
  return new NextRequest(`${origin}/api/workbench/runs${path}`, { method, headers, body });
}
const params = (segments?: string[]) => ({ params: Promise.resolve({ segments }) });
const runID = "e".repeat(32);

beforeEach(() => { setup(); fetchSpy = vi.fn(); vi.stubGlobal("fetch", fetchSpy); });
afterEach(() => { process.env = { ...originalEnv }; vi.unstubAllGlobals(); });

describe("authenticated Next.js ResearchRun BFF", () => {
  it("rejects unauthenticated requests before any backend call", async () => {
    const response = await GET(request("", { authenticated: false }), params());
    expect(response.status).toBe(401);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
  it("rejects traversal and unsupported backend routes", async () => {
    const invalid = await GET(request("/etc/passwd"), params(["etc", "passwd"]));
    const nested = await GET(request(`/${runID}/unknown`), params([runID, "unknown"]));
    expect(invalid.status).toBe(404);
    expect(nested.status).toBe(404);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
  it("forwards only server-held Authorization, never a client bearer", async () => {
    fetchSpy.mockResolvedValue(new Response('{"runs":[]}', { status: 200, headers: { "content-type": "application/json" } }));
    const req = request("");
    req.headers.set("Authorization", "Bearer injected-client-token");
    const response = await GET(req, params());
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ runs: [] });
    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, options] = fetchSpy.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8686/api/v1/research/runs");
    expect((options.headers as Headers).get("Authorization")).toBe("Bearer backend-access-secret-at-least-24-characters");
    expect((options.headers as Headers).get("Authorization")).not.toContain("injected-client-token");
    expect(response.headers.get("Cache-Control")).toBe("no-store");
  });
  it("rejects cross-origin mutation and oversized body before a provider call", async () => {
    const cross = request("", { method: "POST", body: '{"symbol":"600519.SH"}', originHeader: false });
    cross.headers.set("origin", "https://attacker.example");
    expect((await POST(cross, params())).status).toBe(403);
    const large = request("", { method: "POST", body: "x".repeat(16400) });
    expect((await POST(large, params())).status).toBe(413);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
  it("proxies an explicit NOT_PROVIDED account request without altering it", async () => {
    const payload = { symbol: "600519.SH", mode: "PORTFOLIO_REVIEW", portfolio: { state: "NOT_PROVIDED" } };
    fetchSpy.mockResolvedValue(new Response(JSON.stringify({ run_id: runID, state: "PENDING" }), { status: 202, headers: { "content-type": "application/json" } }));
    const response = await POST(request("", { method: "POST", body: JSON.stringify(payload) }), params());
    expect(response.status).toBe(202);
    const [, opts] = fetchSpy.mock.calls[0];
    expect(JSON.parse(opts.body)).toEqual(payload);
    expect(opts.redirect).toBe("manual");
  });
  it("streams ordered SSE without buffering and forwards numeric replay cursor", async () => {
    const sse = 'id: 7\nevent: research_stage\ndata: {"seq":7,"stage":"Event","status":"DONE","at":"2026-10-10T12:00:00Z"}\n\n';
    fetchSpy.mockResolvedValue(new Response(sse, { status: 200, headers: { "content-type": "text/event-stream" } }));
    const response = await GET(request(`/${runID}/stream`, { lastEventId: "6" }), params([runID, "stream"]));
    expect(response.headers.get("content-type")).toContain("text/event-stream");
    expect(await response.text()).toBe(sse);
    expect((fetchSpy.mock.calls[0][1].headers as Headers).get("Last-Event-ID")).toBe("6");
  });
  it("never follows unexpected Go redirects", async () => {
    fetchSpy.mockResolvedValue(new Response(null, { status: 302, headers: { location: "https://evil.example" } }));
    const response = await GET(request(""), params());
    expect(response.status).toBe(502);
  });
});
