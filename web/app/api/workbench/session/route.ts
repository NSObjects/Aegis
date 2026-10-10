import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig, matchesPassword, newSession, sameOriginMutation, SESSION_MAX_AGE } from "@/lib/server/live-auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const noCache = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };
export function GET(req: NextRequest) {
  const config = liveConfig();
  return NextResponse.json({ configured: !!config, authenticated: !!config && isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config), mode: config?.mode ?? "PORTFOLIO_REVIEW", local_dev: !!config?.localDev }, { headers: noCache });
}
export async function POST(req: NextRequest) {
  if (!sameOriginMutation(req)) return NextResponse.json({ error: "invalid origin" }, { status: 403, headers: noCache });
  const config = liveConfig();
  if (!config) return NextResponse.json({ error: "live mode not configured" }, { status: 503, headers: noCache });
  if (config.localDev) return NextResponse.json({ authenticated: true, configured: true, mode: config.mode, local_dev: true }, { headers: noCache });
  let body: unknown;
  try { const raw = await req.text(); if (raw.length > 1024) throw Error("oversize"); body = JSON.parse(raw); } catch { return NextResponse.json({ error: "invalid login" }, { status: 400, headers: noCache }); }
  if (!body || typeof body !== "object" || !matchesPassword((body as { password?: unknown }).password, config))
    return NextResponse.json({ error: "invalid credentials" }, { status: 401, headers: noCache });
  const response = NextResponse.json({ authenticated: true, configured: true, mode: config.mode }, { headers: noCache });
  response.cookies.set(COOKIE_NAME, newSession(config), { httpOnly: true, sameSite: "strict", secure: process.env.NODE_ENV === "production", path: "/", maxAge: SESSION_MAX_AGE });
  return response;
}
export function DELETE(req: NextRequest) {
  if (!sameOriginMutation(req)) return NextResponse.json({ error: "invalid origin" }, { status: 403, headers: noCache });
  const response = NextResponse.json({ authenticated: false }, { headers: noCache });
  response.cookies.set(COOKIE_NAME, "", { httpOnly: true, sameSite: "strict", secure: process.env.NODE_ENV === "production", path: "/", maxAge: 0 });
  return response;
}
