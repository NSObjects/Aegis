/**
 * Read-only, explicit-ID projection of Go EvidenceLedger and EventContext.
 * A matched Event ID is not verification of any underlying document or URL.
 * TradeIntent.Evidence is free text and is never used as a structured link.
 */
export interface EvidenceDocument {
  id: string;
}
export interface EvidenceEvent {
  id: string;
  headline: string;
  summary: string;
  type: string | null;
  occurredAt: string | null;
  sourceCount: number | null;
  documents: EvidenceDocument[];
}
export interface EvidenceLink {
  domain: string;
  eventId: string | null;
  status: "DOMAIN_ONLY" | "MATCHED_EVENT" | "EVENT_NOT_FOUND" | "INVALID_REFERENCE";
}
export interface EvidenceChallenge {
  side: string | null;
  round: number | null;
  claim: string;
  references: EvidenceLink[];
}
export interface EvidenceArgument {
  id: string;
  side: string | null;
  round: number | null;
  claim: string;
  disposition: string | null;
  references: EvidenceLink[];
  challenges: EvidenceChallenge[];
}
export interface EvidenceInsight {
  kind: string | null;
  claim: string;
  argumentIds: string[];
  references: EvidenceLink[];
}
export interface EvidenceNavigatorModel {
  events: EvidenceEvent[];
  arguments: EvidenceArgument[];
  insights: EvidenceInsight[];
}

type JsonObject = Record<string, unknown>;
const object = (input: unknown): JsonObject | null =>
  input !== null && typeof input === "object" && !Array.isArray(input) ? input as JsonObject : null;
const items = (input: unknown): unknown[] => Array.isArray(input) ? input : [];
const string = (input: unknown): string | null =>
  typeof input === "string" && input.trim() !== "" ? input : null;
const number = (input: unknown): number | null =>
  typeof input === "number" && Number.isFinite(input) ? input : null;
const strings = (input: unknown): string[] =>
  items(input).map(string).filter((item): item is string => item !== null);

function links(input: unknown, eventIDs: Set<string>): EvidenceLink[] {
  return items(input).map(item => {
    const ref = object(item);
    const domain = string(ref?.domain);
    const eventId = string(ref?.event_id);
    if (!domain) return { domain: "UNKNOWN", eventId: null, status: "INVALID_REFERENCE" as const };
    if (!eventId) return { domain, eventId: null, status: "DOMAIN_ONLY" as const };
    // A source ID is meaningful only for the EVENT reference domain.
    if (domain !== "EVENT") return { domain, eventId, status: "INVALID_REFERENCE" as const };
    return { domain, eventId, status: eventIDs.has(eventId) ? "MATCHED_EVENT" as const : "EVENT_NOT_FOUND" as const };
  });
}

export function projectEvidenceNavigator(input: unknown): EvidenceNavigatorModel {
  const empty: EvidenceNavigatorModel = { events: [], arguments: [], insights: [] };
  const root = object(input);
  const research = object(root?.research_artifact);
  const review = object(root?.portfolio_review);
  const ledger = object(review?.evidence_ledger);
  if (!research || !ledger) return empty;
  const eventContext = object(research.event);
  const events: EvidenceEvent[] = [];
  const eventIDs = new Set<string>();
  // Duplicate event IDs are ambiguous even if each individual record looks
  // valid. Never project the first match as if it were uniquely referenced.
  const eventCounts = new Map<string, number>();
  items(eventContext?.events).forEach(raw => {
    const id = string(object(raw)?.id);
    if (id) eventCounts.set(id, (eventCounts.get(id) ?? 0) + 1);
  });

  items(eventContext?.events).forEach(raw => {
    const item = object(raw);
    const id = string(item?.id);
    if (!id || eventCounts.get(id) !== 1) return;
    eventIDs.add(id);
    events.push({
      id,
      headline: string(item?.headline) ?? "未提供标题",
      summary: string(item?.fact_summary) ?? "未提供事实摘要",
      type: string(item?.type),
      occurredAt: string(item?.occurred_at),
      sourceCount: number(item?.source_count),
      documents: strings(item?.evidence_document_ids).map(id => ({ id })),
    });
  });

  const argumentsOut: EvidenceArgument[] = [];
  // Exclude every occurrence of a duplicated argument ID, not just later
  // occurrences: otherwise Manager argument_ids could silently pick a claim.
  const argumentCounts = new Map<string, number>();
  items(ledger.arguments).forEach(raw => {
    const id = string(object(raw)?.id);
    if (id) argumentCounts.set(id, (argumentCounts.get(id) ?? 0) + 1);
  });
  items(ledger.arguments).forEach(raw => {
    const item = object(raw);
    const id = string(item?.id);
    const claim = string(item?.claim);
    if (!id || !claim || argumentCounts.get(id) !== 1) return;
    const challenges: EvidenceChallenge[] = items(item?.challenges).flatMap(rawChallenge => {
      const ch = object(rawChallenge);
      const text = string(ch?.claim);
      return text ? [{
        side: string(ch?.side), round: number(ch?.round),
        claim: text, references: links(ch?.references, eventIDs),
      }] : [];
    });
    argumentsOut.push({
      id, claim, side: string(item?.side), round: number(item?.round),
      disposition: string(item?.disposition),
      references: links(item?.references, eventIDs), challenges,
    });
  });

  const insights: EvidenceInsight[] = [];
  items(ledger.insights).forEach(raw => {
    const item = object(raw);
    const claim = string(item?.claim);
    if (!claim) return;
    insights.push({
      kind: string(item?.kind), claim,
      // Go validates these relations: the UI keeps the IDs explicit without
      // claiming semantic support merely because citations overlap.
      argumentIds: strings(item?.argument_ids),
      references: links(item?.references, eventIDs),
    });
  });
  return { events, arguments: argumentsOut, insights };
}
