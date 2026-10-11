/**
 * The real Go-connected research workbench is the product default.
 * Synthetic UI is available only when explicitly opted into demo mode.
 * Never blend fixture charts or inferred market values into the Live UI.
 */
export function isDemoMode(mode: string | undefined): boolean {
  return mode === "demo";
}
