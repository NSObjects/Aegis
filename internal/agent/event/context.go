package event

import capevent "Aegis/internal/capability/event"

type Status string

const (
	StatusNormal               Status = "NORMAL"
	StatusNoEvent              Status = "NO_EVENT"
	StatusInsufficientEvidence Status = "INSUFFICIENT_EVIDENCE"
)

type EventType string

const (
	EventDisclosure EventType = "DISCLOSURE"
	EventEarnings   EventType = "EARNINGS"
	EventGuidance   EventType = "GUIDANCE"
	EventContract   EventType = "CONTRACT"
	EventRegulatory EventType = "REGULATORY"
	EventLitigation EventType = "LITIGATION"
	EventMandA      EventType = "M_AND_A"
	EventFinancing  EventType = "FINANCING"
	EventDividend   EventType = "DIVIDEND"
	EventBuyback    EventType = "BUYBACK"
	EventManagement EventType = "MANAGEMENT"
	EventProduct    EventType = "PRODUCT"
	EventIncident   EventType = "INCIDENT"
	EventPolicy     EventType = "POLICY"
	EventIndustry   EventType = "INDUSTRY"
	EventOther      EventType = "OTHER"
)

type MaterialityLevel string

const (
	MaterialityLow     MaterialityLevel = "LOW"
	MaterialityMedium  MaterialityLevel = "MEDIUM"
	MaterialityHigh    MaterialityLevel = "HIGH"
	MaterialityUnknown MaterialityLevel = "UNKNOWN"
)

type ImpactDirection string

const (
	DirectionPositive  ImpactDirection = "POSITIVE"
	DirectionNegative  ImpactDirection = "NEGATIVE"
	DirectionMixed     ImpactDirection = "MIXED"
	DirectionNeutral   ImpactDirection = "NEUTRAL"
	DirectionUncertain ImpactDirection = "UNCERTAIN"
)

type ImpactHorizon string

const (
	HorizonImmediate  ImpactHorizon = "IMMEDIATE"
	HorizonShortTerm  ImpactHorizon = "SHORT_TERM"
	HorizonMediumTerm ImpactHorizon = "MEDIUM_TERM"
	HorizonUnknown    ImpactHorizon = "UNKNOWN"
)

type Materiality struct {
	Level      MaterialityLevel `json:"level"`
	Direction  ImpactDirection  `json:"direction"`
	Horizon    ImpactHorizon    `json:"horizon"`
	Rationale  string           `json:"rationale"`
	Risks      []string         `json:"risks,omitempty"`
	Confidence float64          `json:"confidence"`
}

type Event struct {
	ID                  string      `json:"id"`
	Type                EventType   `json:"type"`
	OccurredAt          string      `json:"occurred_at,omitempty"`
	Headline            string      `json:"headline"`
	FactSummary         string      `json:"fact_summary"`
	Entities            []string    `json:"entities,omitempty"`
	EvidenceDocumentIDs []string    `json:"evidence_document_ids"`
	SourceCount         int         `json:"source_count"`
	HasPrimarySource    bool                     `json:"has_primary_source"`
	EvidenceSupport     capevent.EvidenceSupport `json:"evidence_support"`
	FactConfidence      float64                  `json:"fact_confidence"`
	Materiality         Materiality `json:"materiality"`
}

type Context struct {
	Symbol        string   `json:"symbol"`
	RequestedAsOf string   `json:"requested_as_of"`
	// EvidenceAsOf is the observation/freeze cutoff when it differs from the
	// caller's requested research cutoff. Live capture sets this to capture
	// completion while preserving RequestedAsOf as the original request time.
	EvidenceAsOf  string   `json:"evidence_as_of,omitempty"`
	Status        Status   `json:"status"`
	Events        []Event  `json:"events,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
	Risk          []string `json:"risk,omitempty"`
	Confidence    float64  `json:"confidence"`
}
