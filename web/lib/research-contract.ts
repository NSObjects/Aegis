/** Proposed Web-facing DTOs. These are NOT claims about the current Go API. */
export type ResearchStatus = "PENDING" | "RUNNING" | "COMPLETED" | "FAILED" | "CANCELLED";
export type StageStatus = "PENDING" | "RUNNING" | "COMPLETED" | "FAILED" | "SKIPPED";

export interface ResearchRun {
  run_id: string;
  symbol: string;
  requested_as_of: string;
  status: ResearchStatus;
  started_at?: string;
  finished_at?: string;
  failure_stage?: string;
  failure_reason?: string;
}

export interface RunEvent {
  event_id: number;
  run_id: string;
  agent: string;
  status: StageStatus;
  timestamp: string;
  detail?: string;
}

/** Backend-owned JSON artifacts remain opaque to the UI until contracts are reviewed. */
export interface ResearchReport {
  run_id: string;
  research_artifact: unknown;
  portfolio_review?: unknown;
  final_risk_gate?: unknown;
}

export interface StartResearchInput {
  symbol: string;
  requested_as_of?: string;
}

export interface ResearchGateway {
  startResearch(input: StartResearchInput): Promise<ResearchRun>;
  getResearchRun(runId: string): Promise<ResearchRun>;
  listResearchRuns(): Promise<ResearchRun[]>;
  getResearchReport(runId: string): Promise<ResearchReport | null>;
  subscribeRunEvents(runId: string, onEvent: (event: RunEvent) => void): () => void;
}
