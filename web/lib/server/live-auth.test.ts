import { afterEach, describe, expect, it } from "vitest";
import { isSessionValid, liveConfig, matchesPassword, newSession, sameOriginMutation } from "./live-auth";
const env = { ...process.env };
afterEach(() => { process.env = { ...env }; });
function config() {
  process.env.AEGIS_WEB_MODE = "live";
  process.env.AEGIS_WEB_API_URL = "http://127.0.0.1:8686";
  process.env.AEGIS_WEB_API_TOKEN = "long-backend-token-secret-that-is-private";
  process.env.AEGIS_WEB_ACCESS_PASSWORD = "not-guessable-test-password";
  process.env.AEGIS_WEB_SESSION_SECRET = "a-separate-random-long-secret-at-least-32-characters";
  const cfg = liveConfig();
  if (!cfg) throw Error("test config unavailable");
  return cfg;
}
describe("single-operator server-side session", () => {
  it("requires explicit secrets and does not enable auth in demo", () => {
    delete process.env.AEGIS_WEB_MODE;
    expect(liveConfig()).toBeNull();
    config(); process.env.AEGIS_WEB_API_TOKEN = "short";
    expect(liveConfig()).toBeNull();
  });
  it("denies invalid/mismatched/expired sessions, constant time password check", () => {
    const cfg = config();
    expect(matchesPassword("not-guessable-test-password", cfg)).toBe(true);
    expect(matchesPassword("other", cfg)).toBe(false);
    const value = newSession(cfg, 1000000000000);
    expect(isSessionValid(value, cfg, 1000000000000 + 100)).toBe(true);
    expect(isSessionValid(value, cfg, 1000000000000 + 6*60*60*1000)).toBe(false);
    expect(isSessionValid(value.slice(0, -2) + "xx", cfg, 1000000000000 + 100)).toBe(false);
    expect(isSessionValid(value, { ...cfg, sessionSecret: "other-secret-that-is-longer-than-32" }, 1000000000000+100)).toBe(false);
  });
  it("checks browser mutation origin and fetch-site", () => {
    const url = "http://localhost:3000/api/workbench/session";
    expect(sameOriginMutation(new Request(url, { method: "POST", headers: { origin: "http://localhost:3000", "sec-fetch-site": "same-origin" } }))).toBe(true);
    expect(sameOriginMutation(new Request(url, { method: "POST", headers: { origin: "https://evil.example", "sec-fetch-site": "cross-site" } }))).toBe(false);
    expect(sameOriginMutation(new Request(url, { method: "POST" }))).toBe(false);
  });
});

describe("loopback-only local debug mode", () => {
  it("permits session-free local access only in opt-in development", () => {
    config();
    process.env.AEGIS_WEB_DEV_MODE = "1";
    process.env.NODE_ENV = "development";
    delete process.env.AEGIS_WEB_API_TOKEN;
    delete process.env.AEGIS_WEB_ACCESS_PASSWORD;
    delete process.env.AEGIS_WEB_SESSION_SECRET;
    const local = liveConfig();
    expect(local?.localDev).toBe(true);
    if (!local) throw Error("missing dev config");
    expect(isSessionValid(undefined, local)).toBe(true);
    process.env.NODE_ENV = "production";
    expect(liveConfig()).toBeNull();
    process.env.NODE_ENV = "development";
    process.env.AEGIS_WEB_API_URL = "http://192.168.1.5:8686";
    expect(liveConfig()).toBeNull();
    process.env.AEGIS_WEB_API_URL = "http://example.com:8686";
    expect(liveConfig()).toBeNull();
  });
});
