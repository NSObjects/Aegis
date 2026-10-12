import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, sameOriginMutation } from "@/lib/server/live-auth";
import { isRunID } from "@/lib/live-contract";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ segments?: string[] }> };
const prefix = "/api/v1/research/runs";
const defaultHeaders = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

function pathFor(segments: string[], method: string): string | null {
  if (segments.length === 0) return method === "GET" || method === "POST" ? prefix : null;
  if (!isRunID(segments[0])) return null;
  if (segments.length === 1) return method === "GET" ? `${prefix}/${segments[0]}` : null;
  if (segments.length !== 2) return null;
  if (segments[1] === "cancel" && method === "POST") return `${prefix}/${segments[0]}/cancel`;
  if (["artifact", "events", "stream"].includes(segments[1]) && method === "GET") return `${prefix}/${segments[0]}/${segments[1]}`;
  return null;
}

async function proxy(req: NextRequest, ctx: Context, method: "GET" | "POST") {
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live backend is not configured" }, { status: 503, headers: defaultHeaders });
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config)) return NextResponse.json({ error: "authentication required" }, { status: 401, headers: defaultHeaders });
  if (method === "POST" && !sameOriginMutation(req)) return NextResponse.json({ error: "invalid origin" }, { status: 403, headers: defaultHeaders });
  const segments = (await ctx.params).segments ?? [];
  const path = pathFor(segments, method);
  if (!path) return NextResponse.json({ error: "route not found" }, { status: 404, headers: defaultHeaders });
  const headers = new Headers({ Authorization: `Bearer ${config.apiToken}` });
  let body: string | undefined;
  if (method === "POST" && segments.length === 0) {
    const raw = await req.text();
    if (raw.length > 16384) return NextResponse.json({ error: "payload too large" }, { status: 413, headers: defaultHeaders });
    body = raw;
    headers.set("Content-Type", "application/json");
    // Forward only this allowlisted request-deduplication header, never arbitrary client headers.
    const key = req.headers.get("Idempotency-Key");
    if (key !== null) {
      if (!/^[a-zA-Z0-9_.:-]{8,128}$/.test(key)) return NextResponse.json({ error: "invalid Idempotency-Key" }, { status: 400, headers: defaultHeaders });
      headers.set("Idempotency-Key", key);
    }
  }
  if (path.endsWith("/stream")) {
    const lastID = req.headers.get("last-event-id");
    if (lastID && /^\d{1,20}$/.test(lastID)) headers.set("Last-Event-ID", lastID);
  }
  try {
    // Forward only bounded history filters on the list route; never arbitrary query parameters.
    let target = path;
    if (method === "GET" && segments.length === 0) {
      const params = new URLSearchParams();
      for (const key of ["limit", "offset", "symbol", "state"]) {
        const values = req.nextUrl.searchParams.getAll(key);
        if (values.length > 1) return NextResponse.json({ error: "duplicate history filter" }, { status: 400, headers: defaultHeaders });
        if (values.length === 1) params.set(key, values[0]);
      }
      if ([...req.nextUrl.searchParams.keys()].some(key => !["limit", "offset", "symbol", "state"].includes(key))) {
        return NextResponse.json({ error: "unsupported history filter" }, { status: 400, headers: defaultHeaders });
      }
      if (params.size > 0) target += "?" + params.toString();
    }
    const upstream = await fetch(`${config.baseURL}${target}`, { method, body, headers, cache: "no-store", redirect: "manual", signal: req.signal });
    if (upstream.status >= 300 && upstream.status < 400) return NextResponse.json({ error: "unexpected backend redirect" }, { status: 502, headers: defaultHeaders });
    const contentType = upstream.headers.get("Content-Type") ?? "";
    if (path.endsWith("/stream") && upstream.ok && contentType.includes("text/event-stream") && upstream.body) {
      return new Response(upstream.body, { status: upstream.status, headers: { ...defaultHeaders, "Content-Type": "text/event-stream; charset=utf-8", "X-Accel-Buffering": "no" } });
    }
    if (path.endsWith("/stream") && upstream.ok) return NextResponse.json({ error: "invalid backend event stream" }, { status: 502, headers: defaultHeaders });
    // Go errors have sanitized text; preserve status without leaking internal headers.
    if (!upstream.ok) return NextResponse.json({ error: `Research service returned HTTP ${upstream.status}` }, { status: upstream.status, headers: defaultHeaders });
    const text = await upstream.text();
    return new Response(text, { status: upstream.status, headers: { ...defaultHeaders, "Content-Type": "application/json; charset=utf-8" } });
  } catch {
    return NextResponse.json({ error: "research service unavailable" }, { status: 502, headers: defaultHeaders });
  }
}
export async function GET(req: NextRequest, ctx: Context) { return proxy(req, ctx, "GET"); }
export async function POST(req: NextRequest, ctx: Context) { return proxy(req, ctx, "POST"); }
