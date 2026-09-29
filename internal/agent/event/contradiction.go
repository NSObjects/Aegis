package event

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ContradictionType string

const (
	ContradictionValue ContradictionType =
		"VALUE_CONFLICT"
	ContradictionStatus ContradictionType =
		"STATUS_CONFLICT"
	ContradictionDate ContradictionType =
		"DATE_CONFLICT"
	ContradictionDirect ContradictionType =
		"DIRECT_CONTRADICTION"
)

// DetectedContradiction is semantic model output. Runtime validates and
// normalizes it before it becomes a ClaimContradiction.
type DetectedContradiction struct {
	Type      ContradictionType `json:"type"`
	FactIDs   []string          `json:"fact_ids"`
	Rationale string            `json:"rationale"`
}

type ContradictionDetectionResult struct {
	Contradictions []DetectedContradiction `json:"contradictions"`
}

// ClaimContradiction is an auditable unresolved relation between two grounded
// facts. It does not select a winning fact or assert which claim is true.
type ClaimContradiction struct {
	ID        string            `json:"id"`
	Type      ContradictionType `json:"type"`
	FactIDs   []string          `json:"fact_ids"`
	Rationale string            `json:"rationale"`
}

func normalizeContradictions(
	facts []Fact,
	detected []DetectedContradiction,
) ([]ClaimContradiction, error) {
	known := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		known[fact.ID] = struct{}{}
	}

	result := make(
		[]ClaimContradiction,
		0,
		len(detected),
	)
	seenPairs := make(map[string]struct{})
	for index, raw := range detected {
		if !validContradictionType(raw.Type) {
			return nil, fmt.Errorf(
				"contradiction %d has invalid type %q",
				index,
				raw.Type,
			)
		}
		if len(raw.FactIDs) != 2 {
			return nil, fmt.Errorf(
				"contradiction %d must reference exactly two fact ids",
				index,
			)
		}

		left := strings.TrimSpace(raw.FactIDs[0])
		right := strings.TrimSpace(raw.FactIDs[1])
		if left == "" || right == "" {
			return nil, fmt.Errorf(
				"contradiction %d fact ids are required",
				index,
			)
		}
		if left == right {
			return nil, fmt.Errorf(
				"contradiction %d must reference two distinct facts",
				index,
			)
		}
		if _, ok := known[left]; !ok {
			return nil, fmt.Errorf(
				"contradiction %d references unknown fact %q",
				index,
				left,
			)
		}
		if _, ok := known[right]; !ok {
			return nil, fmt.Errorf(
				"contradiction %d references unknown fact %q",
				index,
				right,
			)
		}
		if right < left {
			left, right = right, left
		}

		rationale := strings.TrimSpace(raw.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf(
				"contradiction %d rationale is required",
				index,
			)
		}

		pairKey := left + "\x00" + right
		if _, duplicate := seenPairs[pairKey]; duplicate {
			continue
		}
		seenPairs[pairKey] = struct{}{}

		item := ClaimContradiction{
			Type:      raw.Type,
			FactIDs:   []string{left, right},
			Rationale: rationale,
		}
		item.ID = deterministicContradictionID(item)
		result = append(result, item)
	}

	sort.Slice(
		result,
		func(i int, j int) bool {
			return result[i].ID < result[j].ID
		},
	)
	return result, nil
}

func deterministicContradictionID(
	value ClaimContradiction,
) string {
	if len(value.FactIDs) != 2 {
		return ""
	}
	return shortHashID(
		"conflict",
		string(value.Type)+"\x00"+
			value.FactIDs[0]+"\x00"+
			value.FactIDs[1],
	)
}

func validateContradictions(
	facts []Fact,
	values []ClaimContradiction,
) error {
	normalized := make(
		[]DetectedContradiction,
		0,
		len(values),
	)
	for _, value := range values {
		normalized = append(
			normalized,
			DetectedContradiction{
				Type:      value.Type,
				FactIDs:   value.FactIDs,
				Rationale: value.Rationale,
			},
		)
	}
	expected, err := normalizeContradictions(
		facts,
		normalized,
	)
	if err != nil {
		return err
	}
	if len(expected) != len(values) {
		return errors.New(
			"contradictions must not contain duplicate fact pairs",
		)
	}
	for index := range values {
		if values[index].ID != expected[index].ID ||
			values[index].Type != expected[index].Type ||
			!equalStringSlices(
				values[index].FactIDs,
				expected[index].FactIDs,
			) ||
			strings.TrimSpace(values[index].Rationale) !=
				expected[index].Rationale {
			return errors.New(
				"contradictions are not deterministically normalized",
			)
		}
	}
	return nil
}

func validContradictionType(
	value ContradictionType,
) bool {
	switch value {
	case ContradictionValue,
		ContradictionStatus,
		ContradictionDate,
		ContradictionDirect:
		return true
	default:
		return false
	}
}
