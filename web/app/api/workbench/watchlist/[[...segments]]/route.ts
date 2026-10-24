import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, sameOriginMutation } from "@/lib/server/live-auth";
import { isAshareSymbol } from "@/lib/live-contract";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ segments?: string[] }> };
const prefix = "/api/v1/watchlist";
const defaultHeaders = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

// Only the three fixed operations are proxied; no arbitrary query strings or
// client headers reach the Go service.
function pathFor(segments: string[], method: "GET" | "POST" | "DELETE"): string | null {
  if (segments.length === 0) return method === "GET" || method === "POST" ? prefix : null;
  if (segments.length !== 1 || method !== "DELETE" || !isAshareSymbol(segments[0])) return null;
  return `${prefix}/${segments[0]}`;
}

async function proxy(req: NextRequest, ctx: Context, method: "GET" | "POST" | "DELETE") {
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live backend is not configured" }, { status: 503, headers: defaultHeaders });
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config)) return NextResponse.json({ error: "authentication required" }, { status: 401, headers: defaultHeaders });
  if (method !== "GET" && !sameOriginMutation(req)) return NextResponse.json({ error: "invalid origin" }, { status: 403, headers: defaultHeaders });
  const segments = (await ctx.params).segments ?? [];
  const path = pathFor(segments, method);
  if (!path) return NextResponse.json({ error: "route not found" }, { status: 404, headers: defaultHeaders });
  const headers = new Headers({ Authorization: `Bearer ${config.apiToken}` });
  let body: string | undefined;
  if (method === "POST") {
    const raw = await req.text();
    if (raw.length > 4096) return NextResponse.json({ error: "payload too large" }, { status: 413, headers: defaultHeaders });
    body = raw;
    headers.set("Content-Type", "application/json");
  }
  try {
    const upstream = await fetch(`${config.baseURL}${path}`, { method, body, headers, cache: "no-store", redirect: "manual", signal: req.signal });
    if (upstream.status >= 300 && upstream.status < 400) return NextResponse.json({ error: "unexpected backend redirect" }, { status: 502, headers: defaultHeaders });
    if (upstream.status === 204) return new Response(null, { status: 204, headers: defaultHeaders });
    if (!upstream.ok) return NextResponse.json({ error: `Watchlist service returned HTTP ${upstream.status}` }, { status: upstream.status, headers: defaultHeaders });
    const text = await upstream.text();
    return new Response(text, { status: upstream.status, headers: { ...defaultHeaders, "Content-Type": "application/json; charset=utf-8" } });
  } catch {
    return NextResponse.json({ error: "watchlist service unavailable" }, { status: 502, headers: defaultHeaders });
  }
}
export async function GET(req: NextRequest, ctx: Context) { return proxy(req, ctx, "GET"); }
export async function POST(req: NextRequest, ctx: Context) { return proxy(req, ctx, "POST"); }
export async function DELETE(req: NextRequest, ctx: Context) { return proxy(req, ctx, "DELETE"); }
