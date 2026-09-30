package event

import (
	capevent "Aegis/internal/capability/event"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ContradictionKind string

const (
	ContradictionValue    ContradictionKind = "VALUE"
	ContradictionStatus   ContradictionKind = "STATUS"
	ContradictionDate     ContradictionKind = "DATE"
	ContradictionQuantity ContradictionKind = "QUANTITY"
	ContradictionIdentity ContradictionKind = "IDENTITY"
	ContradictionOther    ContradictionKind = "OTHER"
)

type DetectedContradiction struct {
	LeftFactID  string            `json:"left_fact_id"`
	RightFactID string            `json:"right_fact_id"`
	Kind        ContradictionKind `json:"kind"`
	Rationale   string            `json:"rationale"`
}

type ContradictionDetectionResult struct {
	Contradictions []DetectedContradiction `json:"contradictions"`
}

type ClaimContradiction struct {
	LeftFactID              string            `json:"left_fact_id"`
	RightFactID             string            `json:"right_fact_id"`
	Kind                    ContradictionKind `json:"kind"`
	Rationale               string            `json:"rationale"`
	LeftEvidenceDocumentIDs []string          `json:"left_evidence_document_ids"`
	RightEvidenceDocumentIDs []string         `json:"right_evidence_document_ids"`
	EvidenceDocumentIDs     []string          `json:"evidence_document_ids"`
	IndependentEvidenceSides bool             `json:"independent_evidence_sides"`
	IndependentSourceSides   bool             `json:"independent_source_sides"`
}

func normalizeContradictions(
	detected []DetectedContradiction,
	facts []Fact,
	corroboration capevent.CorroborationResult,
) ([]ClaimContradiction, error) {
	factByID := make(map[string]Fact, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.ID) == "" ||
			len(fact.Groundings) == 0 {
			return nil, errors.New(
				"contradiction detection requires grounded facts",
			)
		}
		factByID[fact.ID] = fact
	}

	clusterByDocumentID := make(map[string]capevent.EvidenceCluster)
	for _, cluster := range corroboration.Clusters {
		for _, documentID := range cluster.DocumentIDs {
			clusterByDocumentID[
				strings.TrimSpace(documentID),
			] = cluster
		}
	}

	seen := make(map[string]struct{}, len(detected))
	result := make([]ClaimContradiction, 0, len(detected))
	for index, raw := range detected {
		leftID := strings.TrimSpace(raw.LeftFactID)
		rightID := strings.TrimSpace(raw.RightFactID)
		if leftID == "" || rightID == "" {
			return nil, fmt.Errorf(
				"contradiction %d requires both fact ids",
				index,
			)
		}
		if leftID == rightID {
			return nil, fmt.Errorf(
				"contradiction %d cannot reference the same fact twice",
				index,
			)
		}
		left, leftOK := factByID[leftID]
		right, rightOK := factByID[rightID]
		if !leftOK || !rightOK {
			return nil, fmt.Errorf(
				"contradiction %d references fact outside the event cluster",
				index,
			)
		}
		if !validContradictionKind(raw.Kind) {
			return nil, fmt.Errorf(
				"contradiction %d has invalid kind %q",
				index,
				raw.Kind,
			)
		}
		rationale := strings.TrimSpace(raw.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf(
				"contradiction %d rationale is required",
				index,
			)
		}

		if rightID < leftID {
			leftID, rightID = rightID, leftID
			left, right = right, left
		}
		key := leftID + "\x00" + rightID
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		leftEvidence := uniqueSortedStrings(
			left.EvidenceDocumentIDs,
		)
		rightEvidence := uniqueSortedStrings(
			right.EvidenceDocumentIDs,
		)
		allEvidence := uniqueSortedStrings(
			append(
				append([]string(nil), leftEvidence...),
				rightEvidence...,
			),
		)
		leftClusters, leftSources, err :=
			contradictionProvenanceSets(
				leftEvidence,
				clusterByDocumentID,
			)
		if err != nil {
			return nil, fmt.Errorf(
				"contradiction %d left provenance: %w",
				index,
				err,
			)
		}
		rightClusters, rightSources, err :=
			contradictionProvenanceSets(
				rightEvidence,
				clusterByDocumentID,
			)
		if err != nil {
			return nil, fmt.Errorf(
				"contradiction %d right provenance: %w",
				index,
				err,
			)
		}

		result = append(result, ClaimContradiction{
			LeftFactID:               leftID,
			RightFactID:              rightID,
			Kind:                     raw.Kind,
			Rationale:                rationale,
			LeftEvidenceDocumentIDs:  leftEvidence,
			RightEvidenceDocumentIDs: rightEvidence,
			EvidenceDocumentIDs:      allEvidence,
			IndependentEvidenceSides: disjointNonEmptySets(
				leftClusters,
				rightClusters,
			),
			IndependentSourceSides: disjointNonEmptySets(
				leftSources,
				rightSources,
			),
		})
	}

	sort.SliceStable(result, func(i int, j int) bool {
		if result[i].LeftFactID != result[j].LeftFactID {
			return result[i].LeftFactID < result[j].LeftFactID
		}
		if result[i].RightFactID != result[j].RightFactID {
			return result[i].RightFactID < result[j].RightFactID
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}

func contradictionProvenanceSets(
	documentIDs []string,
	clusterByDocumentID map[string]capevent.EvidenceCluster,
) (map[string]struct{}, map[string]struct{}, error) {
	clusters := make(map[string]struct{})
	sources := make(map[string]struct{})
	for _, documentID := range documentIDs {
		cluster, ok := clusterByDocumentID[documentID]
		if !ok || strings.TrimSpace(cluster.ClusterID) == "" {
			return nil, nil, fmt.Errorf(
				"document %q has no corroboration cluster",
				documentID,
			)
		}
		clusters[cluster.ClusterID] = struct{}{}
		for _, sourceKey := range cluster.SourceKeys {
			sourceKey = strings.TrimSpace(sourceKey)
			if sourceKey != "" {
				sources[sourceKey] = struct{}{}
			}
		}
	}
	return clusters, sources, nil
}

func disjointNonEmptySets(
	left map[string]struct{},
	right map[string]struct{},
) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	for value := range left {
		if _, overlap := right[value]; overlap {
			return false
		}
	}
	return true
}

func validContradictionKind(
	value ContradictionKind,
) bool {
	switch value {
	case ContradictionValue,
		ContradictionStatus,
		ContradictionDate,
		ContradictionQuantity,
		ContradictionIdentity,
		ContradictionOther:
		return true
	default:
		return false
	}
}
