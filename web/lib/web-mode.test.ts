import { describe, expect, it } from "vitest";
import { isDemoMode } from "./web-mode";

describe("Web entry mode", () => {
  it("shows Live workbench when the mode is missing or live", () => {
    expect(isDemoMode(undefined)).toBe(false);
    expect(isDemoMode("live")).toBe(false);
    expect(isDemoMode("")).toBe(false);
  });
  it("requires explicit demo opt-in so users cannot accidentally see old mock after updating", () => {
    expect(isDemoMode("demo")).toBe(true);
    expect(isDemoMode("unknown-mode")).toBe(false);
  });
});
