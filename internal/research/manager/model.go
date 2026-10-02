package manager

import "Aegis/internal/research/synthesis"

type Conclusion string

const (
	ConclusionBullLean             Conclusion = "BULL_LEAN"
	ConclusionBearLean             Conclusion = "BEAR_LEAN"
	ConclusionBalanced             Conclusion = "BALANCED"
	ConclusionInsufficientEvidence Conclusion = "INSUFFICIENT_EVIDENCE"
)

type Insight struct {
	Claim      string                `json:"claim"`
	References []synthesis.Reference `json:"references"`
}

type ResearchPlan struct {
	Symbol                string     `json:"symbol"`
	Conclusion            Conclusion `json:"conclusion"`
	Summary               string     `json:"summary"`
	Opportunities         []Insight  `json:"opportunities,omitempty"`
	Risks                 []Insight  `json:"risks,omitempty"`
	AcceptedArgumentIDs   []string   `json:"accepted_argument_ids,omitempty"`
	UnresolvedArgumentIDs []string   `json:"unresolved_argument_ids,omitempty"`
	TradingConstraints    []string   `json:"trading_constraints,omitempty"`
	Confidence            float64    `json:"confidence"`
}
