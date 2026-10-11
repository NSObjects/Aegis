/**
 * Read-only browser projection of Go's evidenceledger.Ledger.
 * A shared domain or matching event ID is a citation identity, NEVER proof
 * of semantic entailment or a validated external source URL.
 */
export interface EvidenceEvent {
  id: string;
  headline: string | null;
  factSummary: string | null;
  occurredAt: string | null;
  documentIds: string[];
  sourceCount: number | null;
}
export interface EvidenceReference {
  domain: string;
  eventId: string | null;
  event: EvidenceEvent | null;
  status: "DOMAIN_ONLY" | "EVENT_MATCHED" | "EVENT_UNRESOLVED" | "UNKNOWN_DOMAIN";
}
export interface EvidenceChallenge {
  side: string | null;
  round: number | null;
  claim: string;
  references: EvidenceReference[];
}
export interface EvidenceArgument {
  id: string;
  side: string | null;
  round: number | null;
  claim: string;
  disposition: string | null;
  references: EvidenceReference[];
  challenges: EvidenceChallenge[];
}
export interface EvidenceInsight {
  kind: string | null;
  claim: string;
  references: EvidenceReference[];
  argumentIds: string[];
  audits: { argumentId: string; alignment: string }[];
}
export interface EvidenceInspectorData {
  available: boolean;
  conclusion: string | null;
  arguments: EvidenceArgument[];
  insights: EvidenceInsight[];
  invalidArgumentCount: number;
}
type JsonObject = Record<string, unknown>;
const obj = (v: unknown): JsonObject | null =>
  v !== null && typeof v === "object" && !Array.isArray(v) ? v as JsonObject : null;
const arr = (v: unknown): unknown[] => Array.isArray(v) ? v : [];
const str = (v: unknown): string | null =>
  typeof v === "string" && v.trim().length > 0 ? v : null;
const integer = (v: unknown): number | null =>
  typeof v === "number" && Number.isSafeInteger(v) && v >= 0 ? v : null;
const stringList = (v: unknown): string[] => arr(v).map(str).filter((s): s is string => s !== null);
const domains = new Set(["MARKET", "TECHNICAL", "EVENT", "FUNDAMENTAL"]);

export function readLiveEvidence(input: unknown): EvidenceInspectorData {
  const root = obj(input);
  const review = obj(root?.portfolio_review);
  const ledger = obj(review?.evidence_ledger);
  const empty: EvidenceInspectorData = {
    available: false, conclusion: null, arguments: [], insights: [], invalidArgumentCount: 0,
  };
  if (!ledger) return empty;

  // A duplicate event ID is not an unambiguous event citation.
  const eventIndex = new Map<string, EvidenceEvent[]>();
  const researchEvent = obj(obj(root?.research_artifact)?.event);
  for (const item of arr(researchEvent?.events)) {
    const event = obj(item);
    const id = str(event?.id);
    if (!id) continue;
    const record: EvidenceEvent = {
      id,
      headline: str(event?.headline),
      factSummary: str(event?.fact_summary),
      occurredAt: str(event?.occurred_at),
      documentIds: stringList(event?.evidence_document_ids),
      sourceCount: integer(event?.source_count),
    };
    eventIndex.set(id, [...(eventIndex.get(id) ?? []), record]);
  }

  function references(source: unknown): EvidenceReference[] {
    return arr(source).flatMap(raw => {
      const ref = obj(raw);
      const domain = str(ref?.domain);
      if (!domain) return [];
      const eventId = domain === "EVENT" ? str(ref?.event_id) : null;
      const matches = eventId ? eventIndex.get(eventId) ?? [] : [];
      const event = matches.length === 1 ? matches[0] : null;
      const status: EvidenceReference["status"] = !domains.has(domain) ? "UNKNOWN_DOMAIN" :
        domain !== "EVENT" || !eventId ? "DOMAIN_ONLY" :
        event ? "EVENT_MATCHED" : "EVENT_UNRESOLVED";
      return [{ domain, eventId, event, status }];
    });
  }

  const argumentsList: EvidenceArgument[] = [];
  const seen = new Set<string>();
  let invalidArgumentCount = 0;
  for (const raw of arr(ledger.arguments)) {
    const source = obj(raw);
    const id = str(source?.id);
    const claim = str(source?.claim);
    if (!id || !claim || seen.has(id)) {
      invalidArgumentCount++;
      continue;
    }
    seen.add(id);
    const challenges: EvidenceChallenge[] = arr(source?.challenges).flatMap(challengeRaw => {
      const challenge = obj(challengeRaw);
      const claim = str(challenge?.claim);
      return claim ? [{
        side: str(challenge?.side), round: integer(challenge?.round),
        claim, references: references(challenge?.references),
      }] : [];
    });
    argumentsList.push({
      id, side: str(source?.side), round: integer(source?.round),
      claim, disposition: str(source?.disposition),
      references: references(source?.references), challenges,
    });
  }

  const insights: EvidenceInsight[] = arr(ledger.insights).flatMap(raw => {
    const source = obj(raw);
    const claim = str(source?.claim);
    if (!claim) return [];
    const audits = arr(source?.argument_reference_audits).flatMap(rawAudit => {
      const audit = obj(rawAudit);
      const argumentId = str(audit?.argument_id);
      const alignment = str(audit?.reference_alignment);
      return argumentId && alignment ? [{ argumentId, alignment }] : [];
    });
    return [{
      kind: str(source?.kind), claim,
      references: references(source?.references),
      argumentIds: stringList(source?.argument_ids), audits,
    }];
  });
  return {
    available: true, conclusion: str(ledger.conclusion),
    arguments: argumentsList, insights, invalidArgumentCount,
  };
}
