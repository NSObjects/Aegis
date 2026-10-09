package manager

import (
	"Aegis/internal/research/synthesis"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInsightArgumentIDsValidateAtDebateBoundary(t *testing.T) {
	tests := []struct {
		name      string
		ids       []string
		wantError string
	}{
		{name: "legacy omitted"},
		{name: "known argument", ids: []string{" bull-r1-a1 ", "bear-r1-a1"}},
		{name: "empty argument", ids: []string{""}, wantError: "empty"},
		{name: "duplicate normalized argument", ids: []string{"bull-r1-a1", " bull-r1-a1 "}, wantError: "appears"},
		{name: "unknown argument", ids: []string{"invented"}, wantError: "does not exist in debate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := &ResearchPlan{
				Symbol: "600519.SH", Conclusion: ConclusionBalanced, Summary: "mixed evidence",
				Opportunities: []Insight{{
					Claim: "technical support",
					References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
					ArgumentIDs: tc.ids,
				}},
				Confidence: .7,
			}
			err := Validate(plan, managerArtifact(), managerDebate())
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("valid insight links: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("invalid ids %q: err=%v, want %q", tc.ids, err, tc.wantError)
			}
		})
	}
}

func TestInsightArgumentIDsForTradingChecksOnlyShape(t *testing.T) {
	plan := &ResearchPlan{
		Symbol: "600519.SH", Conclusion: ConclusionBalanced, Summary: "a research summary",
		Risks: []Insight{{
			Claim: "market downside",
			References: []synthesis.Reference{{Domain: synthesis.DomainMarket}},
			ArgumentIDs: []string{"unknown-at-trading-boundary"},
		}},
		Confidence: .7,
	}
	if err := ValidateForTrading(plan, managerArtifact()); err != nil {
		t.Fatalf("trading boundary cannot resolve debate provenance: %v", err)
	}
	if err := Validate(plan, managerArtifact(), managerDebate()); err == nil || !strings.Contains(err.Error(), "does not exist in debate") {
		t.Fatalf("full boundary must reject unknown insight id: %v", err)
	}
	plan.Risks[0].ArgumentIDs = []string{"bull-r1-a1", " bull-r1-a1 "}
	if err := ValidateForTrading(plan, managerArtifact()); err == nil || !strings.Contains(err.Error(), "appears") {
		t.Fatalf("trading boundary must reject duplicate ids: %v", err)
	}
}

func TestInsightArgumentIDsLegacyJSONAndManagerResponse(t *testing.T) {
	legacy, err := json.Marshal(Insight{
		Claim: "a legacy insight",
		References: []synthesis.Reference{{Domain: synthesis.DomainMarket}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), "argument_ids") {
		t.Fatalf("omitted optional links changed legacy JSON: %s", legacy)
	}
	model := &managerModelStub{output: `{
		"symbol":"600519.SH", "conclusion":"BALANCED",
		"summary":"mixed evidence",
		"opportunities":[{
			"claim":"technical support", "references":[{"domain":"TECHNICAL"}],
			"argument_ids":["bull-r1-a1"]
		}],
		"confidence":0.7
	}`}
	result, err := (&ResearchManager{Model: model}).BuildPlan(context.Background(), managerArtifact(), managerDebate())
	if err != nil {
		t.Fatalf("new optional argument_ids field was not accepted: %v", err)
	}
	if len(result.Opportunities) != 1 || len(result.Opportunities[0].ArgumentIDs) != 1 ||
		result.Opportunities[0].ArgumentIDs[0] != "bull-r1-a1" {
		t.Fatalf("manager did not preserve explicit upstream argument link: %+v", result.Opportunities)
	}
}
