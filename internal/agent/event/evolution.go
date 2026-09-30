package event

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type FactRelationType string

const (
	FactRelationUpdates    FactRelationType = "UPDATES"
	FactRelationSupersedes FactRelationType = "SUPERSEDES"
	FactRelationFinalizes  FactRelationType = "FINALIZES"
	FactRelationRealizes   FactRelationType = "REALIZES"
)

type DetectedFactRelation struct {
	Type       FactRelationType `json:"type"`
	FromFactID string           `json:"from_fact_id"`
	ToFactID   string           `json:"to_fact_id"`
	Rationale  string           `json:"rationale"`
}

type FactEvolutionDetectionResult struct {
	Relations []DetectedFactRelation `json:"relations"`
}

type FactRelation struct {
	ID         string           `json:"id"`
	Type       FactRelationType `json:"type"`
	FromFactID string           `json:"from_fact_id"`
	ToFactID   string           `json:"to_fact_id"`
	Rationale  string           `json:"rationale"`
}

func normalizeFactRelations(
	detected []DetectedFactRelation,
	facts []Fact,
) ([]FactRelation, error) {
	factByID := make(map[string]Fact, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.ID) == "" || len(fact.Groundings) == 0 {
			return nil, errors.New("fact evolution detection requires grounded facts")
		}
		factByID[fact.ID] = fact
	}

	seen := make(map[string]int, len(detected))
	result := make([]FactRelation, 0, len(detected))
	for index, raw := range detected {
		fromID := strings.TrimSpace(raw.FromFactID)
		toID := strings.TrimSpace(raw.ToFactID)
		if fromID == "" || toID == "" {
			return nil, fmt.Errorf("fact relation %d requires both fact ids", index)
		}
		if fromID == toID {
			return nil, fmt.Errorf("fact relation %d cannot reference the same fact twice", index)
		}
		if _, ok := factByID[fromID]; !ok {
			return nil, fmt.Errorf("fact relation %d references fact outside the event cluster", index)
		}
		if _, ok := factByID[toID]; !ok {
			return nil, fmt.Errorf("fact relation %d references fact outside the event cluster", index)
		}
		if !validFactRelationType(raw.Type) {
			return nil, fmt.Errorf("fact relation %d has invalid type %q", index, raw.Type)
		}
		rationale := strings.TrimSpace(raw.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf("fact relation %d rationale is required", index)
		}

		// Direction is semantic: from is the later/evolving claim and to is
		// the earlier claim. Never canonicalize by swapping endpoints.
		key := string(raw.Type) + "\x00" + fromID + "\x00" + toID
		if existingIndex, duplicate := seen[key]; duplicate {
			if rationale < result[existingIndex].Rationale {
				result[existingIndex].Rationale = rationale
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, FactRelation{
			ID: deterministicFactRelationID(raw.Type, fromID, toID),
			Type: raw.Type,
			FromFactID: fromID,
			ToFactID: toID,
			Rationale: rationale,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].FromFactID != result[j].FromFactID {
			return result[i].FromFactID < result[j].FromFactID
		}
		if result[i].ToFactID != result[j].ToFactID {
			return result[i].ToFactID < result[j].ToFactID
		}
		return result[i].Type < result[j].Type
	})
	return result, nil
}

func deterministicFactRelationID(
	relationType FactRelationType,
	fromFactID string,
	toFactID string,
) string {
	sum := sha256.Sum256([]byte(
		string(relationType) + "\x00" + fromFactID + "\x00" + toFactID,
	))
	return "relation_" + hex.EncodeToString(sum[:8])
}

func validFactRelationType(value FactRelationType) bool {
	switch value {
	case FactRelationUpdates,
		FactRelationSupersedes,
		FactRelationFinalizes,
		FactRelationRealizes:
		return true
	default:
		return false
	}
}
