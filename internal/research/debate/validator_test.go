package debate

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/research/synthesis"
	"strings"
	"testing"
)

func TestValidateStateAcceptsGroundedDebate(t *testing.T) {
	artifact := validArtifact()
	state := &State{
		Symbol: "600519.SH",
		Turns: []Turn{
			{
				Round: 1, Side: SideBull, Thesis: "constructive", Confidence: .7,
				Arguments: []Argument{{
					ID: "bull-1", Claim: "technical setup is constructive",
					References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
				}},
			},
			{
				Round: 1, Side: SideBear, Thesis: "risks remain", Confidence: .6,
				Arguments: []Argument{{
					ID: "bear-1", Claim: "event risk remains",
					References: []synthesis.Reference{{Domain: synthesis.DomainEvent, EventID: "event-1"}},
				}},
				Rebuttals: []Rebuttal{{
					Claim: "technical strength does not remove event risk",
					TargetArgumentID: "bull-1",
					References: []synthesis.Reference{{Domain: synthesis.DomainEvent, EventID: "event-1"}},
				}},
			},
		},
	}
	if err := ValidateState(state, artifact); err != nil {
		t.Fatalf("ValidateState() error = %v", err)
	}
}

func TestValidateStateRejectsUnknownEventReference(t *testing.T) {
	artifact := validArtifact()
	state := &State{
		Symbol: "600519.SH",
		Turns: []Turn{{
			Round: 1, Side: SideBull, Thesis: "constructive", Confidence: .7,
			Arguments: []Argument{{
				ID: "bull-1", Claim: "unsupported event",
				References: []synthesis.Reference{{Domain: synthesis.DomainEvent, EventID: "invented"}},
			}},
		}},
	}
	err := ValidateState(state, artifact)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("ValidateState() error = %v, want unknown event reference", err)
	}
}

func TestValidateStateRejectsRebuttalToNonPriorArgument(t *testing.T) {
	artifact := validArtifact()
	state := &State{
		Symbol: "600519.SH",
		Turns: []Turn{{
			Round: 1, Side: SideBull, Thesis: "constructive", Confidence: .7,
			Arguments: []Argument{{
				ID: "bull-1", Claim: "technical setup",
				References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
			}},
			Rebuttals: []Rebuttal{{
				Claim: "cannot rebut a future claim", TargetArgumentID: "bear-future",
				References: []synthesis.Reference{{Domain: synthesis.DomainMarket}},
			}},
		}},
	}
	err := ValidateState(state, artifact)
	if err == nil || !strings.Contains(err.Error(), "prior argument") {
		t.Fatalf("ValidateState() error = %v, want prior argument failure", err)
	}
}

func validArtifact() *synthesis.Artifact {
	return &synthesis.Artifact{
		Symbol: "600519.SH",
		Market: &market.Context{
			AsOf: "20260930",
			Regime: market.RegimeRiskOn,
			Confidence: .8,
		},
		Technical: &technical.Context{
			Symbol: "600519.SH",
			RequestedAsOf: "20260930",
			EffectiveAsOf: "20260930",
			Status: technical.StatusNormal,
		},
		Event: &event.Context{
			Symbol: "600519.SH",
			Status: event.StatusNormal,
			Events: []event.Event{{ID: "event-1", Type: event.EventOther, Headline: "event", FactSummary: "event", EvidenceDocumentIDs: []string{"doc-1"}, SourceCount: 1, FactConfidence: .7, Materiality: event.Materiality{Level: event.MaterialityLow, Direction: event.DirectionUncertain, Horizon: event.HorizonUnknown, Rationale: "test", Confidence: .7}}},
			Confidence: .7,
		},
		Synthesis: synthesis.Synthesis{Summary: "synthesis", Confidence: .7},
	}
}
