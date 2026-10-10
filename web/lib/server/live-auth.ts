import { createHmac, randomBytes, createHash, timingSafeEqual } from "node:crypto";

export const COOKIE_NAME = "aegis_live_session";
const SESSION_AGE_MS = 6 * 60 * 60 * 1000;

export function liveConfig() {
  if (process.env.AEGIS_WEB_MODE !== "live") return null;
  const upstream = process.env.AEGIS_WEB_API_URL ?? "http://127.0.0.1:8686";
  const apiToken = process.env.AEGIS_WEB_API_TOKEN ?? "";
  const password = process.env.AEGIS_WEB_ACCESS_PASSWORD ?? "";
  const sessionSecret = process.env.AEGIS_WEB_SESSION_SECRET ?? "";
  const mode = process.env.AEGIS_WEB_RESEARCH_MODE === "DECISION" ? "DECISION" as const : "PORTFOLIO_REVIEW" as const;
  try {
    const parsed = new URL(upstream);
    if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password || parsed.search || parsed.hash || parsed.pathname !== "/") return null;
    const localDev = process.env.AEGIS_WEB_DEV_MODE === "1" &&
      process.env.NODE_ENV === "development" && parsed.hostname === "127.0.0.1" &&
      parsed.protocol === "http:";
    if (!localDev && (apiToken.length < 24 || password.length < 16 || sessionSecret.length < 32)) return null;
    return { baseURL: parsed.origin, apiToken, password, sessionSecret, mode, localDev };
  } catch { return null; }
}

type Config = NonNullable<ReturnType<typeof liveConfig>>;
function sign(payload: string, secret: string) { return createHmac("sha256", secret).update(payload).digest("base64url"); }
export function newSession(config: Config, now = Date.now()) {
  const payload = `${now}.${randomBytes(20).toString("base64url")}`;
  return `${payload}.${sign(payload, config.sessionSecret)}`;
}
export function isSessionValid(cookie: string | undefined, config: Config, now = Date.now()): boolean {
  if (config.localDev) return true;
  if (!cookie || cookie.length > 300) return false;
  const parts = cookie.split(".");
  if (parts.length !== 3 || !/^\d{13}$/.test(parts[0]) || !/^[a-zA-Z0-9_-]{27}$/.test(parts[1]) || !/^[a-zA-Z0-9_-]{43}$/.test(parts[2])) return false;
  const issued = Number(parts[0]);
  if (!Number.isFinite(issued) || issued > now || now - issued >= SESSION_AGE_MS) return false;
  const received = Buffer.from(parts[2]);
  const expected = Buffer.from(sign(`${parts[0]}.${parts[1]}`, config.sessionSecret));
  return received.length === expected.length && timingSafeEqual(received, expected);
}
export function matchesPassword(candidate: unknown, config: Config): boolean {
  if (typeof candidate !== "string" || candidate.length > 512) return false;
  const a = createHash("sha256").update(candidate).digest();
  const b = createHash("sha256").update(config.password).digest();
  return timingSafeEqual(a, b);
}
export function sameOriginMutation(req: Request): boolean {
  const site = req.headers.get("sec-fetch-site");
  const dev = process.env.AEGIS_WEB_DEV_MODE === "1" && process.env.NODE_ENV === "development";
  if (site && site !== "same-origin" && site !== "none" && !(dev && site === "same-site")) return false;
  const origin = req.headers.get("origin");
  if (!origin) return false;
  const target = new URL(req.url);
  if (origin === target.origin) return true;
  // Some Next.js dev versions build req.url with localhost even when the
  // actual browser and Host header use 127.0.0.1. Only permit the exact
  // browser Host/Origin pair when both sides are literal loopback hosts.
  if (!dev) return false;
  try {
    const submitted = new URL(origin);
    const host = req.headers.get("host");
    return target.protocol === "http:" && submitted.protocol === "http:" &&
      ["localhost", "127.0.0.1"].includes(target.hostname) &&
      ["localhost", "127.0.0.1"].includes(submitted.hostname) &&
      target.port === submitted.port && submitted.host === host;
  } catch { return false; }
}
export const SESSION_MAX_AGE = SESSION_AGE_MS / 1000;
