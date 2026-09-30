package event

import (
	capevent "Aegis/internal/capability/event"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeContradictionsDerivesIndependentProvenance(
	t *testing.T,
) {
	facts := []Fact{
		groundedContradictionFact(
			"fact_a",
			"doc_a",
		"合同金额为10亿元。",
		),
		groundedContradictionFact(
			"fact_b",
			"doc_b",
			"合同金额为12亿元。",
		),
	}
	corroboration := capevent.CorroborationResult{
		Clusters: []capevent.EvidenceCluster{
			{
				ClusterID:   "cluster_a",
				DocumentIDs: []string{"doc_a"},
				SourceKeys:  []string{"domain:a.example"},
			},
			{
				ClusterID:   "cluster_b",
				DocumentIDs: []string{"doc_b"},
				SourceKeys:  []string{"domain:b.example"},
			},
		},
	}

	got, err := normalizeContradictions(
		[]DetectedContradiction{
			{
				LeftFactID:  "fact_b",
				RightFactID: "fact_a",
				Kind:        ContradictionValue,
				Rationale:   "same contract, incompatible amounts",
			},
		},
		facts,
		corroboration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("contradictions = %d, want 1", len(got))
	}
	item := got[0]
	if item.LeftFactID != "fact_a" ||
		item.RightFactID != "fact_b" {
		t.Fatalf("fact order = %+v", item)
	}
	if !item.IndependentEvidenceSides ||
		!item.IndependentSourceSides {
		t.Fatalf("independence = %+v", item)
	}
	if !reflect.DeepEqual(
		item.EvidenceDocumentIDs,
		[]string{"doc_a", "doc_b"},
	) {
		t.Fatalf(
			"evidence ids = %v",
			item.EvidenceDocumentIDs,
		)
	}
}

func TestNormalizeContradictionsCollapsesReversedDuplicate(
	t *testing.T,
) {
	facts := []Fact{
		groundedContradictionFact(
			"fact_a", "doc_a", "已完成",
		),
		groundedContradictionFact(
			"fact_b", "doc_b", "尚未完成",
		),
	}
	corroboration := capevent.CorroborationResult{
		Clusters: []capevent.EvidenceCluster{
			{
				ClusterID:   "cluster_a",
				DocumentIDs: []string{"doc_a"},
				SourceKeys:  []string{"domain:a.example"},
			},
			{
				ClusterID:   "cluster_b",
				DocumentIDs: []string{"doc_b"},
				SourceKeys:  []string{"domain:b.example"},
			},
		},
	}
	got, err := normalizeContradictions(
		[]DetectedContradiction{
			{
				LeftFactID:  "fact_a",
				RightFactID: "fact_b",
				Kind:        ContradictionStatus,
				Rationale:   "status conflict",
			},
			{
				LeftFactID:  "fact_b",
				RightFactID: "fact_a",
				Kind:        ContradictionStatus,
				Rationale:   "same conflict reversed",
			},
		},
		facts,
		corroboration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("contradictions = %d, want 1", len(got))
	}
}

func TestNormalizeContradictionsRejectsInvalidRelations(
	t *testing.T,
) {
	facts := []Fact{
		groundedContradictionFact(
			"fact_a", "doc_a", "fact A",
		),
		groundedContradictionFact(
			"fact_b", "doc_b", "fact B",
		),
	}
	corroboration := capevent.CorroborationResult{
		Clusters: []capevent.EvidenceCluster{
			{
				ClusterID:   "cluster_a",
				DocumentIDs: []string{"doc_a"},
				SourceKeys:  []string{"domain:a.example"},
			},
			{
				ClusterID:   "cluster_b",
				DocumentIDs: []string{"doc_b"},
				SourceKeys:  []string{"domain:b.example"},
			},
		},
	}
	cases := []struct {
		name string
		item DetectedContradiction
		want string
	}{
		{
			name: "self",
			item: DetectedContradiction{
				LeftFactID:  "fact_a",
				RightFactID: "fact_a",
				Kind:        ContradictionStatus,
				Rationale:   "x",
			},
			want: "same fact",
		},
		{
			name: "outside cluster",
			item: DetectedContradiction{
				LeftFactID:  "fact_a",
				RightFactID: "fact_missing",
				Kind:        ContradictionStatus,
				Rationale:   "x",
			},
			want: "outside the event cluster",
		},
		{
			name: "kind",
			item: DetectedContradiction{
				LeftFactID:  "fact_a",
				RightFactID: "fact_b",
				Kind:        "TRUTH",
				Rationale:   "x",
			},
			want: "invalid kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeContradictions(
				[]DetectedContradiction{tc.item},
				facts,
				corroboration,
			)
			if err == nil ||
				!strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestNormalizeContradictionsDetectsSharedReprintCluster(
	t *testing.T,
) {
	facts := []Fact{
		groundedContradictionFact(
			"fact_a", "doc_a", "value A",
		),
		groundedContradictionFact(
			"fact_b", "doc_b", "value B",
		),
	}
	corroboration := capevent.CorroborationResult{
		Clusters: []capevent.EvidenceCluster{
			{
				ClusterID: "cluster_shared",
				DocumentIDs: []string{
					"doc_a",
					"doc_b",
				},
				SourceKeys: []string{
					"domain:a.example",
					"domain:b.example",
				},
			},
		},
	}
	got, err := normalizeContradictions(
		[]DetectedContradiction{
			{
				LeftFactID:  "fact_a",
				RightFactID: "fact_b",
				Kind:        ContradictionValue,
				Rationale:   "value conflict",
			},
		},
		facts,
		corroboration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].IndependentEvidenceSides {
		t.Fatalf(
			"shared reprint cluster must not be independent: %+v",
			got[0],
		)
	}
}

func groundedContradictionFact(
	id string,
	documentID string,
	quote string,
) Fact {
	return Fact{
		ID:       id,
		Type:     EventDisclosure,
		Headline: id,
		Summary:  quote,
		Groundings: []EvidenceGrounding{
			{
				DocumentID: documentID,
				Quote:      quote,
				StartByte:  0,
				EndByte:    len(quote),
			},
		},
		EvidenceDocumentIDs: []string{
			documentID,
		},
		Confidence: 0.9,
	}
}
