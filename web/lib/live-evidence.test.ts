import { describe, expect, it } from "vitest";
import { readLiveEvidence } from "./live-evidence";

const fixture = {
  research_artifact: {
    event: { events: [{
      id: "EVT-1", headline: "公开公告", fact_summary: "归档公告事实",
      occurred_at: "2026-10-09T08:00:00Z", source_count: 2,
      evidence_document_ids: ["DOC-9"],
    }] },
  },
  portfolio_review: {
    evidence_ledger: {
      conclusion: "BALANCED",
      arguments: [
        { id: "bull-1", side: "BULL", round: 1, claim: "多头主张",
          disposition: "UNRESOLVED", references: [{ domain: "EVENT", event_id: "EVT-1" }],
          challenges: [{ side: "BEAR", round: 2, claim: "反驳观点", references: [{ domain: "MARKET" }] }] },
        { id: "bear-1", side: "BEAR", round: 2, claim: "空头主张",
          disposition: "UNCLASSIFIED", references: [{ domain: "EVENT" }] },
      ],
      insights: [
        { kind: "RISK", claim: "需进一步证实", argument_ids: ["bull-1"],
          references: [{ domain: "EVENT", event_id: "EVT-1" }],
          argument_reference_audits: [{ argument_id: "bull-1", reference_alignment: "SAME_EVENT_ID" }] },
        { kind: "OPPORTUNITY", claim: "不链接任何论点", references: [{ domain: "MARKET" }] },
      ],
    },
  },
};

describe("Live EvidenceLedger identity navigation", () => {
  it("maps only literal argument, challenge, manager and event IDs from one artifact", () => {
    const read = readLiveEvidence(fixture);
    expect(read.available).toBe(true);
    expect(read.conclusion).toBe("BALANCED");
    expect(read.arguments).toHaveLength(2);
    expect(read.arguments[0].disposition).toBe("UNRESOLVED");
    expect(read.arguments[0].challenges[0].claim).toBe("反驳观点");
    expect(read.arguments[0].challenges[0].references[0].status).toBe("DOMAIN_ONLY");
    const link = read.arguments[0].references[0];
    expect(link).toMatchObject({ domain: "EVENT", eventId: "EVT-1", status: "EVENT_MATCHED" });
    expect(link.event).toMatchObject({
      id: "EVT-1", headline: "公开公告", documentIds: ["DOC-9"],
      occurredAt: "2026-10-09T08:00:00Z", sourceCount: 2,
    });
    expect(read.insights[0].argumentIds).toEqual(["bull-1"]);
    expect(read.insights[0].audits).toEqual([{ argumentId: "bull-1", alignment: "SAME_EVENT_ID" }]);
    expect(read.insights[1].argumentIds).toEqual([]);
  });

  it("does not synthesize Ledger, event citations, links, or arguments for Decision and incomplete runs", () => {
    expect(readLiveEvidence({ action: "WATCH" })).toMatchObject({
      available: false, arguments: [], insights: [],
    });
    expect(readLiveEvidence({ portfolio_review: {} }).available).toBe(false);
    expect(readLiveEvidence({ portfolio_review: { evidence_ledger: {} } })).toMatchObject({
      available: true, arguments: [], insights: [], conclusion: null,
    });
  });

  it("does not turn event-domain-only, missing or ambiguous event IDs into concrete events", () => {
    const model = {
      research_artifact: { event: { events: [
        fixture.research_artifact.event.events[0],
        { ...fixture.research_artifact.event.events[0], fact_summary: "第二个同名 ID" },
      ] } },
      portfolio_review: { evidence_ledger: { arguments: [{
        id: "bull-1", claim: "多头主张", references: [
          { domain: "EVENT", event_id: "EVT-1" },
          { domain: "EVENT", event_id: "EVT-NOT-FOUND" },
          { domain: "EVENT" }, { domain: "MARKET" },
        ],
      }] } },
    };
    const refs = readLiveEvidence(model).arguments[0].references;
    expect(refs.map(ref => ref.status)).toEqual([
      "EVENT_UNRESOLVED", "EVENT_UNRESOLVED", "DOMAIN_ONLY", "DOMAIN_ONLY",
    ]);
    expect(refs.every(ref => ref.event === null)).toBe(true);
  });

  it("stops navigation to duplicate or malformed argument identities without reassigning links", () => {
    const model = structuredClone(fixture);
    model.portfolio_review.evidence_ledger.arguments.push({
      ...model.portfolio_review.evidence_ledger.arguments[0], claim: "冲突论点",
    });
    const read = readLiveEvidence(model);
    expect(read.invalidArgumentCount).toBe(1);
    expect(read.arguments).toHaveLength(2);
    expect(read.arguments[0].claim).toBe("多头主张");
  });

  it("never treats a numeric source count or metadata as a source URL", () => {
    const read = readLiveEvidence(fixture);
    const matched = read.arguments[0].references[0].event;
    expect(matched?.sourceCount).toBe(2);
    expect(Object.keys(matched ?? {})).not.toContain("url");
    expect(read.arguments[1].references[0]).toMatchObject({ status: "DOMAIN_ONLY", event: null });
  });
});
