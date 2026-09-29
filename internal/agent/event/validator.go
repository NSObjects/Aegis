package event

import (
	capevent "Aegis/internal/capability/event"
	"errors"
	"fmt"
	"strings"
	"time"
)

func Validate(v *Context) error {
	if v == nil {
		return errors.New("event context is nil")
	}
	if strings.TrimSpace(v.Symbol) == "" {
		return errors.New("symbol is required")
	}
	if strings.TrimSpace(v.RequestedAsOf) == "" {
		return errors.New("requested_as_of is required")
	}
	if _, err := time.Parse(
		time.RFC3339,
		v.RequestedAsOf,
	); err != nil {
		return fmt.Errorf(
			"requested_as_of must be RFC3339: %w",
			err,
		)
	}
	if strings.TrimSpace(v.EvidenceAsOf) != "" {
		if _, err := time.Parse(
			time.RFC3339,
			v.EvidenceAsOf,
		); err != nil {
			return fmt.Errorf(
				"evidence_as_of must be RFC3339: %w",
				err,
			)
		}
	}
	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"confidence out of range: %f",
			v.Confidence,
		)
	}

	switch v.Status {
	case StatusNormal:
		if len(v.Events) == 0 {
			return errors.New(
				"events are required for NORMAL status",
			)
		}
		if len(v.Evidence) == 0 {
			return errors.New(
				"evidence is required for NORMAL status",
			)
		}
	case StatusNoEvent:
		if len(v.Events) != 0 {
			return errors.New(
				"events must be empty for NO_EVENT status",
			)
		}
	case StatusInsufficientEvidence:
		if len(v.Events) != 0 {
			return errors.New(
				"events must be empty for INSUFFICIENT_EVIDENCE status",
			)
		}
		if len(v.Risk) == 0 {
			return errors.New(
				"risk is required for INSUFFICIENT_EVIDENCE status",
			)
		}
	default:
		return fmt.Errorf(
			"invalid event status: %q",
			v.Status,
		)
	}

	seenEventIDs := make(map[string]struct{}, len(v.Events))
	for index := range v.Events {
		item := &v.Events[index]
		if err := validateEvent(item); err != nil {
			return fmt.Errorf(
				"event %d: %w",
				index,
				err,
			)
		}
		if _, duplicate := seenEventIDs[item.ID]; duplicate {
			return fmt.Errorf(
				"event id %q is duplicated",
				item.ID,
			)
		}
		seenEventIDs[item.ID] = struct{}{}
	}

	return nil
}

func validateEvent(v *Event) error {
	if v == nil {
		return errors.New("event is nil")
	}
	if strings.TrimSpace(v.ID) == "" {
		return errors.New("id is required")
	}
	if !validEventType(v.Type) {
		return fmt.Errorf(
			"invalid type: %q",
			v.Type,
		)
	}
	if strings.TrimSpace(v.Headline) == "" {
		return errors.New("headline is required")
	}
	if strings.TrimSpace(v.FactSummary) == "" {
		return errors.New("fact_summary is required")
	}
	if v.OccurredAt != "" {
		if _, err := time.Parse(
			time.RFC3339,
			v.OccurredAt,
		); err != nil {
			return fmt.Errorf(
				"occurred_at must be RFC3339: %w",
				err,
			)
		}
	}
	if len(v.Facts) == 0 {
		return errors.New(
			"facts are required",
		)
	}
	factEvidenceIDs := make([]string, 0)
	seenFactIDs := make(
		map[string]struct{},
		len(v.Facts),
	)
	for index := range v.Facts {
		fact := &v.Facts[index]
		if err := validateGroundedFact(fact); err != nil {
			return fmt.Errorf(
				"fact %d: %w",
				index,
				err,
			)
		}
		if _, duplicate :=
			seenFactIDs[fact.ID]; duplicate {
			return fmt.Errorf(
				"fact id %q is duplicated",
				fact.ID,
			)
		}
		seenFactIDs[fact.ID] = struct{}{}
		factEvidenceIDs = append(
			factEvidenceIDs,
			fact.EvidenceDocumentIDs...,
		)
	}
	factEvidenceIDs =
		uniqueSortedStrings(factEvidenceIDs)
	eventEvidenceIDs :=
		uniqueSortedStrings(
			v.EvidenceDocumentIDs,
		)
	if !equalStringSlices(
		eventEvidenceIDs,
		factEvidenceIDs,
	) {
		return errors.New(
			"evidence_document_ids must equal the union of grounded fact evidence",
		)
	}
	if len(eventEvidenceIDs) == 0 {
		return errors.New(
			"evidence_document_ids are required",
		)
	}
	if v.EvidenceSupport.DocumentCount !=
		len(eventEvidenceIDs) {
		return errors.New(
			"evidence_support document_count must equal grounded event evidence count",
		)
	}
	if v.SourceCount < 0 {
		return errors.New(
			"source_count must not be negative",
		)
	}
	if err := validateEvidenceSupport(
		v.EvidenceSupport,
	); err != nil {
		return fmt.Errorf(
			"evidence_support: %w",
			err,
		)
	}
	if v.SourceCount !=
		v.EvidenceSupport.IndependentSourceCount {
		return errors.New(
			"source_count must equal evidence_support independent_source_count",
		)
	}
	if v.HasPrimarySource !=
		v.EvidenceSupport.PrimarySourcePresent {
		return errors.New(
			"has_primary_source must equal evidence_support primary_source_present",
		)
	}
	if v.FactConfidence < 0 ||
		v.FactConfidence > 1 {
		return fmt.Errorf(
			"fact_confidence out of range: %f",
			v.FactConfidence,
		)
	}

	if err := validateContradictions(
		v.Facts,
		v.Contradictions,
	); err != nil {
		return fmt.Errorf(
			"contradictions: %w",
			err,
		)
	}

	if err := validateMateriality(
		v.Materiality,
	); err != nil {
		return err
	}

	return nil
}

func validateGroundedFact(
	v *Fact,
) error {
	if v == nil {
		return errors.New("fact is nil")
	}
	if strings.TrimSpace(v.ID) == "" {
		return errors.New("id is required")
	}
	if !validEventType(v.Type) {
		return fmt.Errorf(
			"invalid type: %q",
			v.Type,
		)
	}
	if strings.TrimSpace(v.Headline) == "" {
		return errors.New("headline is required")
	}
	if strings.TrimSpace(v.Summary) == "" {
		return errors.New("summary is required")
	}
	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"confidence out of range: %f",
			v.Confidence,
		)
	}
	if len(v.Groundings) == 0 {
		return errors.New("groundings are required")
	}

	groundingIDs := make(
		[]string,
		0,
		len(v.Groundings),
	)
	seenGroundings := make(map[string]struct{})
	for index, grounding := range v.Groundings {
		documentID := strings.TrimSpace(
			grounding.DocumentID,
		)
		if documentID == "" {
			return fmt.Errorf(
				"grounding %d document_id is required",
				index,
			)
		}
		if strings.TrimSpace(grounding.Quote) == "" {
			return fmt.Errorf(
				"grounding %d quote is required",
				index,
			)
		}
		if grounding.StartByte < 0 ||
			grounding.EndByte <=
				grounding.StartByte {
			return fmt.Errorf(
				"grounding %d has invalid byte span",
				index,
			)
		}
		if grounding.EndByte-
			grounding.StartByte !=
			len(grounding.Quote) {
			return fmt.Errorf(
				"grounding %d byte span length does not match quote",
				index,
			)
		}
		key := fmt.Sprintf(
			"%s\x00%d\x00%d\x00%s",
			documentID,
			grounding.StartByte,
			grounding.EndByte,
			grounding.Quote,
		)
		if _, duplicate :=
			seenGroundings[key]; duplicate {
			return fmt.Errorf(
				"grounding %d is duplicated",
				index,
			)
		}
		seenGroundings[key] = struct{}{}
		groundingIDs = append(
			groundingIDs,
			documentID,
		)
	}

	groundingIDs =
		uniqueSortedStrings(groundingIDs)
	evidenceIDs :=
		uniqueSortedStrings(
			v.EvidenceDocumentIDs,
		)
	if !equalStringSlices(
		groundingIDs,
		evidenceIDs,
	) {
		return errors.New(
			"evidence_document_ids must be derived from groundings",
		)
	}
	return nil
}

func equalStringSlices(
	left []string,
	right []string,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateEvidenceSupport(
	v capevent.EvidenceSupport,
) error {
	if v.DocumentCount <= 0 {
		return errors.New(
			"document_count must be positive",
		)
	}
	if v.IndependentEvidenceCount <= 0 ||
		v.IndependentEvidenceCount >
			v.DocumentCount {
		return errors.New(
			"independent_evidence_count must be between 1 and document_count",
		)
	}
	if v.IndependentSourceCount < 0 ||
		v.IndependentSourceCount >
			v.IndependentEvidenceCount {
		return errors.New(
			"independent_source_count must be between 0 and independent_evidence_count",
		)
	}
	if v.CollapsedReprintCount !=
		v.DocumentCount-
			v.IndependentEvidenceCount {
		return errors.New(
			"collapsed_reprint_count is inconsistent",
		)
	}
	if v.UnknownSourceCount < 0 ||
		v.UnknownSourceCount >
			v.IndependentEvidenceCount {
		return errors.New(
			"unknown_source_count is invalid",
		)
	}

	switch v.SupportClass {
	case capevent.EvidenceSupportUnknown:
		if v.IndependentSourceCount != 0 ||
			v.PrimarySourcePresent {
			return errors.New(
				"UNKNOWN support class is inconsistent",
			)
		}
	case capevent.EvidenceSupportSingleSource:
		if v.IndependentSourceCount != 1 ||
			v.PrimarySourcePresent {
			return errors.New(
				"SINGLE_SOURCE support class is inconsistent",
			)
		}
	case capevent.EvidenceSupportPrimaryOnly:
		if !v.PrimarySourcePresent ||
			v.IndependentSourceCount >= 2 {
			return errors.New(
				"PRIMARY_ONLY support class is inconsistent",
			)
		}
	case capevent.EvidenceSupportCorroborated:
		if v.IndependentSourceCount < 2 ||
			v.PrimarySourcePresent {
			return errors.New(
				"CORROBORATED support class is inconsistent",
			)
		}
	case capevent.EvidenceSupportPrimaryCorroborated:
		if !v.PrimarySourcePresent ||
			v.IndependentSourceCount < 2 {
			return errors.New(
				"PRIMARY_CORROBORATED support class is inconsistent",
			)
		}
	default:
		return fmt.Errorf(
			"invalid support_class: %q",
			v.SupportClass,
		)
	}
	return nil
}

func validateMateriality(
	v Materiality,
) error {
	switch v.Level {
	case MaterialityLow,
		MaterialityMedium,
		MaterialityHigh,
		MaterialityUnknown:
	default:
		return fmt.Errorf(
			"invalid materiality level: %q",
			v.Level,
		)
	}

	switch v.Direction {
	case DirectionPositive,
		DirectionNegative,
		DirectionMixed,
		DirectionNeutral,
		DirectionUncertain:
	default:
		return fmt.Errorf(
			"invalid impact direction: %q",
			v.Direction,
		)
	}

	switch v.Horizon {
	case HorizonImmediate,
		HorizonShortTerm,
		HorizonMediumTerm,
		HorizonUnknown:
	default:
		return fmt.Errorf(
			"invalid impact horizon: %q",
			v.Horizon,
		)
	}

	if strings.TrimSpace(v.Rationale) == "" {
		return errors.New(
			"materiality rationale is required",
		)
	}
	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"materiality confidence out of range: %f",
			v.Confidence,
		)
	}
	return nil
}

func validEventType(
	value EventType,
) bool {
	switch value {
	case EventDisclosure,
		EventEarnings,
		EventGuidance,
		EventContract,
		EventRegulatory,
		EventLitigation,
		EventMandA,
		EventFinancing,
		EventDividend,
		EventBuyback,
		EventManagement,
		EventProduct,
		EventIncident,
		EventPolicy,
		EventIndustry,
		EventOther:
		return true
	default:
		return false
	}
}
