import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET, PUT } from "./route";
import { COOKIE_NAME, liveConfig, newSession } from "@/lib/server/live-auth";

const host = "http://localhost:3000";
const prior = { ...process.env };
let spy: ReturnType<typeof vi.fn>;
const fakeStatus = { editable: true, configured: { DEEPSEEK_API_KEY: true, TUSHARE_TOKEN: false, TAVILY_API_KEY: false } };
function setup() {
  process.env.AEGIS_WEB_MODE = "live";
  process.env.AEGIS_WEB_API_URL = "http://127.0.0.1:8686";
  process.env.AEGIS_WEB_API_TOKEN = "backend-token-not-present-in-browser-session";
  process.env.AEGIS_WEB_ACCESS_PASSWORD = "frontend-access-password-long";
  process.env.AEGIS_WEB_SESSION_SECRET = "session-secret-not-present-in-browser-js";
  const config = liveConfig();if (!config) throw Error("invalid fixture setup");return config;
}
function req(method: "GET" | "PUT", authenticated = true, body?: string, origin = host) {
  const headers = new Headers();
  if (authenticated) headers.set("cookie", `${COOKIE_NAME}=${newSession(setup())}`);
  if (method === "PUT") { headers.set("origin", origin);headers.set("sec-fetch-site", "same-origin"); }
  return new NextRequest(`${host}/api/workbench/credentials`, { method, headers, body });
}
beforeEach(() => { setup();spy = vi.fn();vi.stubGlobal("fetch", spy); });
afterEach(() => { process.env = { ...prior };vi.unstubAllGlobals(); });
describe("provider credential BFF", () => {
  it("never calls Go API without an authenticated session", async () => {
    expect((await GET(req("GET", false))).status).toBe(401);
    expect((await PUT(req("PUT", false, "{}"))).status).toBe(401);
    expect(spy).not.toHaveBeenCalled();
  });
  it("blocks cross-site credential writes", async () => {
    expect((await PUT(req("PUT", true, "{}", "https://attacker.example"))).status).toBe(403);
    expect(spy).not.toHaveBeenCalled();
  });
  it("forwards only server-held bearer and masks all upstream values", async () => {
    spy.mockResolvedValue(new Response(JSON.stringify({ ...fakeStatus, secret: "must-not-leak" }), { status: 200 }));
    const response = await GET(req("GET"));
    expect(response.status).toBe(200);
    expect(JSON.stringify(await response.json())).not.toContain("must-not-leak");
    const [url, options] = spy.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8686/api/v1/settings/credentials");
    expect((options.headers.Authorization as string)).toContain("backend-token-not-present");
    expect(response.headers.get("cache-control")).toBe("no-store");
  });
  it("rejects oversized and invalid updates before contacting Go", async () => {
    expect((await PUT(req("PUT", true, "x".repeat(17000)))).status).toBe(413);
    expect((await PUT(req("PUT", true, "not-json"))).status).toBe(400);
    expect(spy).not.toHaveBeenCalled();
  });
  it("passes replacement secret to Go only, never echoes it to browser", async () => {
    const secret = "provider-secret-should-not-echo";
    spy.mockResolvedValue(new Response(JSON.stringify(fakeStatus), { status: 200 }));
    const response = await PUT(req("PUT", true, JSON.stringify({ changes: { DEEPSEEK_API_KEY: secret } })));
    expect(response.status).toBe(200);
    expect(JSON.stringify(await response.json())).not.toContain(secret);
    expect(spy.mock.calls[0][1].body).toContain(secret);
  });
});
