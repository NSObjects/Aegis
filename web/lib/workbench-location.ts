/**
 * Browser-location contract for the single-operator Live research console.
 * URLs contain only view and opaque Run ID; never credentials or report data.
 */
export type WorkbenchView = "workspace" | "history" | "settings";

export type WorkbenchLocation = {
  view: WorkbenchView;
  runId: string | null;
};

const isRunId = (value: string | null): value is string =>
  typeof value === "string" && /^[a-f0-9]{32}$/.test(value);

export function readWorkbenchLocation(search: string): WorkbenchLocation {
  const params = new URLSearchParams(search);
  const requestedView = params.get("view");
  const view: WorkbenchView = requestedView === "history" || requestedView === "settings"
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
