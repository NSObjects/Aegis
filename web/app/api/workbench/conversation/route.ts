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
  if (!payload || typeof payload !== "object" || Array.isArray(payload))
    return NextResponse.json({ error: "invalid conversation request" }, { status: 400, headers });
  const data = payload as Record<string, unknown>;
  const keys = Object.keys(data);
  if (keys.some(key => !["message", "start_symbol", "confirm_start", "session_id"].includes(key)) ||
      typeof data.message !== "string" || !data.message.trim() || data.message.length > 2048)
    return NextResponse.json({ error: "invalid conversation message" }, { status: 400, headers });
  if ("session_id" in data && (typeof data.session_id !== "string" || !/^[a-f0-9]{32}$/.test(data.session_id)))
    return NextResponse.json({ error: "invalid conversation session" }, { status: 400, headers });
  const authorized = "start_symbol" in data || "confirm_start" in data;
  if (authorized && (data.confirm_start !== true || typeof data.start_symbol !== "string" ||
      !/^(?:(?:600|601|603|605|688)[0-9]{3}\.SH|(?:000|001|002|003|300|301|302)[0-9]{3}\.SZ)$/.test(data.start_symbol)))
    return NextResponse.json({ error: "research start requires a confirmed A-share ticker" }, { status: 400, headers });
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
    if (authorized && (typeof (data as { run_id?: unknown }).run_id !== "string" || !/^[a-f0-9]{32}$/.test((data as { run_id: string }).run_id)))
      return NextResponse.json({ error: "backend did not create a valid research run" }, { status: 502, headers });
    return NextResponse.json(authorized ? { answer: (data as { answer: string }).answer, run_id: (data as { run_id: string }).run_id, symbol: (payload as { start_symbol: string }).start_symbol, session_id: typeof (data as { session_id?: unknown }).session_id === "string" && /^[a-f0-9]{32}$/.test((data as { session_id: string }).session_id) ? (data as { session_id: string }).session_id : undefined, state: (data as { state?: unknown }).state, session_binding: (data as { session_binding?: unknown }).session_binding } : { answer: (data as { answer: string }).answer }, { headers });
  } catch { return NextResponse.json({ error: "conversation unavailable" }, { status: 503, headers }); }
}
