package event

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"
)

type EvidenceClusterKind string

const (
	EvidenceClusterSingle       EvidenceClusterKind = "single"
	EvidenceClusterSameSource   EvidenceClusterKind = "same_source"
	EvidenceClusterExactReprint EvidenceClusterKind = "exact_reprint"
	EvidenceClusterNearReprint  EvidenceClusterKind = "near_reprint"
	EvidenceClusterMixed        EvidenceClusterKind = "mixed"
)

type EvidenceCluster struct {
	ClusterID              string
	Kind                   EvidenceClusterKind
	DocumentIDs            []string
	SourceKeys             []string
	OriginDocumentID       string
	IndependentSourceCount int
	Reason                 string
}

type CorroborationResult struct {
	Clusters               []EvidenceCluster
	Relations              []EvidenceRelation
	IndependentSourceCount int
}

type EvidenceCorroborator struct {
	NearDuplicateMatcher NearDuplicateMatcher
}

func (c EvidenceCorroborator) Corroborate(
	documents []CanonicalDocument,
) CorroborationResult {
	if len(documents) == 0 {
		return CorroborationResult{
			Clusters:  make([]EvidenceCluster, 0),
			Relations: make([]EvidenceRelation, 0),
		}
	}

	parent := make([]int, len(documents))
	for index := range parent {
		parent[index] = index
	}

	prepared := make(
		[]nearDuplicatePrepared,
		len(documents),
	)
	for index, document := range documents {
		prepared[index] =
			c.NearDuplicateMatcher.prepare(document)
	}

	relations := make(
		[]EvidenceRelation,
		0,
	)
	for left := 0; left < len(documents); left++ {
		for right := left + 1;
			right < len(documents);
			right++ {
			relation, matched :=
				c.NearDuplicateMatcher.matchPrepared(
					prepared[left],
					prepared[right],
				)
			if !matched {
				continue
			}
			relations = append(
				relations,
				relation,
			)
			corroborationUnion(
				parent,
				left,
				right,
			)
		}
	}
	sortEvidenceRelations(relations)

	membersByRoot := make(map[int][]int)
	for index := range documents {
		root := corroborationFind(parent, index)
		membersByRoot[root] = append(
			membersByRoot[root],
			index,
		)
	}

	clusters := make(
		[]EvidenceCluster,
		0,
		len(membersByRoot),
	)
	creditedSources := make(map[string]struct{})

	for _, members := range membersByRoot {
		cluster := buildEvidenceCluster(
			documents,
			members,
			relations,
		)
		clusters = append(clusters, cluster)

		if cluster.OriginDocumentID == "" {
			continue
		}
		for _, index := range members {
			if strings.TrimSpace(documents[index].ID) !=
				cluster.OriginDocumentID {
				continue
			}
			sourceKey := corroborationSourceKey(
				documents[index].Source,
			)
			if sourceKey != "" {
				creditedSources[sourceKey] = struct{}{}
			}
			break
		}
	}

	sort.SliceStable(
		clusters,
		func(i int, j int) bool {
			return clusters[i].ClusterID <
				clusters[j].ClusterID
		},
	)

	return CorroborationResult{
		Clusters:               clusters,
		Relations:              relations,
		IndependentSourceCount: len(creditedSources),
	}
}
func buildEvidenceCluster(
	documents []CanonicalDocument,
	members []int,
	relations []EvidenceRelation,
) EvidenceCluster {
	documentIDs := make([]string, 0, len(members))
	sourceSet := make(map[string]struct{})

	originIndex := members[0]
	for _, index := range members {
		document := documents[index]
		documentID := strings.TrimSpace(document.ID)
		if documentID != "" {
			documentIDs = append(
				documentIDs,
				documentID,
			)
		}

		sourceKey := corroborationSourceKey(
			document.Source,
		)
		if sourceKey != "" {
			sourceSet[sourceKey] = struct{}{}
		}

		if corroborationDocumentLess(
			document,
			documents[originIndex],
		) {
			originIndex = index
		}
	}

	sort.Strings(documentIDs)
	sourceKeys := make(
		[]string,
		0,
		len(sourceSet),
	)
	for sourceKey := range sourceSet {
		sourceKeys = append(sourceKeys, sourceKey)
	}
	sort.Strings(sourceKeys)

	kind, reason := corroborationClusterSemantics(
		documentIDs,
		len(sourceSet),
		relations,
	)

	independent := 0
	if corroborationSourceKey(
		documents[originIndex].Source,
	) != "" {
		independent = 1
	}

	return EvidenceCluster{
		ClusterID: corroborationClusterID(
			documentIDs,
		),
		Kind:                   kind,
		DocumentIDs:            documentIDs,
		SourceKeys:             sourceKeys,
		OriginDocumentID:       strings.TrimSpace(documents[originIndex].ID),
		IndependentSourceCount: independent,
		Reason:                 reason,
	}
}

func corroborationClusterSemantics(
	documentIDs []string,
	sourceCount int,
	relations []EvidenceRelation,
) (EvidenceClusterKind, string) {
	if len(documentIDs) <= 1 {
		return EvidenceClusterSingle,
			"single canonical document"
	}

	memberSet := make(
		map[string]struct{},
		len(documentIDs),
	)
	for _, documentID := range documentIDs {
		memberSet[documentID] = struct{}{}
	}

	hasExact := false
	hasNear := false
	for _, relation := range relations {
		if _, ok := memberSet[
			relation.LeftDocumentID,
		]; !ok {
			continue
		}
		if _, ok := memberSet[
			relation.RightDocumentID,
		]; !ok {
			continue
		}
		switch relation.Kind {
		case EvidenceRelationNearReprint:
			hasNear = true
		case EvidenceRelationExactReprint:
			hasExact = true
		}
	}

	switch {
	case hasNear && hasExact:
		return EvidenceClusterNearReprint,
			"documents are connected by exact and deterministic near-reprint relations"
	case hasNear:
		return EvidenceClusterNearReprint,
			"documents are connected by deterministic near-reprint relations"
	case hasExact && sourceCount > 1:
		return EvidenceClusterExactReprint,
			"documents from multiple source identities share an exact non-empty content hash"
	case hasExact && sourceCount == 1:
		return EvidenceClusterSameSource,
			"same-source documents share an exact non-empty content hash"
	default:
		return EvidenceClusterMixed,
			"documents are connected by deterministic corroboration relations"
	}
}
func corroborationClusterID(
	documentIDs []string,
) string {
	sum := sha256.Sum256(
		[]byte(strings.Join(documentIDs, "\x00")),
	)
	return "cluster_" +
		hex.EncodeToString(sum[:8])
}

func corroborationSourceKey(
	source DocumentSource,
) string {
	if domain := strings.ToLower(
		strings.TrimSpace(
			source.CanonicalDomain,
		),
	); domain != "" {
		return "domain:" + domain
	}

	if publisher := strings.ToLower(
		strings.TrimSpace(source.Publisher),
	); publisher != "" {
		return "publisher:" + publisher
	}

	if host := corroborationURLHost(
		source.URL,
	); host != "" {
		return "host:" + host
	}

	return ""
}

func corroborationURLHost(
	rawURL string,
) string {
	parsed, err := url.Parse(
		strings.TrimSpace(rawURL),
	)
	if err != nil {
		return ""
	}
	return strings.ToLower(
		strings.TrimSpace(parsed.Hostname()),
	)
}

func corroborationFind(
	parent []int,
	index int,
) int {
	if parent[index] != index {
		parent[index] = corroborationFind(
			parent,
			parent[index],
		)
	}
	return parent[index]
}

func corroborationUnion(
	parent []int,
	left int,
	right int,
) {
	leftRoot := corroborationFind(parent, left)
	rightRoot := corroborationFind(parent, right)
	if leftRoot == rightRoot {
		return
	}
	if leftRoot < rightRoot {
		parent[rightRoot] = leftRoot
		return
	}
	parent[leftRoot] = rightRoot
}

func corroborationDocumentLess(
	left CanonicalDocument,
	right CanonicalDocument,
) bool {
	leftKnown := corroborationSourceKey(
		left.Source,
	) != ""
	rightKnown := corroborationSourceKey(
		right.Source,
	) != ""
	if leftKnown != rightKnown {
		return leftKnown
	}

	leftPrimary := left.Source.IsPrimary
	rightPrimary := right.Source.IsPrimary
	if leftPrimary != rightPrimary {
		return leftPrimary
	}

	leftAuthority := corroborationAuthorityRank(
		left.Source.Authority,
	)
	rightAuthority := corroborationAuthorityRank(
		right.Source.Authority,
	)
	if leftAuthority != rightAuthority {
		return leftAuthority < rightAuthority
	}

	leftTime := corroborationDocumentTime(left)
	rightTime := corroborationDocumentTime(right)
	if !leftTime.Equal(rightTime) {
		if leftTime.IsZero() {
			return false
		}
		if rightTime.IsZero() {
			return true
		}
		return leftTime.Before(rightTime)
	}

	return strings.TrimSpace(left.ID) <
		strings.TrimSpace(right.ID)
}

func corroborationAuthorityRank(
	authority EvidenceAuthority,
) int {
	switch authority {
	case AuthorityIssuer:
		return 0
	case AuthorityExchange:
		return 1
	case AuthorityRegulator:
		return 2
	case AuthorityMedia:
		return 3
	case AuthorityResearch:
		return 4
	case AuthoritySocial:
		return 5
	default:
		return 6
	}
}

func corroborationDocumentTime(
	document CanonicalDocument,
) time.Time {
	switch {
	case document.AvailableAt != nil:
		return document.AvailableAt.UTC()
	case document.PublishedAt != nil:
		return document.PublishedAt.UTC()
	case document.FirstSeenAt != nil:
		return document.FirstSeenAt.UTC()
	default:
		return time.Time{}
	}
}
