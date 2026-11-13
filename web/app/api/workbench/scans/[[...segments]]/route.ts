import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, sameOriginMutation } from "@/lib/server/live-auth";
import { isRunID } from "@/lib/live-contract";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type Context = { params: Promise<{ segments?: string[] }> };
const prefix = "/api/v1/scans";
const headers = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

function targetFor(parts: string[], method: "GET" | "POST"): string | null {
  if (parts.length === 0) return prefix;
  if (method !== "GET" || !isRunID(parts[0])) return null;
  if (parts.length === 1) return `${prefix}/${parts[0]}`;
  if (parts.length === 2 && ["comparison", "replay"].includes(parts[1])) return `${prefix}/${parts[0]}/${parts[1]}`;
  if (parts.length === 4 && parts[1] === "results" && parts[3] === "snapshot" &&
      /^[0-9]{6}\.(SH|SZ)$/.test(parts[2])) return `${prefix}/${parts[0]}/results/${parts[2]}/snapshot`;
  return null;
}

async function proxy(req: NextRequest, ctx: Context, method: "GET" | "POST") {
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live backend is not configured" }, { status: 503, headers });
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config))
    return NextResponse.json({ error: "authentication required" }, { status: 401, headers });
  if (method === "POST" && !sameOriginMutation(req))
    return NextResponse.json({ error: "invalid origin" }, { status: 403, headers });

  const parts = (await ctx.params).segments ?? [];
  const target = targetFor(parts, method);
  if (!target || (method === "POST" && parts.length !== 0))
    return NextResponse.json({ error: "route not found" }, { status: 404, headers });
  const upstreamHeaders = new Headers({ Authorization: `Bearer ${config.apiToken}` });
  let body: string | undefined;
  let url = target;

  if (method === "POST") {
    const raw = await req.text();
    if (raw.length > 2048) return NextResponse.json({ error: "payload too large" }, { status: 413, headers });
    let input: unknown;
    try { input = JSON.parse(raw); } catch { /* reject below */ }
    const v = input && typeof input === "object" && !Array.isArray(input) ? input as Record<string, unknown> : null;
    if (!v || Object.keys(v).some(k => !["as_of", "universe_source", "symbols"].includes(k)) ||
        v.universe_source !== "USER_SUPPLIED" || typeof v.as_of !== "string" || !/^\d{8}$/.test(v.as_of) ||
        !Array.isArray(v.symbols) || v.symbols.length < 1 || v.symbols.length > 20 ||
        v.symbols.some(s => typeof s !== "string" || !/^[0-9]{6}\.(SH|SZ)$/.test(s)) ||
        new Set(v.symbols).size !== v.symbols.length)
      return NextResponse.json({ error: "invalid scan inputs" }, { status: 400, headers });
    body = JSON.stringify(v);
    upstreamHeaders.set("Content-Type", "application/json");
  } else if (parts.length === 0) {
    const entries = [...req.nextUrl.searchParams.entries()];
    if (entries.some(([k]) => !["limit","universe_source"].includes(k)) ||
        new Set(entries.map(([key])=>key)).size !== entries.length)
      return NextResponse.json({ error: "unsupported scan filter" }, { status: 400, headers });
    const source = req.nextUrl.searchParams.get("universe_source");
    if (source !== null && source !== "USER_SUPPLIED")
      return NextResponse.json({ error: "invalid scan source" }, { status: 400, headers });
    const limit = req.nextUrl.searchParams.get("limit");
    if (limit !== null) {
      if (!/^[1-9][0-9]?$|^100$/.test(limit))
        return NextResponse.json({ error: "invalid scan history limit" }, { status: 400, headers });
      url += `?limit=${limit}`;
    }
    if (source !== null) url += (limit === null ? "?" : "&") + "universe_source=USER_SUPPLIED";
  } else if (req.nextUrl.search)
    return NextResponse.json({ error: "unsupported scan filter" }, { status: 400, headers });

  try {
    const upstream = await fetch(`${config.baseURL}${url}`, {
      method, headers: upstreamHeaders, body, cache: "no-store", redirect: "manual", signal: req.signal,
    });
    if (upstream.status >= 300 && upstream.status < 400)
      return NextResponse.json({ error: "unexpected backend redirect" }, { status: 502, headers });
    if (!upstream.ok)
      return NextResponse.json({ error: `Scan service returned HTTP ${upstream.status}` }, { status: upstream.status, headers });
    const contentType = upstream.headers.get("Content-Type") ?? "";
    if (!contentType.includes("application/json"))
      return NextResponse.json({ error: "invalid scan service response" }, { status: 502, headers });
    const response = await upstream.text();
    return new Response(response, { status: upstream.status,
      headers: { ...headers, "Content-Type": "application/json; charset=utf-8" } });
  } catch {
    return NextResponse.json({ error: "scan service unavailable" }, { status: 502, headers });
  }
}

export async function GET(req: NextRequest, ctx: Context) { return proxy(req, ctx, "GET"); }
export async function POST(req: NextRequest, ctx: Context) { return proxy(req, ctx, "POST"); }
