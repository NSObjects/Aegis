import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, sameOriginMutation } from "@/lib/server/live-auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const headers = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

async function credentials(req: NextRequest, method: "GET" | "PUT") {
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live mode unavailable" }, { status: 503, headers });
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config))
    return NextResponse.json({ error: "authentication required" }, { status: 401, headers });
  if (method === "PUT" && !sameOriginMutation(req))
    return NextResponse.json({ error: "invalid origin" }, { status: 403, headers });
  let body: string | undefined;
  if (method === "PUT") {
    body = await req.text();
    if (body.length > 16384) return NextResponse.json({ error: "request too large" }, { status: 413, headers });
    try {
      const parsed: unknown = JSON.parse(body);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw Error("invalid");
    } catch { return NextResponse.json({ error: "invalid JSON" }, { status: 400, headers }); }
  }
  try {
    const upstream = await fetch(`${config.baseURL}/api/v1/settings/credentials`, {
      method,
      headers: { Authorization: `Bearer ${config.apiToken}`, ...(method === "PUT" ? { "Content-Type": "application/json" } : {}) },
      ...(body !== undefined ? { body } : {}),
      redirect: "manual",
      cache: "no-store",
      signal: req.signal,
    });
    if (upstream.status >= 300 && upstream.status < 400)
      return NextResponse.json({ error: "unexpected redirect" }, { status: 502, headers });
    if (!upstream.ok)
      return NextResponse.json({ error: `credential settings HTTP ${upstream.status}` }, { status: upstream.status, headers });
    // Never forward arbitrary upstream response headers or raw values.
    const data: unknown = await upstream.json();
    if (!data || typeof data !== "object" || Array.isArray(data)) throw Error("invalid response");
    const obj = data as Record<string, unknown>;
    const configured = obj.configured;
    if (!configured || typeof configured !== "object" || Array.isArray(configured) || typeof obj.editable !== "boolean")
      throw Error("invalid credential status");
    const mask = configured as Record<string, unknown>;
    const names = ["DEEPSEEK_API_KEY", "TUSHARE_TOKEN", "TAVILY_API_KEY"] as const;
    if (names.some(name => typeof mask[name] !== "boolean")) throw Error("invalid credential status");
    return NextResponse.json({ editable: obj.editable, configured: Object.fromEntries(names.map(name => [name, mask[name]])) }, { headers });
  } catch {
    return NextResponse.json({ error: "credential settings unavailable" }, { status: 502, headers });
  }
}
export function GET(req: NextRequest) { return credentials(req, "GET"); }
export function PUT(req: NextRequest) { return credentials(req, "PUT"); }
