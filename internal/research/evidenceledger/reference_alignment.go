package evidenceledger

import (
	"Aegis/internal/research/synthesis"
	"fmt"
	"strings"
)

// ReferenceAlignment describes reference overlap only. None of these values
// establishes whether the text of an Insight follows from a Debate argument.
type ReferenceAlignment string

const (
	AlignmentNoSharedReference ReferenceAlignment = "NO_SHARED_REFERENCE"
	AlignmentSameDomainOnly    ReferenceAlignment = "SAME_DOMAIN_ONLY"
	AlignmentSameEventID       ReferenceAlignment = "SAME_EVENT_ID"
)

// ArgumentReferenceAudit is derived from one explicit and validated
// ResearchPlan insight argument_id. Its status never controls trading,
// risk evaluation, PortfolioDecision, or the wording of a claim.
type ArgumentReferenceAudit struct {
	ArgumentID         string             `json:"argument_id"`
	ReferenceAlignment ReferenceAlignment `json:"reference_alignment"`
}

// alignReferences compares only typed citation identities, not claim text.
// MARKET and TECHNICAL are domain-level references. EVENT is concrete only
// when both sides cite the same nonempty event_id; distinct concrete events
// cannot be treated as overlapping simply because their domain is EVENT.
func alignReferences(insightRefs, argumentRefs []synthesis.Reference) ReferenceAlignment {
	result := AlignmentNoSharedReference
	for _, insight := range insightRefs {
		for _, argument := range argumentRefs {
			if insight.Domain != argument.Domain {
				continue
			}
			if insight.Domain == synthesis.DomainEvent {
				leftID := strings.TrimSpace(insight.EventID)
				rightID := strings.TrimSpace(argument.EventID)
				if leftID != "" && rightID != "" {
					if leftID == rightID {
						return AlignmentSameEventID
					}
					continue
				}
			}
			result = AlignmentSameDomainOnly
		}
	}
	return result
}

// auditArgumentReferences follows the caller's explicit link order. The
// upstream manager.Validate must have checked these IDs against the debate;
// this function nevertheless fails closed rather than silently mapping an
// unexpected missing ID to argument position zero.
func auditArgumentReferences(
	ids []string,
	refs []synthesis.Reference,
	positions map[string]int,
	arguments []Argument,
) ([]ArgumentReferenceAudit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	audits := make([]ArgumentReferenceAudit, 0, len(ids))
	for _, id := range ids {
		normalized := strings.TrimSpace(id)
		position, ok := positions[normalized]
		if !ok || position < 0 || position >= len(arguments) {
			return nil, fmt.Errorf("validated insight argument %q is missing from ledger", normalized)
		}
		audits = append(audits, ArgumentReferenceAudit{
			ArgumentID: normalized,
			ReferenceAlignment: alignReferences(refs, arguments[position].References),
		})
	}
	return audits, nil
}
