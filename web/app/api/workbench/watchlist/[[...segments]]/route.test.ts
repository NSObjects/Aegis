import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { DELETE, GET, POST } from "./route";
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
function request(path: string, { method = "GET", authenticated = true, originHeader = true, body }: { method?: "GET" | "POST" | "DELETE"; authenticated?: boolean; originHeader?: boolean; body?: string } = {}) {
  const headers = new Headers();
  if (authenticated) headers.set("cookie", `${COOKIE_NAME}=${newSession(setup())}`);
  if (method !== "GET" && originHeader) {
    headers.set("origin", origin);
    headers.set("sec-fetch-site", "same-origin");
  }
  if (body) headers.set("content-type", "application/json");
  return new NextRequest(`${origin}/api/workbench/watchlist${path}`, { method, headers, body });
}
const params = (segments?: string[]) => ({ params: Promise.resolve({ segments }) });
const jsonResponse = (status: number, value: unknown) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });

beforeEach(() => { setup(); fetchSpy = vi.fn(); vi.stubGlobal("fetch", fetchSpy); });
afterEach(() => { process.env = { ...originalEnv }; vi.unstubAllGlobals(); });

describe("authenticated watchlist BFF proxy", () => {
  it("rejects unauthenticated requests before any backend call", async () => {
    const response = await GET(request("", { authenticated: false }), params());
    expect(response.status).toBe(401);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
  it("rejects cross-origin mutations and unsupported routes", async () => {
    expect((await POST(request("", { originHeader: false }), params())).status).toBe(403);
    expect((await DELETE(request("", { method: "DELETE" }), params())).status).toBe(404);
    expect((await GET(request("/600519"), params(["600519"]))).status).toBe(404);
    expect((await DELETE(request("/../etc/passwd", { method: "DELETE" }), params(["..", "etc", "passwd"]))).status).toBe(404);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
  it("proxies list, add and remove with only the fixed operations", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(200, { items: [] }));
    const list = await GET(request(""), params());
    expect(list.status).toBe(200);
    expect(fetchSpy).toHaveBeenCalledWith("http://127.0.0.1:8686/api/v1/watchlist", expect.objectContaining({ method: "GET" }));

    fetchSpy.mockResolvedValueOnce(jsonResponse(201, { symbol: "600519.SH" }));
    const add = await POST(request("", { method: "POST", body: JSON.stringify({ symbol: "600519.SH", reason: "r" }) }), params());
    expect(add.status).toBe(201);
    const [, addInit] = fetchSpy.mock.calls[1];
    expect(addInit.method).toBe("POST");
    expect(addInit.headers.get("Authorization")).toBe("Bearer backend-access-secret-at-least-24-characters");

    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }));
    const remove = await DELETE(request("/600519.SH", { method: "DELETE" }), params(["600519.SH"]));
    expect(remove.status).toBe(204);
    expect(fetchSpy).toHaveBeenLastCalledWith("http://127.0.0.1:8686/api/v1/watchlist/600519.SH", expect.objectContaining({ method: "DELETE" }));
  });
  it("caps mutation payloads and surfaces backend failures without internals", async () => {
    const oversized = await POST(request("", { method: "POST", body: "x".repeat(4097) }), params());
    expect(oversized.status).toBe(413);
    expect(fetchSpy).not.toHaveBeenCalled();

    fetchSpy.mockResolvedValueOnce(jsonResponse(409, { error: "watchlist symbol already present" }));
    const conflict = await POST(request("", { method: "POST", body: JSON.stringify({ symbol: "600519.SH", reason: "r" }) }), params());
    expect(conflict.status).toBe(409);

    fetchSpy.mockRejectedValueOnce(new Error("socket hang up"));
    const unavailable = await GET(request(""), params());
    expect(unavailable.status).toBe(502);
  });
});
