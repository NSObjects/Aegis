package research

import (
	"Aegis/internal/risk"
	"context"
	"encoding/json"
	"testing"
	"time"
)

// This is the complete response shape serialized by cmd/aegis in
// -portfolio-review mode, not just the inner PortfolioReviewResult.
func TestFullPortfolioWorkflowJSONPreservesEvidenceLedgerContract(t *testing.T) {
	ctx := context.Background()
	graph, err := NewArtifact(ctx, Config{
		MarketAgent:    &fakeMarketAgent{result: validMarketContext()},
		TechnicalAgent: &fakeTechnicalAgent{result: validTechnicalContext()},
		EventAgent:     &fakeEventAgent{result: validEventContext()},
		Synthesizer:    &fakeSynthesizer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	review, _, _, _, _ := reviewWorkflowFixture(t, reviewContextFixture(risk.AssessmentHighRisk))
	book := reviewBookFixture()
	book.AsOf = validWorkflowInput().RequestedAsOf.Format(time.RFC3339)
	result, err := (&FullPortfolioWorkflow{Research: graph, Review: review}).Run(
		ctx, validWorkflowInput(), book,
	)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var outer map[string]json.RawMessage
	if err := json.Unmarshal(payload, &outer); err != nil {
		t.Fatal(err)
	}
	if len(outer["research_artifact"]) == 0 || string(outer["research_artifact"]) == "null" {
		t.Fatal("research_artifact missing from production response")
	}
	var reviewJSON map[string]json.RawMessage
	if err := json.Unmarshal(outer["portfolio_review"], &reviewJSON); err != nil {
		t.Fatalf("portfolio_review absent or invalid: %v", err)
	}
	for _, key := range []string{"debate_state", "research_plan", "trade_intent", "risk_context", "portfolio_decision"} {
		if len(reviewJSON[key]) == 0 || string(reviewJSON[key]) == "null" {
			t.Fatalf("portfolio_review.%s missing or null", key)
		}
	}

	var ledger struct {
		Symbol    string `json:"symbol"`
		Arguments []struct {
			ID          string `json:"id"`
			Disposition string `json:"disposition"`
			References  []struct {
				Domain string `json:"domain"`
			} `json:"references"`
		} `json:"arguments"`
	}
	if len(reviewJSON["evidence_ledger"]) == 0 || string(reviewJSON["evidence_ledger"]) == "null" {
		t.Fatal("portfolio_review.evidence_ledger missing or null")
	}
	if err := json.Unmarshal(reviewJSON["evidence_ledger"], &ledger); err != nil {
		t.Fatalf("invalid evidence_ledger JSON: %v", err)
	}
	if ledger.Symbol != "600519.SH" || len(ledger.Arguments) != 2 {
		t.Fatalf("invalid evidence ledger identity/count: %+v", ledger)
	}
	if ledger.Arguments[0].ID != "bull-1" ||
		ledger.Arguments[0].Disposition != "ACCEPTED" ||
		len(ledger.Arguments[0].References) != 1 ||
		ledger.Arguments[0].References[0].Domain != "TECHNICAL" {
		t.Fatalf("bull argument provenance lost: %+v", ledger.Arguments[0])
	}
	if ledger.Arguments[1].ID != "bear-1" ||
		ledger.Arguments[1].Disposition != "UNRESOLVED" ||
		len(ledger.Arguments[1].References) != 1 ||
		ledger.Arguments[1].References[0].Domain != "MARKET" {
		t.Fatalf("bear argument provenance lost: %+v", ledger.Arguments[1])
	}
}
