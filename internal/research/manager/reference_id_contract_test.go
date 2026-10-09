package manager

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/research/synthesis"
	"strings"
	"testing"
)

func TestReferenceRejectsNoncanonicalEventIDs(t *testing.T) {
	artifact := &synthesis.Artifact{Event: &event.Context{Events: []event.Event{{ID: "event-1"}}}}
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
