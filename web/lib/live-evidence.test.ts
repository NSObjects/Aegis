import { describe, expect, it } from "vitest";
import { projectEvidenceNavigator } from "./live-evidence";

const fixture = {
  research_artifact: {
    event: {
      status: "NORMAL", events: [
        { id: "evt-1", type: "DISCLOSURE", headline: "公告示例", fact_summary: "来源于后端记录的事实",
          occurred_at: "2026-10-10T09:00:00Z", source_count: 0,
          evidence_document_ids: ["doc-1", "doc-2"] },
        { id: "evt-2", headline: "另一条事件", fact_summary: "未保留文档引用" },
      ],
    },
  },
  portfolio_review: {
    trade_intent: { evidence: ["evt-2", "doc-not-structured"] },
    evidence_ledger: {
      arguments: [
        { id: "a-1", side: "BULL", round: 1, claim: "公告影响判断",
          disposition: "UNRESOLVED",
          references: [
            { domain: "EVENT", event_id: "evt-1" },
            { domain: "MARKET" },
            { domain: "EVENT", event_id: "unknown" },
          ],
          challenges: [{ side: "BEAR", round: 2, claim: "可验证性不足", references: [{ domain: "TECHNICAL" }] }],
        },
        { id: "a-2", side: "BEAR", round: 2, claim: "另一种观点", references: [{ domain: "MARKET", event_id: "evt-2" }] },
      ],
      insights: [
        { kind: "RISK", claim: "风险意见", argument_ids: ["a-1"], references: [{ domain: "EVENT", event_id: "evt-2" }] },
      ],
    },
  },
};

describe("EvidenceLedger navigator projection", () => {
  it("matches only explicit EVENT IDs to events recorded in the same Artifact", () => {
    const p = projectEvidenceNavigator(fixture);
    expect(p.events).toHaveLength(2);
    expect(p.arguments).toHaveLength(2);
    expect(p.arguments[0].references).toEqual([
      { domain: "EVENT", eventId: "evt-1", status: "MATCHED_EVENT" },
      { domain: "MARKET", eventId: null, status: "DOMAIN_ONLY" },
      { domain: "EVENT", eventId: "unknown", status: "EVENT_NOT_FOUND" },
    ]);
    expect(p.arguments[1].references).toEqual([
      { domain: "MARKET", eventId: "evt-2", status: "INVALID_REFERENCE" },
    ]);
  });
  it("does not turn event metadata or documents into invented original URLs", () => {
    const p = projectEvidenceNavigator(fixture);
    expect(p.events[0]).toEqual({
      id: "evt-1", headline: "公告示例", summary: "来源于后端记录的事实",
      type: "DISCLOSURE", occurredAt: "2026-10-10T09:00:00Z",
      sourceCount: 0, documents: [{ id: "doc-1" }, { id: "doc-2" }],
    });
    expect(p.events[1].documents).toEqual([]);
    expect(JSON.stringify(p)).not.toContain("https://");
  });
  it("preserves rebuttal targeting and manager argument links without inferring semantic support", () => {
    const p = projectEvidenceNavigator(fixture);
    expect(p.arguments[0].disposition).toBe("UNRESOLVED");
    expect(p.arguments[0].challenges[0].claim).toBe("可验证性不足");
    expect(p.arguments[0].challenges[0].references[0].status).toBe("DOMAIN_ONLY");
    expect(p.insights[0].argumentIds).toEqual(["a-1"]);
    expect(p.insights[0].references[0].status).toBe("MATCHED_EVENT");
    expect(p.arguments.find(a => a.id === "doc-not-structured")).toBeUndefined();
  });
  it("fails closed without a ledger and cannot harvest TradeIntent free text as structured references", () => {
    expect(projectEvidenceNavigator(null)).toEqual({ events: [], arguments: [], insights: [] });
    expect(projectEvidenceNavigator({ research_artifact: fixture.research_artifact,
      portfolio_review: { trade_intent: fixture.portfolio_review.trade_intent } }))
      .toEqual({ events: [], arguments: [], insights: [] });
  });
  it("rejects every duplicate event/argument identity instead of linking to the first one", () => {
    const p = projectEvidenceNavigator({
      research_artifact: { event: { events: [
        { id: "e", headline: "A" }, { id: "e", headline: "B" },
        { id: "unique", headline: "Unambiguous event" }, { id: "" },
      ] } },
      portfolio_review: { evidence_ledger: { arguments: [
        { id: "a", claim: "X", references: [{ domain: "EVENT", event_id: "e" }] },
        { id: "a", claim: "Y" }, { id: "", claim: "Z" },
        { id: "only", claim: "Valid argument", references: [
          { domain: "EVENT", event_id: "e" },
          { domain: "EVENT", event_id: "unique" },
        ] },
      ], insights: [{ kind: "RISK", claim: "Manager", argument_ids: ["a", "only"], references: [] }] } },
    });
    expect(p.events.map(item => item.id)).toEqual(["unique"]);
    expect(p.arguments.map(item => item.id)).toEqual(["only"]);
    expect(p.arguments[0].references).toEqual([
      { domain: "EVENT", eventId: "e", status: "EVENT_NOT_FOUND" },
      { domain: "EVENT", eventId: "unique", status: "MATCHED_EVENT" },
    ]);
    // Preserve the upstream ID without silently mapping it to either duplicate claim.
    expect(p.insights[0].argumentIds).toEqual(["a", "only"]);
    expect(p.arguments.some(item => item.id === "a")).toBe(false);
  });
});
