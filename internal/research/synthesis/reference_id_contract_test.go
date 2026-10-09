package synthesis

import (
	"strings"
	"testing"
)

func TestReferenceRejectsNoncanonicalEventIDs(t *testing.T) {
	known := map[string]struct{}{"event-1": {}}
	for _, ref := range []Reference{
		{Domain: DomainMarket, EventID: " "},
		{Domain: DomainTechnical, EventID: " "},
		{Domain: DomainEvent, EventID: " "},
		{Domain: DomainEvent, EventID: " event-1 "},
	} {
		if err := validateReference(ref, known); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
			t.Fatalf("invalid %+v accepted: %v", ref, err)
		}
	}
	if err := validateReference(Reference{Domain: DomainEvent}, known); err != nil {
		t.Fatalf("domain-only event rejected: %v", err)
	}
}
