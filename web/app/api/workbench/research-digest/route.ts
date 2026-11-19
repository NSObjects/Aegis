import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig } from "@/lib/server/live-auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const responseHeaders = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };

function errorResponse(error: string, status: number) {
  return NextResponse.json({ error }, { status, headers: responseHeaders });
}

export async function GET(req: NextRequest) {
  const config = liveConfig();
  if (!config) return errorResponse("live backend is not configured", 503);
  if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value, config)) {
    return errorResponse("authentication required", 401);
  }
  const period = req.nextUrl.searchParams.get("period") ?? "daily";
  if (period !== "daily" && period !== "weekly") return errorResponse("invalid digest period", 400);
  const through = req.nextUrl.searchParams.get("through");
  if (through !== null && !/^\d{8}$/.test(through)) return errorResponse("invalid digest date", 400);

  const query = new URLSearchParams({ period });
  if (through !== null) query.set("through", through);
  try {
    const upstream = await fetch(`${config.baseURL}/api/v1/research-digest?${query}`, {
      headers: { Authorization: `Bearer ${config.apiToken}` },
      cache: "no-store",
      redirect: "manual",
      signal: req.signal,
    });
    if (upstream.status >= 300 && upstream.status < 400) {
      return errorResponse("unexpected backend redirect", 502);
    }
    if (!upstream.ok) return errorResponse("research digest unavailable", upstream.status);
    return NextResponse.json(await upstream.json(), { headers: responseHeaders });
  } catch {
    return errorResponse("research digest unavailable", 502);
  }
}
