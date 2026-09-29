package event

import (
	capevent "Aegis/internal/capability/event"
	"strings"
	"testing"
)

func TestValidateEventContext(t *testing.T) {
	valid := &Context{
		Symbol:        "600519.SH",
		RequestedAsOf: "2026-09-29T15:00:00Z",
		Status:        StatusNormal,
		Events: []Event{
			{
				ID:          "event_1",
				Type:        EventDisclosure,
				Headline:    "公告",
				FactSummary: "公司发布公告。",
				EvidenceDocumentIDs: []string{
					"doc_1",
				},
				SourceCount:      1,
				HasPrimarySource: true,
				EvidenceSupport: capevent.EvidenceSupport{
					DocumentCount:            1,
					IndependentEvidenceCount: 1,
					IndependentSourceCount:   1,
					PrimarySourcePresent:     true,
					SupportClass: capevent.
						EvidenceSupportPrimaryOnly,
				},
				FactConfidence: 0.9,
				Materiality: Materiality{
					Level:      MaterialityMedium,
					Direction:  DirectionUncertain,
					Horizon:    HorizonShortTerm,
					Rationale:  "需要结合更多经营信息判断影响。",
					Confidence: 0.7,
				},
			},
		},
		Evidence: []string{"doc_1"},
		Confidence: 0.8,
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := *valid
	invalid.Events = append(
		[]Event(nil),
		valid.Events...,
	)
	invalid.Events[0].Materiality.Direction =
		ImpactDirection("BUY")
	if err := Validate(&invalid); err == nil ||
		!strings.Contains(
			err.Error(),
			"invalid impact direction",
		) {
		t.Fatalf(
			"error = %v, want invalid-direction error",
			err,
		)
	}
}


func TestValidateEventContextEvidenceAsOf(t *testing.T) {
	valid := &Context{
		Symbol:        "600519.SH",
		RequestedAsOf: "2026-09-29T12:00:00Z",
		EvidenceAsOf:  "2026-09-29T12:00:05Z",
		Status:        StatusNoEvent,
		Confidence:    0.8,
	}
	if err := Validate(valid); err != nil {
		t.Fatalf(
			"Validate() error = %v",
			err,
		)
	}

	invalid := *valid
	invalid.EvidenceAsOf = "not-a-time"
	if err := Validate(&invalid); err == nil ||
		!strings.Contains(
			err.Error(),
			"evidence_as_of must be RFC3339",
		) {
		t.Fatalf(
			"error = %v, want evidence_as_of validation error",
			err,
		)
	}
}
