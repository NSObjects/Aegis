/**
 * Browser-location contract for the single-operator Live research console.
 * URLs contain only view and opaque Run ID; never credentials or report data.
 */
export type WorkbenchView = "workspace" | "history" | "compare" | "settings";

export type WorkbenchLocation = {
  view: WorkbenchView;
  runId: string | null;
};

const isRunId = (value: string | null): value is string =>
  typeof value === "string" && /^[a-f0-9]{32}$/.test(value);

export function readWorkbenchLocation(search: string): WorkbenchLocation {
  const params = new URLSearchParams(search);
  const requestedView = params.get("view");
  const view: WorkbenchView = requestedView === "history" || requestedView === "settings" || requestedView === "compare"
    ? requestedView : "workspace";
  return { view, runId: view === "workspace" && isRunId(params.get("run")) ? params.get("run") : null };
}

export function workbenchHref(pathname: string, view: WorkbenchView, runId: string | null = null): string {
  const params = new URLSearchParams();
  if (view !== "workspace") params.set("view", view);
  else if (isRunId(runId)) params.set("run", runId);
  const query = params.toString();
  return `${pathname || "/"}${query ? `?${query}` : ""}`;
}

/** A pair contains only validated opaque Run IDs, never research inputs or credentials. */
export type ComparisonLocation = { leftId: string | null; rightId: string | null };
export function readComparisonLocation(search: string): ComparisonLocation {
  const p = new URLSearchParams(search);
  if (p.get("view") !== "compare") return { leftId: null, rightId: null };
  return { leftId: isRunId(p.get("left")) ? p.get("left") : null,
    rightId: isRunId(p.get("right")) ? p.get("right") : null };
}
export function comparisonHref(pathname: string, leftId: string | null, rightId: string | null): string {
  const params = new URLSearchParams({ view: "compare" });
  if (isRunId(leftId)) params.set("left", leftId);
  if (isRunId(rightId)) params.set("right", rightId);
  return (pathname || "/") + "?" + params.toString();
}
