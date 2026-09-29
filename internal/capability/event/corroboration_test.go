package event

import (
	"testing"
	"time"
)

func TestEvidenceCorroboratorExactReprintAcrossSources(
	t *testing.T,
) {
	documents := []CanonicalDocument{
		corroborationTestDocument(
			"doc-official",
			"cninfo.com.cn",
			"same-content",
		),
		corroborationTestDocument(
			"doc-media",
			"cls.cn",
			"same-content",
		),
	}

	result := (EvidenceCorroborator{}).Corroborate(
		documents,
	)

	if len(result.Clusters) != 1 {
		t.Fatalf(
			"clusters = %d, want 1: %+v",
			len(result.Clusters),
			result.Clusters,
		)
	}
	cluster := result.Clusters[0]
	if cluster.Kind != EvidenceClusterExactReprint {
		t.Fatalf(
			"kind = %q, want %q",
			cluster.Kind,
			EvidenceClusterExactReprint,
		)
	}
	if result.IndependentSourceCount != 1 {
		t.Fatalf(
			"independent sources = %d, want 1",
			result.IndependentSourceCount,
		)
	}
	if len(cluster.DocumentIDs) != 2 ||
		len(cluster.SourceKeys) != 2 {
		t.Fatalf(
			"cluster = %+v",
			cluster,
		)
	}
}

func TestEvidenceCorroboratorSameSourceDoesNotInflateIndependence(
	t *testing.T,
) {
	documents := []CanonicalDocument{
		corroborationTestDocument(
			"doc-1",
			"cls.cn",
			"content-1",
		),
		corroborationTestDocument(
			"doc-2",
			"cls.cn",
			"content-2",
		),
	}

	result := (EvidenceCorroborator{}).Corroborate(
		documents,
	)

	if len(result.Clusters) != 1 ||
		result.Clusters[0].Kind !=
			EvidenceClusterSameSource {
		t.Fatalf(
			"corroboration = %+v",
			result,
		)
	}
	if result.IndependentSourceCount != 1 {
		t.Fatalf(
			"independent sources = %d, want 1",
			result.IndependentSourceCount,
		)
	}
}

func TestEvidenceCorroboratorDifferentSourcesAndContentRemainIndependent(
	t *testing.T,
) {
	documents := []CanonicalDocument{
		corroborationTestDocument(
			"doc-1",
			"cninfo.com.cn",
			"content-1",
		),
		corroborationTestDocument(
			"doc-2",
			"cls.cn",
			"content-2",
		),
	}

	result := (EvidenceCorroborator{}).Corroborate(
		documents,
	)

	if len(result.Clusters) != 2 {
		t.Fatalf(
			"clusters = %d, want 2: %+v",
			len(result.Clusters),
			result.Clusters,
		)
	}
	if result.IndependentSourceCount != 2 {
		t.Fatalf(
			"independent sources = %d, want 2",
			result.IndependentSourceCount,
		)
	}
}

func TestEvidenceCorroboratorUsesTransitiveEvidenceGroups(
	t *testing.T,
) {
	documents := []CanonicalDocument{
		corroborationTestDocument(
			"doc-a-original",
			"a.example",
			"shared-content",
		),
		corroborationTestDocument(
			"doc-b-reprint",
			"b.example",
			"shared-content",
		),
		corroborationTestDocument(
			"doc-b-followup",
			"b.example",
			"different-content",
		),
	}

	result := (EvidenceCorroborator{}).Corroborate(
		documents,
	)

	if len(result.Clusters) != 1 {
		t.Fatalf(
			"clusters = %d, want 1: %+v",
			len(result.Clusters),
			result.Clusters,
		)
	}
	if result.Clusters[0].Kind != EvidenceClusterMixed {
		t.Fatalf(
			"kind = %q, want %q",
			result.Clusters[0].Kind,
			EvidenceClusterMixed,
		)
	}
	if result.IndependentSourceCount != 1 {
		t.Fatalf(
			"independent sources = %d, want 1",
			result.IndependentSourceCount,
		)
	}
}

func TestEvidenceCorroboratorUnknownSourceDoesNotInflateCount(
	t *testing.T,
) {
	known := corroborationTestDocument(
		"doc-known",
		"cls.cn",
		"known-content",
	)
	unknown := CanonicalDocument{
		ID:          "doc-unknown",
		ContentHash: "unknown-content",
	}

	result := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{known, unknown},
	)

	if len(result.Clusters) != 2 {
		t.Fatalf(
			"clusters = %d, want 2",
			len(result.Clusters),
		)
	}
	if result.IndependentSourceCount != 1 {
		t.Fatalf(
			"independent sources = %d, want 1",
			result.IndependentSourceCount,
		)
	}
}

func TestEvidenceCorroboratorOriginCandidatePrefersPrimaryAuthority(
	t *testing.T,
) {
	published := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)

	media := corroborationTestDocument(
		"doc-media",
		"cls.cn",
		"same-content",
	)
	media.Source.Authority = AuthorityMedia
	media.PublishedAt = cloneTime(&published)

	official := corroborationTestDocument(
		"doc-official",
		"cninfo.com.cn",
		"same-content",
	)
	official.Source.IsPrimary = true
	official.Source.Authority = AuthorityIssuer
	later := published.Add(time.Hour)
	official.PublishedAt = cloneTime(&later)

	result := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{media, official},
	)

	if len(result.Clusters) != 1 {
		t.Fatalf(
			"clusters = %+v",
			result.Clusters,
		)
	}
	if result.Clusters[0].OriginDocumentID !=
		"doc-official" {
		t.Fatalf(
			"origin = %q, want doc-official",
			result.Clusters[0].OriginDocumentID,
		)
	}
}

func TestEvidenceCorroboratorDeterministicAcrossInputOrder(
	t *testing.T,
) {
	first := corroborationTestDocument(
		"doc-a",
		"a.example",
		"same-content",
	)
	second := corroborationTestDocument(
		"doc-b",
		"b.example",
		"same-content",
	)

	left := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{first, second},
	)
	right := (EvidenceCorroborator{}).Corroborate(
		[]CanonicalDocument{second, first},
	)

	if len(left.Clusters) != 1 ||
		len(right.Clusters) != 1 {
		t.Fatalf(
			"left=%+v right=%+v",
			left,
			right,
		)
	}
	if left.Clusters[0].ClusterID !=
		right.Clusters[0].ClusterID {
		t.Fatalf(
			"cluster IDs differ: %q vs %q",
			left.Clusters[0].ClusterID,
			right.Clusters[0].ClusterID,
		)
	}
	if left.Clusters[0].OriginDocumentID !=
		right.Clusters[0].OriginDocumentID {
		t.Fatalf(
			"origins differ: %q vs %q",
			left.Clusters[0].OriginDocumentID,
			right.Clusters[0].OriginDocumentID,
		)
	}
}

func TestTaskEvaluatorConsumesProvidedCorroboration(
	t *testing.T,
) {
	task := ResearchTask{
		ID: "corroboration-evaluator",
		Requirements: ResearchRequirements{
			MinDocuments:          2,
			MinIndependentSources: 2,
		},
	}
	documents := []CanonicalDocument{
		corroborationTestDocument(
			"doc-1",
			"a.example",
			"content-1",
		),
		corroborationTestDocument(
			"doc-2",
			"b.example",
			"content-2",
		),
	}
	corroboration := CorroborationResult{
		IndependentSourceCount: 1,
	}

	result := (TaskEvaluator{
		Corroboration: &corroboration,
	}).Evaluate(
		task,
		documents,
		nil,
	)

	if result.Status != TaskPartial {
		t.Fatalf(
			"status = %q, want %q",
			result.Status,
			TaskPartial,
		)
	}
	if !hasGap(
		result.Gaps,
		GapInsufficientIndependentSources,
	) {
		t.Fatalf(
			"gaps = %+v",
			result.Gaps,
		)
	}
}

func corroborationTestDocument(
	id string,
	domain string,
	contentHash string,
) CanonicalDocument {
	return CanonicalDocument{
		ID:          id,
		ContentHash: contentHash,
		Source: DocumentSource{
			Publisher:       domain,
			URL:             "https://" + domain + "/article/" + id,
			CanonicalDomain: domain,
		},
	}
}
