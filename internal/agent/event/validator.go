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
	if err := validateEventContradictions(
		v.Contradictions,
		v.Facts,
	); err != nil {
		return fmt.Errorf(
			"contradictions: %w",
			err,
		)
	}
	if err := validateEventFactRelations(v.FactRelations, v.Facts); err != nil {
		return fmt.Errorf("fact_relations: %w", err)
	}
	if err := validateEventImpactMechanisms(v.ImpactMechanisms, v.Facts); err != nil {
		return fmt.Errorf("impact_mechanisms: %w", err)
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

	if err := validateMateriality(
		v.Materiality,
	); err != nil {
		return err
	}

	return nil
}

func validateEventContradictions(
	values []ClaimContradiction,
	facts []Fact,
) error {
	factByID := make(map[string]Fact, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}
	seen := make(map[string]struct{}, len(values))
	for index, item := range values {
		if item.LeftFactID == "" ||
			item.RightFactID == "" {
			return fmt.Errorf(
				"contradiction %d requires both fact ids",
				index,
			)
		}
		if item.LeftFactID >= item.RightFactID {
			return fmt.Errorf(
				"contradiction %d fact ids must be in canonical order",
				index,
			)
		}
		if !validContradictionKind(item.Kind) {
			return fmt.Errorf(
				"contradiction %d has invalid kind %q",
				index,
				item.Kind,
			)
		}
		if strings.TrimSpace(item.Rationale) == "" {
			return fmt.Errorf(
				"contradiction %d rationale is required",
				index,
			)
		}
		left, leftOK := factByID[item.LeftFactID]
		right, rightOK := factByID[item.RightFactID]
		if !leftOK || !rightOK {
			return fmt.Errorf(
				"contradiction %d references unknown event fact",
				index,
			)
		}
		if !equalStringSlices(
			uniqueSortedStrings(
				item.LeftEvidenceDocumentIDs,
			),
			uniqueSortedStrings(
				left.EvidenceDocumentIDs,
			),
		) {
			return fmt.Errorf(
				"contradiction %d left evidence is inconsistent",
				index,
			)
		}
		if !equalStringSlices(
			uniqueSortedStrings(
				item.RightEvidenceDocumentIDs,
			),
			uniqueSortedStrings(
				right.EvidenceDocumentIDs,
			),
		) {
			return fmt.Errorf(
				"contradiction %d right evidence is inconsistent",
				index,
			)
		}
		expectedEvidence := uniqueSortedStrings(
			append(
				append(
					[]string(nil),
					left.EvidenceDocumentIDs...,
				),
				right.EvidenceDocumentIDs...,
			),
		)
		if !equalStringSlices(
			uniqueSortedStrings(
				item.EvidenceDocumentIDs,
			),
			expectedEvidence,
		) {
			return fmt.Errorf(
				"contradiction %d evidence union is inconsistent",
				index,
			)
		}
		key := item.LeftFactID +
			"\x00" + item.RightFactID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"contradiction %d duplicates a fact pair",
				index,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateEventFactRelations(values []FactRelation, facts []Fact) error {
	factIDs := make(map[string]struct{}, len(facts))
	for _, fact := range facts { factIDs[fact.ID] = struct{}{} }
	seenIDs := make(map[string]struct{}, len(values))
	seenRelations := make(map[string]struct{}, len(values))
	for index, item := range values {
		if strings.TrimSpace(item.ID) == "" { return fmt.Errorf("relation %d id is required", index) }
		if !validFactRelationType(item.Type) { return fmt.Errorf("relation %d has invalid type %q", index, item.Type) }
		if item.FromFactID == item.ToFactID { return fmt.Errorf("relation %d cannot reference the same fact twice", index) }
		if _, ok := factIDs[item.FromFactID]; !ok { return fmt.Errorf("relation %d references unknown event fact", index) }
		if _, ok := factIDs[item.ToFactID]; !ok { return fmt.Errorf("relation %d references unknown event fact", index) }
		if strings.TrimSpace(item.Rationale) == "" { return fmt.Errorf("relation %d rationale is required", index) }
		expectedID := deterministicFactRelationID(item.Type, item.FromFactID, item.ToFactID)
		if item.ID != expectedID { return fmt.Errorf("relation %d id is not deterministic", index) }
		if _, duplicate := seenIDs[item.ID]; duplicate { return fmt.Errorf("relation %d duplicates relation id", index) }
		seenIDs[item.ID] = struct{}{}
		key := string(item.Type)+"\\x00"+item.FromFactID+"\\x00"+item.ToFactID
		if _, duplicate := seenRelations[key]; duplicate { return fmt.Errorf("relation %d is duplicated", index) }
		seenRelations[key] = struct{}{}
	}
	return nil
}

func validateEventImpactMechanisms(values []ImpactMechanism, facts []Fact) error {
	factIDs:=make(map[string]struct{},len(facts))
	for _,fact:=range facts { factIDs[fact.ID]=struct{}{} }
	seen:=make(map[string]struct{},len(values))
	for index,item:=range values {
		if strings.TrimSpace(item.ID)=="" { return fmt.Errorf("impact mechanism %d id is required",index) }
		if !validImpactChannel(item.Channel) { return fmt.Errorf("impact mechanism %d has invalid channel %q",index,item.Channel) }
		if !validImpactDirection(item.Direction) { return fmt.Errorf("impact mechanism %d has invalid direction %q",index,item.Direction) }
		if strings.TrimSpace(item.Rationale)=="" { return fmt.Errorf("impact mechanism %d rationale is required",index) }
		ids:=uniqueSortedStrings(item.SupportingFactIDs)
		if len(ids)==0 || !equalStringSlices(ids,item.SupportingFactIDs) { return fmt.Errorf("impact mechanism %d supporting fact ids must be non-empty, unique, and sorted",index) }
		for _,id:=range ids { if _,ok:=factIDs[id];!ok { return fmt.Errorf("impact mechanism %d references unknown event fact",index) } }
		expected:=deterministicImpactMechanismID(item.Channel,item.Direction,ids)
		if item.ID!=expected { return fmt.Errorf("impact mechanism %d id is not deterministic",index) }
		if _,dup:=seen[item.ID];dup { return fmt.Errorf("impact mechanism %d is duplicated",index) }
		seen[item.ID]=struct{}{}
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
