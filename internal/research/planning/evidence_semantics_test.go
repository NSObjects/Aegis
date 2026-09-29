package planning

import (
	"Aegis/internal/capability/event"
	"testing"
)

func TestResearchRoundResultPreservesEvidenceSemanticGaps(
	t *testing.T,
) {
	result := buildResearchRoundResult(
		1,
		event.ResearchTaskResult{
			TaskID: "task-semantic-gap",
			Status: event.TaskPartial,
			Gaps: []event.ResearchGap{
				{
					Type:      event.GapMissingRequiredAuthority,
					Authority: event.AuthorityIssuer,
					Description: "issuer authority is required",
				},
				{
					Type:         event.GapMissingRequiredEvidenceKind,
					EvidenceKind: event.EvidenceDisclosure,
					Description:  "disclosure evidence is required",
				},
			},
		},
		event.AcquisitionPlanOutput{},
		nil,
	)

	if len(result.Gaps) != 2 {
		t.Fatalf(
			"gaps = %d, want 2",
			len(result.Gaps),
		)
	}
	if result.Gaps[0].Authority != event.AuthorityIssuer {
		t.Fatalf(
			"authority = %q, want %q",
			result.Gaps[0].Authority,
			event.AuthorityIssuer,
		)
	}
	if result.Gaps[1].EvidenceKind != event.EvidenceDisclosure {
		t.Fatalf(
			"evidence kind = %q, want %q",
			result.Gaps[1].EvidenceKind,
			event.EvidenceDisclosure,
		)
	}
}
