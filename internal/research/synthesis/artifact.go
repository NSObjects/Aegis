package synthesis

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
)

type Domain string

const (
	DomainMarket    Domain = "MARKET"
	DomainTechnical Domain = "TECHNICAL"
	DomainEvent     Domain = "EVENT"
)

type Reference struct {
	Domain  Domain `json:"domain"`
	EventID string `json:"event_id,omitempty"`
}

type Agreement struct {
	Claim      string      `json:"claim"`
	References []Reference `json:"references"`
}

type Conflict struct {
	Description string      `json:"description"`
	References  []Reference `json:"references"`
}

type Synthesis struct {
	Summary     string      `json:"summary"`
	Agreements  []Agreement `json:"agreements,omitempty"`
	Conflicts   []Conflict  `json:"conflicts,omitempty"`
	Risks       []string    `json:"risks,omitempty"`
	Confidence  float64     `json:"confidence"`
}

type Artifact struct {
	Symbol    string             `json:"symbol"`
	Market    *market.Context    `json:"market"`
	Technical *technical.Context `json:"technical"`
	Event     *event.Context     `json:"event"`
	Synthesis Synthesis          `json:"synthesis"`
}

type Input struct {
	Symbol    string
	Market    *market.Context
	Technical *technical.Context
	Event     *event.Context
}
