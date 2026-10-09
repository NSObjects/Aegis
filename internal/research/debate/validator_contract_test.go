package debate

import (
	"Aegis/internal/research/synthesis"
	"strings"
	"testing"
)

func TestValidateStateRejectsDecreasingRounds(t *testing.T) {
	state := &State{Symbol: "600519.SH", Turns: []Turn{
		{Round: 2, Side: SideBull, Thesis: "bull", Confidence: .7,
			Arguments: []Argument{{ID: "bull-1", Claim: "bull claim", References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}}}}},
		{Round: 1, Side: SideBear, Thesis: "bear", Confidence: .7,
			Arguments: []Argument{{ID: "bear-1", Claim: "bear claim", References: []synthesis.Reference{{Domain: synthesis.DomainMarket}}}},
			Rebuttals: []Rebuttal{{TargetArgumentID: "bull-1", Claim: "bear rebuttal", References: []synthesis.Reference{{Domain: synthesis.DomainMarket}}}}},
	}}
	if err := ValidateState(state, validArtifact()); err == nil || !strings.Contains(err.Error(), "precedes prior round") {
		t.Fatalf("decreasing round accepted: %v", err)
	}
	state.Turns[1].Round = 2
	if err := ValidateState(state, validArtifact()); err != nil {
		t.Fatalf("same-round turns rejected: %v", err)
	}
}

func TestReferenceRejectsNoncanonicalEventIDs(t *testing.T) {
	artifact := validArtifact()
	for _, ref := range []synthesis.Reference{
		{Domain: synthesis.DomainMarket, EventID: " "},
		{Domain: synthesis.DomainTechnical, EventID: " "},
		{Domain: synthesis.DomainEvent, EventID: " "},
		{Domain: synthesis.DomainEvent, EventID: " event-1 "},
	} {
		if err := validateReference(ref, artifact); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
			t.Fatalf("invalid %+v accepted: %v", ref, err)
		}
	}
}
