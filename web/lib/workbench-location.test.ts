import { describe, expect, it } from "vitest";
import { readWorkbenchLocation, workbenchHref, readComparisonLocation, comparisonHref } from "./workbench-location";

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

  it("keeps existing root research entry while offering explicit overview deep links", () => {
    expect(readWorkbenchLocation("")).toEqual({view:"workspace",runId:null});
    expect(readWorkbenchLocation("?view=overview&run="+id)).toEqual({view:"overview",runId:null});
    expect(workbenchHref("/","overview")).toBe("/?view=overview");
    expect(workbenchHref("/","overview",id)).toBe("/?view=overview");
    expect(readComparisonLocation("?view=overview&left="+id)).toEqual({leftId:null,rightId:null});
  });

  it("round trips two opaque Run IDs for comparison and preserves absence safely", () => {
    const other = "f".repeat(32);
    expect(readWorkbenchLocation("?view=compare&left=" + id)).toEqual({ view: "compare", runId: null });
    expect(readComparisonLocation(comparisonHref("/", id, other).slice(1))).toEqual({
      leftId: id, rightId: other,
    });
    expect(comparisonHref("/", id, other)).toBe("/?view=compare&left=" + id + "&right=" + other);
    expect(readComparisonLocation("?view=compare&left=../evil&right=" + other)).toEqual({
      leftId: null, rightId: other,
    });
    expect(readComparisonLocation("?view=history&left=" + id)).toEqual({ leftId: null, rightId: null });
    expect(workbenchHref("/", "compare")).toBe("/?view=compare");
  });

  it("rejects malformed run identifiers rather than requesting arbitrary paths", () => {
    for (const value of ["../settings", "123", "F".repeat(32), "g".repeat(32)]) {
      expect(readWorkbenchLocation(`?run=${encodeURIComponent(value)}`).runId).toBeNull();
      expect(workbenchHref("/", "workspace", value)).toBe("/");
    }
    expect(readWorkbenchLocation("?view=unrecognized")).toEqual({ view: "workspace", runId: null });
  });
});
