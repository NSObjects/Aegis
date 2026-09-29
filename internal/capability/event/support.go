package event

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type EvidenceSupportClass string

const (
	EvidenceSupportUnknown             EvidenceSupportClass = "UNKNOWN"
	EvidenceSupportSingleSource        EvidenceSupportClass = "SINGLE_SOURCE"
	EvidenceSupportPrimaryOnly         EvidenceSupportClass = "PRIMARY_ONLY"
	EvidenceSupportCorroborated        EvidenceSupportClass = "CORROBORATED"
	EvidenceSupportPrimaryCorroborated EvidenceSupportClass = "PRIMARY_CORROBORATED"
)

type EvidenceSupport struct {
	DocumentCount            int
	IndependentEvidenceCount int
	IndependentSourceCount   int
	PrimarySourcePresent     bool
	Authorities              []EvidenceAuthority
	EvidenceKinds            []EvidenceKind
	CollapsedReprintCount    int
	UnknownSourceCount       int
	SupportClass             EvidenceSupportClass
}

type EvidenceSupportAnalyzer struct{}

func (EvidenceSupportAnalyzer) Analyze(
	evidenceDocumentIDs []string,
	documents []CanonicalDocument,
	corroboration CorroborationResult,
) (EvidenceSupport, error) {
	evidenceIDs := uniqueSupportDocumentIDs(
		evidenceDocumentIDs,
	)
	if len(evidenceIDs) == 0 {
		return EvidenceSupport{}, errors.New(
			"evidence support requires at least one document id",
		)
	}

	documentByID := make(
		map[string]CanonicalDocument,
		len(documents),
	)
	for _, document := range documents {
		documentID := strings.TrimSpace(document.ID)
		if documentID == "" {
			return EvidenceSupport{}, errors.New(
				"evidence support canonical document id is required",
			)
		}
		if _, duplicate := documentByID[documentID]; duplicate {
			return EvidenceSupport{}, fmt.Errorf(
				"evidence support canonical document id %q is duplicated",
				documentID,
			)
		}
		documentByID[documentID] = document
	}

	clusterByDocumentID := make(
		map[string]EvidenceCluster,
		len(documents),
	)
	for _, cluster := range corroboration.Clusters {
		for _, rawDocumentID := range cluster.DocumentIDs {
			documentID := strings.TrimSpace(rawDocumentID)
			if documentID == "" {
				continue
			}
			if existing, duplicate :=
				clusterByDocumentID[documentID]; duplicate &&
				existing.ClusterID != cluster.ClusterID {
				return EvidenceSupport{}, fmt.Errorf(
					"evidence support document %q belongs to multiple corroboration clusters",
					documentID,
				)
			}
			clusterByDocumentID[documentID] = cluster
		}
	}

	selectedByCluster := make(
		map[string][]CanonicalDocument,
	)
	authorities := make(
		map[EvidenceAuthority]struct{},
	)
	evidenceKinds := make(
		map[EvidenceKind]struct{},
	)
	primaryPresent := false

	for _, documentID := range evidenceIDs {
		document, ok := documentByID[documentID]
		if !ok {
			return EvidenceSupport{}, fmt.Errorf(
				"evidence support references unknown canonical document %q",
				documentID,
			)
		}
		cluster, ok := clusterByDocumentID[documentID]
		if !ok || strings.TrimSpace(cluster.ClusterID) == "" {
			return EvidenceSupport{}, fmt.Errorf(
				"evidence support document %q has no corroboration cluster",
				documentID,
			)
		}

		selectedByCluster[cluster.ClusterID] = append(
			selectedByCluster[cluster.ClusterID],
			document,
		)
		if document.Source.Authority != "" {
			authorities[document.Source.Authority] = struct{}{}
		}
		if document.Source.EvidenceKind != "" {
			evidenceKinds[document.Source.EvidenceKind] = struct{}{}
		}
		primaryPresent = primaryPresent ||
			document.Source.IsPrimary
	}

	sourceSet := make(map[string]struct{})
	unknownSources := 0
	for _, selected := range selectedByCluster {
		representative := selected[0]
		for _, candidate := range selected[1:] {
			if corroborationDocumentLess(
				candidate,
				representative,
			) {
				representative = candidate
			}
		}

		sourceKey := corroborationSourceKey(
			representative.Source,
		)
		if sourceKey == "" {
			unknownSources++
			continue
		}
		sourceSet[sourceKey] = struct{}{}
	}

	authorityValues := make(
		[]EvidenceAuthority,
		0,
		len(authorities),
	)
	for authority := range authorities {
		authorityValues = append(
			authorityValues,
			authority,
		)
	}
	sort.Slice(
		authorityValues,
		func(i int, j int) bool {
			leftRank := corroborationAuthorityRank(
				authorityValues[i],
			)
			rightRank := corroborationAuthorityRank(
				authorityValues[j],
			)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			return authorityValues[i] <
				authorityValues[j]
		},
	)

	evidenceKindValues := make(
		[]EvidenceKind,
		0,
		len(evidenceKinds),
	)
	for kind := range evidenceKinds {
		evidenceKindValues = append(
			evidenceKindValues,
			kind,
		)
	}
	sort.Slice(
		evidenceKindValues,
		func(i int, j int) bool {
			return evidenceKindValues[i] <
				evidenceKindValues[j]
		},
	)

	independentEvidenceCount := len(selectedByCluster)
	independentSourceCount := len(sourceSet)

	return EvidenceSupport{
		DocumentCount:            len(evidenceIDs),
		IndependentEvidenceCount: independentEvidenceCount,
		IndependentSourceCount:   independentSourceCount,
		PrimarySourcePresent:     primaryPresent,
		Authorities:              authorityValues,
		EvidenceKinds:            evidenceKindValues,
		CollapsedReprintCount: len(evidenceIDs) -
			independentEvidenceCount,
		UnknownSourceCount: unknownSources,
		SupportClass: evidenceSupportClass(
			primaryPresent,
			independentSourceCount,
		),
	}, nil
}

func evidenceSupportClass(
	primaryPresent bool,
	independentSourceCount int,
) EvidenceSupportClass {
	switch {
	case primaryPresent &&
		independentSourceCount >= 2:
		return EvidenceSupportPrimaryCorroborated
	case primaryPresent:
		return EvidenceSupportPrimaryOnly
	case independentSourceCount >= 2:
		return EvidenceSupportCorroborated
	case independentSourceCount == 1:
		return EvidenceSupportSingleSource
	default:
		return EvidenceSupportUnknown
	}
}

func uniqueSupportDocumentIDs(
	values []string,
) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}

	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
