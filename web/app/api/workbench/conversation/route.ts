import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, sameOriginMutation } from "@/lib/server/live-auth";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const headers = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

export async function POST(req: NextRequest) {
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live backend is not configured" }, { status: 503, headers });
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config))
    return NextResponse.json({ error: "authentication required" }, { status: 401, headers });
  if (!sameOriginMutation(req)) return NextResponse.json({ error: "invalid origin" }, { status: 403, headers });
  let payload: unknown;
  try {
    const raw = await req.text();
    if (raw.length > 4096) return NextResponse.json({ error: "conversation request too large" }, { status: 413, headers });
    payload = JSON.parse(raw);
  } catch { return NextResponse.json({ error: "invalid conversation request" }, { status: 400, headers }); }
  if (!payload || typeof payload !== "object" || Array.isArray(payload) ||
      Object.keys(payload).length !== 1 || typeof (payload as { message?: unknown }).message !== "string" ||
      !(payload as { message: string }).message.trim() ||
      (payload as { message: string }).message.length > 2048)
    return NextResponse.json({ error: "invalid conversation message" }, { status: 400, headers });
  try {
    const upstream = await fetch(`${config.baseURL}/api/v1/conversation/query`, {
      method: "POST",
      headers: { Authorization: `Bearer ${config.apiToken}`, "Content-Type": "application/json" },
      body: JSON.stringify(payload), cache: "no-store", redirect: "manual", signal: req.signal,
    });
    if (!upstream.ok) return NextResponse.json({ error: "conversation unavailable" }, { status: upstream.status >= 500 ? 503 : upstream.status, headers });
    const data: unknown = await upstream.json();
    if (!data || typeof data !== "object" || typeof (data as { answer?: unknown }).answer !== "string")
      return NextResponse.json({ error: "invalid backend conversation response" }, { status: 502, headers });
    return NextResponse.json({ answer: (data as { answer: string }).answer }, { headers });
  } catch { return NextResponse.json({ error: "conversation unavailable" }, { status: 503, headers }); }
}
