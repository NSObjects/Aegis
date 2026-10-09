package research

import (
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"context"
	"strings"
	"testing"
)

func TestPortfolioReviewRejectsUnknownManagerInsightArgumentBeforeTrader(t *testing.T) {
	w, _, researchManager, trader, riskAnalyst := reviewWorkflowFixture(
		t, reviewContextFixture(risk.AssessmentHighRisk),
	)
	researchManager.plan.Opportunities = []manager.Insight{{
		Claim: "technical case",
		References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
		ArgumentIDs: []string{"forged-insight-argument"},
	}}
	result, err := w.Run(context.Background(), reviewArtifactFixture(), reviewBookFixture())
	if err == nil || result != nil || !strings.Contains(err.Error(), "does not exist in debate") {
		t.Fatalf("manager provenance must fail closed: result=%+v err=%v", result, err)
	}
	if trader.calls != 0 || riskAnalyst.calls != 0 {
		t.Fatalf("invalid Manager provenance reached downstream agents: trading=%d risk=%d", trader.calls, riskAnalyst.calls)
	}
}

func TestPortfolioReviewPassesValidatedInsightLinksToLedger(t *testing.T) {
	w, _, researchManager, _, _ := reviewWorkflowFixture(
		t, reviewContextFixture(risk.AssessmentHighRisk),
	)
	researchManager.plan.Opportunities = []manager.Insight{{
		Claim: "technical case",
		References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
		ArgumentIDs: []string{"bull-1"},
	}}
	result, err := w.Run(context.Background(), reviewArtifactFixture(), reviewBookFixture())
	if err != nil {
		t.Fatal(err)
	}
	if result.EvidenceLedger == nil || len(result.EvidenceLedger.Insights) != 1 ||
		len(result.EvidenceLedger.Insights[0].ArgumentIDs) != 1 ||
		result.EvidenceLedger.Insights[0].ArgumentIDs[0] != "bull-1" {
		t.Fatalf("validated manager insight provenance missing: %+v", result.EvidenceLedger)
	}
}
