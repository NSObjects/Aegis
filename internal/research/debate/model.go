package debate

import "Aegis/internal/research/synthesis"

type Side string

const (
	SideBull Side = "BULL"
	SideBear Side = "BEAR"
)

type Argument struct {
	ID         string                `json:"id"`
	Claim      string                `json:"claim"`
	References []synthesis.Reference `json:"references"`
}

type Rebuttal struct {
	Claim            string                `json:"claim"`
	TargetArgumentID string                `json:"target_argument_id"`
	References       []synthesis.Reference `json:"references"`
}

type Turn struct {
	Round      int        `json:"round"`
	Side       Side       `json:"side"`
	Thesis     string     `json:"thesis"`
	Arguments  []Argument `json:"arguments"`
	Rebuttals  []Rebuttal `json:"rebuttals,omitempty"`
	Confidence float64    `json:"confidence"`
}

type State struct {
	Symbol string `json:"symbol"`
	Turns  []Turn `json:"turns"`
}
