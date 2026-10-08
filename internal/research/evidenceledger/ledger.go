// Package evidenceledger builds an auditable, deterministic view of an
// already-validated ResearchArtifact, debate and ResearchPlan. It performs no
// model calls, data acquisition, or trading operations.
package evidenceledger

import (
	"Aegis/internal/research/debate"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"fmt"
	"strings"
)

// Disposition is Research Manager classification, NOT a trading action.
// An unmentioned argument remains UNCLASSIFIED, never implicitly rejected.
type Disposition string

const (
	DispositionAccepted     Disposition = "ACCEPTED"
	DispositionUnresolved   Disposition = "UNRESOLVED"
	DispositionUnclassified Disposition = "UNCLASSIFIED"
)

type Challenge struct {
	Side       debate.Side           `json:"side"`
	Round      int                   `json:"round"`
	Claim      string                `json:"claim"`
	References []synthesis.Reference `json:"references"`
}

type Argument struct {
	ID          string                `json:"id"`
	Side        debate.Side           `json:"side"`
	Round       int                   `json:"round"`
	Claim       string                `json:"claim"`
	References  []synthesis.Reference `json:"references"`
	Disposition Disposition           `json:"disposition"`
	// Challenges are sourced from actual later rebuttals targeting this ID.
	Challenges  []Challenge           `json:"challenges,omitempty"`
}

type InsightKind string

const (
	InsightOpportunity InsightKind = "OPPORTUNITY"
	InsightRisk        InsightKind = "RISK"
)

type Insight struct {
	Kind       InsightKind           `json:"kind"`
	Claim      string                `json:"claim"`
	References []synthesis.Reference `json:"references"`
}

// Ledger is a projection of validated upstream contracts, not a substitute
// for their underlying evidence. EVENT references lacking an event_id denote
// event-domain context only; they do not identify a specific event/document.
type Ledger struct {
	Symbol     string             `json:"symbol"`
	Conclusion manager.Conclusion `json:"conclusion"`
	Arguments  []Argument         `json:"arguments"`
	Insights   []Insight          `json:"insights"`
}

// Build rejects invalid source contracts instead of repairing or inventing
// argument IDs or evidence. Output order follows debate turns/arguments and
// then plan opportunity/risk order; map iteration never determines output.
// All reference slices are copied, so callers can mutate neither source nor
// ledger into one another by aliasing shared slice storage.
func Build(artifact *synthesis.Artifact, state *debate.State, plan *manager.ResearchPlan) (*Ledger, error) {
	if err := manager.Validate(plan, artifact, state); err != nil {
		return nil, fmt.Errorf("invalid evidence ledger inputs: %w", err)
	}
	if len(state.Turns) == 0 {
		return nil, fmt.Errorf("evidence ledger requires a non-empty debate")
	}
	accepted := make(map[string]struct{}, len(plan.AcceptedArgumentIDs))
	for _, id := range plan.AcceptedArgumentIDs {
		accepted[strings.TrimSpace(id)] = struct{}{}
	}
	unresolved := make(map[string]struct{}, len(plan.UnresolvedArgumentIDs))
	for _, id := range plan.UnresolvedArgumentIDs {
		unresolved[strings.TrimSpace(id)] = struct{}{}
	}

	count := 0
	for _, turn := range state.Turns {
		count += len(turn.Arguments)
	}
	out := &Ledger{
		Symbol: artifact.Symbol,
		Conclusion: plan.Conclusion,
		Arguments: make([]Argument, 0, count),
		Insights: make([]Insight, 0, len(plan.Opportunities)+len(plan.Risks)),
	}
	index := make(map[string]int, count)
	for _, turn := range state.Turns {
		for _, arg := range turn.Arguments {
			id := strings.TrimSpace(arg.ID)
			disposition := DispositionUnclassified
			if _, ok := accepted[id]; ok {
				disposition = DispositionAccepted
			} else if _, ok := unresolved[id]; ok {
				disposition = DispositionUnresolved
			}
			index[id] = len(out.Arguments)
			out.Arguments = append(out.Arguments, Argument{
				ID: id, Side: turn.Side, Round: turn.Round,
				Claim: arg.Claim,
				References: append([]synthesis.Reference{}, arg.References...),
				Disposition: disposition,
			})
		}
	}
	for _, turn := range state.Turns {
		for _, rebuttal := range turn.Rebuttals {
			// Guaranteed to exist and refer to a previous turn by
			// debate.ValidateState (invoked by manager.Validate above).
			target, ok := index[strings.TrimSpace(rebuttal.TargetArgumentID)]
			if !ok {
				return nil, fmt.Errorf("rebuttal target %q missing from validated debate", rebuttal.TargetArgumentID)
			}
			out.Arguments[target].Challenges = append(out.Arguments[target].Challenges, Challenge{
				Side: turn.Side, Round: turn.Round,
				Claim: rebuttal.Claim,
				References: append([]synthesis.Reference{}, rebuttal.References...),
			})
		}
	}
	for _, opportunity := range plan.Opportunities {
		out.Insights = append(out.Insights, Insight{
			Kind: InsightOpportunity,
			Claim: opportunity.Claim,
			References: append([]synthesis.Reference{}, opportunity.References...),
		})
	}
	for _, risk := range plan.Risks {
		out.Insights = append(out.Insights, Insight{
			Kind: InsightRisk,
			Claim: risk.Claim,
			References: append([]synthesis.Reference{}, risk.References...),
		})
	}
	return out, nil
}
