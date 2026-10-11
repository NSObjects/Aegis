import { describe, expect, it } from "vitest";
import { readWorkbenchLocation, workbenchHref } from "./workbench-location";

const id = "abcdef1234567890abcdef1234567890";

describe("Live workbench URL contract", () => {
  it("defaults to the workspace without a selected run", () => {
    expect(readWorkbenchLocation("")).toEqual({ view: "workspace", runId: null });
    expect(workbenchHref("/", "workspace")).toBe("/");
  });

  it("round trips an opaque run id, never embedding report data", () => {
    expect(readWorkbenchLocation(`?run=${id}`)).toEqual({ view: "workspace", runId: id });
    expect(workbenchHref("/", "workspace", id)).toBe(`/?run=${id}`);
  });

  it("selects history/settings explicitly and ignores stale run parameters", () => {
    expect(readWorkbenchLocation(`?view=history&run=${id}`)).toEqual({ view: "history", runId: null });
    expect(readWorkbenchLocation("?view=settings")).toEqual({ view: "settings", runId: null });
    expect(workbenchHref("/", "history", id)).toBe("/?view=history");
    expect(workbenchHref("/", "settings", id)).toBe("/?view=settings");
  });

  it("rejects malformed run identifiers rather than requesting arbitrary paths", () => {
    for (const value of ["../settings", "123", "F".repeat(32), "g".repeat(32)]) {
      expect(readWorkbenchLocation(`?run=${encodeURIComponent(value)}`).runId).toBeNull();
      expect(workbenchHref("/", "workspace", value)).toBe("/");
    }
    expect(readWorkbenchLocation("?view=unrecognized")).toEqual({ view: "workspace", runId: null });
  });
});
