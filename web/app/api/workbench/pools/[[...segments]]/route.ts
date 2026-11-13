import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, sameOriginMutation } from "@/lib/server/live-auth";
import { isRunID } from "@/lib/live-contract";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
type Context = { params: Promise<{ segments?: string[] }> };
type Method = "GET" | "POST";
const prefix = "/api/v1/pools";
const headers = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

function pathFor(parts: string[], method: Method): string | null {
  if (parts.length === 0) return prefix;
  if (!isRunID(parts[0])) return null;
  if (parts.length === 1 && method === "GET") return `${prefix}/${parts[0]}`;
  if (parts.length === 2) {
    if (method === "GET" && parts[1] === "comparison") return `${prefix}/${parts[0]}/comparison`;
    if (method === "POST" && ["resume", "pause"].includes(parts[1])) return `${prefix}/${parts[0]}/${parts[1]}`;
  }
  return null;
}

async function proxy(req: NextRequest, ctx: Context, method: Method) {
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live backend is not configured" }, { status: 503, headers });
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config))
    return NextResponse.json({ error: "authentication required" }, { status: 401, headers });
  if (method === "POST" && !sameOriginMutation(req))
    return NextResponse.json({ error: "invalid origin" }, { status: 403, headers });

  const parts = (await ctx.params).segments ?? [];
  const path = pathFor(parts, method);
  if (!path) return NextResponse.json({ error: "route not found" }, { status: 404, headers });
  const upstreamHeaders = new Headers({ Authorization: `Bearer ${config.apiToken}` });
  let target = path;
  let body: string | undefined;

  if (method === "POST" && parts.length === 0) {
    const raw = await req.text();
    if (raw.length > 1024) return NextResponse.json({ error: "payload too large" }, { status: 413, headers });
    let value: unknown;
    try { value = JSON.parse(raw); } catch { /* rejected below */ }
    const input = value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
    if (!input || Object.keys(input).length !== 1 ||
        typeof input.as_of !== "string" || !/^\d{8}$/.test(input.as_of))
      return NextResponse.json({ error: "invalid pool request" }, { status: 400, headers });
    body = JSON.stringify(input);
    upstreamHeaders.set("Content-Type", "application/json");
  } else if (method === "GET" && (parts.length === 0 || parts[1] === "comparison")) {
    const allowed = parts.length === 0 ? ["limit"] : ["limit", "offset"];
    const entries = [...req.nextUrl.searchParams.entries()];
    if (entries.some(([name]) => !allowed.includes(name)) || new Set(entries.map(([name])=>name)).size !== entries.length)
      return NextResponse.json({ error: "invalid pool query" }, { status: 400, headers });
    for (const [key, value] of entries) {
      if (key === "limit" && (!/^[1-9][0-9]?$|^100$/.test(value)))
        return NextResponse.json({ error: "invalid limit" }, { status: 400, headers });
      if (key === "offset" && (!/^\d{1,4}$/.test(value) || Number(value) > 4500))
        return NextResponse.json({ error: "invalid offset" }, { status: 400, headers });
    }
    if (entries.length) target += "?" + new URLSearchParams(entries).toString();
  } else if (req.nextUrl.search) {
    return NextResponse.json({ error: "unsupported query" }, { status: 400, headers });
  }

  try {
    const upstream = await fetch(`${config.baseURL}${target}`, {
      method, headers: upstreamHeaders, body, cache: "no-store", redirect: "manual", signal: req.signal,
    });
    if (upstream.status >= 300 && upstream.status < 400)
      return NextResponse.json({ error: "unexpected backend redirect" }, { status: 502, headers });
    if (!upstream.ok)
      return NextResponse.json({ error: `Pool service returned HTTP ${upstream.status}` }, { status: upstream.status, headers });
    if (!(upstream.headers.get("Content-Type") ?? "").includes("application/json"))
      return NextResponse.json({ error: "invalid pool response" }, { status: 502, headers });
    return new Response(await upstream.text(), { status: upstream.status,
      headers: { ...headers, "Content-Type": "application/json; charset=utf-8" } });
  } catch {
    return NextResponse.json({ error: "pool service unavailable" }, { status: 502, headers });
  }
}
export async function GET(req: NextRequest, ctx: Context) { return proxy(req, ctx, "GET"); }
export async function POST(req: NextRequest, ctx: Context) { return proxy(req, ctx, "POST"); }
