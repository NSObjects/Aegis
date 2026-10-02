package debate

import (
	"Aegis/internal/research/synthesis"
	"context"
	"fmt"
	"strings"
	"testing"
)

type researcherStub struct {
	side  Side
	calls []researcherCall
}

type researcherCall struct {
	round int
	turns int
}

func (s *researcherStub) GenerateTurn(_ context.Context, _ *synthesis.Artifact, state *State, round int) (Turn, error) {
	s.calls = append(s.calls, researcherCall{round: round, turns: len(state.Turns)})
	id := fmt.Sprintf("%s-r%d-a1", strings.ToLower(string(s.side)), round)
	return Turn{
		Round: round,
		Side:  s.side,
		Thesis: string(s.side) + " thesis",
		Arguments: []Argument{{
			ID:         id,
			Claim:      "grounded claim",
			References: []synthesis.Reference{{Domain: synthesis.DomainMarket}},
		}},
		Confidence: .6,
	}, nil
}

type mutatingResearcherStub struct {
	side Side
}

func (s *mutatingResearcherStub) GenerateTurn(_ context.Context, _ *synthesis.Artifact, state *State, round int) (Turn, error) {
	if len(state.Turns) > 0 && len(state.Turns[0].Arguments) > 0 && len(state.Turns[0].Arguments[0].References) > 0 {
		state.Turns[0].Arguments[0].References[0].Domain = synthesis.DomainTechnical
	}
	id := fmt.Sprintf("%s-r%d-a1", strings.ToLower(string(s.side)), round)
	return Turn{
		Round: round,
		Side:  s.side,
		Thesis: string(s.side) + " thesis",
		Arguments: []Argument{{
			ID:         id,
			Claim:      "grounded claim",
			References: []synthesis.Reference{{Domain: synthesis.DomainMarket}},
		}},
		Confidence: .6,
	}, nil
}

func TestRuntimeRunsBullThenBearForEachRound(t *testing.T) {
	bull := &researcherStub{side: SideBull}
	bear := &researcherStub{side: SideBear}
	state, err := (&Runtime{Bull: bull, Bear: bear, MaxRounds: 2}).Run(context.Background(), validArtifact())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(state.Turns) != 4 {
		t.Fatalf("turn count = %d, want 4", len(state.Turns))
	}
	want := []Side{SideBull, SideBear, SideBull, SideBear}
	for i, side := range want {
		if state.Turns[i].Side != side {
			t.Fatalf("turn[%d].Side = %q, want %q", i, state.Turns[i].Side, side)
		}
	}
	if bear.calls[0].turns != 1 || bull.calls[1].turns != 2 || bear.calls[1].turns != 3 {
		t.Fatalf("researchers did not receive accumulated debate state: bull=%#v bear=%#v", bull.calls, bear.calls)
	}
}

func TestRuntimeUsesDefaultMaxRounds(t *testing.T) {
	bull := &researcherStub{side: SideBull}
	bear := &researcherStub{side: SideBear}
	state, err := (&Runtime{Bull: bull, Bear: bear}).Run(context.Background(), validArtifact())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(state.Turns) != DefaultMaxRounds*2 {
		t.Fatalf("turn count = %d", len(state.Turns))
	}
}

func TestRuntimeRejectsResearcherChangingSide(t *testing.T) {
	bull := &researcherStub{side: SideBear}
	bear := &researcherStub{side: SideBear}
	_, err := (&Runtime{Bull: bull, Bear: bear, MaxRounds: 1}).Run(context.Background(), validArtifact())
	if err == nil {
		t.Fatal("expected side error")
	}
}

func TestRuntimeResearcherCannotMutatePriorReferences(t *testing.T) {
	bull := &researcherStub{side: SideBull}
	bear := &mutatingResearcherStub{side: SideBear}
	state, err := (&Runtime{Bull: bull, Bear: bear, MaxRounds: 1}).Run(context.Background(), validArtifact())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := state.Turns[0].Arguments[0].References[0].Domain
	if got != synthesis.DomainMarket {
		t.Fatalf("prior reference domain = %q, want %q", got, synthesis.DomainMarket)
	}
}
