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
	IndependentSourceCount int
}

type EvidenceCorroborator struct{}

func (EvidenceCorroborator) Corroborate(
	documents []CanonicalDocument,
) CorroborationResult {
	if len(documents) == 0 {
		return CorroborationResult{
			Clusters: make([]EvidenceCluster, 0),
		}
	}

	parent := make([]int, len(documents))
	for index := range parent {
		parent[index] = index
	}

	sourceOwner := make(map[string]int)
	contentOwner := make(map[string]int)

	for index, document := range documents {
		sourceKey := corroborationSourceKey(
			document.Source,
		)
		if sourceKey != "" {
			if owner, ok := sourceOwner[sourceKey]; ok {
				corroborationUnion(parent, index, owner)
			} else {
				sourceOwner[sourceKey] = index
			}
		}

		contentKey := strings.TrimSpace(
			document.ContentHash,
		)
		if contentKey != "" {
			if owner, ok := contentOwner[contentKey]; ok {
				corroborationUnion(parent, index, owner)
			} else {
				contentOwner[contentKey] = index
			}
		}
	}

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
	independentSourceCount := 0

	for _, members := range membersByRoot {
		cluster := buildEvidenceCluster(
			documents,
			members,
		)
		if cluster.IndependentSourceCount > 0 {
			independentSourceCount++
		}
		clusters = append(clusters, cluster)
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
		IndependentSourceCount: independentSourceCount,
	}
}

func buildEvidenceCluster(
	documents []CanonicalDocument,
	members []int,
) EvidenceCluster {
	documentIDs := make([]string, 0, len(members))
	sourceSet := make(map[string]struct{})
	contentSet := make(map[string]struct{})
	sourceDocumentCount := make(map[string]int)

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
			sourceDocumentCount[sourceKey]++
		}

		contentKey := strings.TrimSpace(
			document.ContentHash,
		)
		if contentKey != "" {
			contentSet[contentKey] = struct{}{}
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
		len(members),
		len(sourceSet),
		len(contentSet),
		sourceDocumentCount,
	)

	independent := 0
	if len(sourceSet) > 0 {
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
	memberCount int,
	sourceCount int,
	contentCount int,
	sourceDocumentCount map[string]int,
) (EvidenceClusterKind, string) {
	if memberCount <= 1 {
		return EvidenceClusterSingle,
			"single canonical document"
	}

	hasSameSource := false
	for _, count := range sourceDocumentCount {
		if count > 1 {
			hasSameSource = true
			break
		}
	}
	hasExactReprint := contentCount > 0 &&
		contentCount < memberCount

	switch {
	case hasSameSource && hasExactReprint:
		return EvidenceClusterMixed,
			"documents are connected by same-source provenance and exact-content reprints"
	case hasExactReprint:
		return EvidenceClusterExactReprint,
			"documents share an exact non-empty content hash across sources"
	case hasSameSource || sourceCount == 1:
		return EvidenceClusterSameSource,
			"documents resolve to the same source identity"
	default:
		return EvidenceClusterMixed,
			"documents are transitively connected by deterministic corroboration rules"
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
