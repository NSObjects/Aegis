package event

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeFactRelationsPreservesDirectionAndDeterminism(t *testing.T) {
	facts := []Fact{
		groundedEvolutionFact("fact_old", "doc_old", "预计收入100亿元。"),
		groundedEvolutionFact("fact_new", "doc_new", "实际收入120亿元。"),
	}
	got, err := normalizeFactRelations([]DetectedFactRelation{
		{Type: FactRelationRealizes, FromFactID: "fact_new", ToFactID: "fact_old", Rationale: "actual result realizes prior forecast"},
		{Type: FactRelationRealizes, FromFactID: "fact_new", ToFactID: "fact_old", Rationale: "actual realizes forecast"},
	}, facts)
	if err != nil { t.Fatal(err) }
	if len(got) != 1 { t.Fatalf("relations=%d, want 1", len(got)) }
	if got[0].FromFactID != "fact_new" || got[0].ToFactID != "fact_old" {
		t.Fatalf("direction changed: %+v", got[0])
	}
	if got[0].ID != deterministicFactRelationID(FactRelationRealizes, "fact_new", "fact_old") {
		t.Fatalf("id=%q", got[0].ID)
	}
	if got[0].Rationale != "actual realizes forecast" {
		t.Fatalf("rationale=%q", got[0].Rationale)
	}
}

func TestNormalizeFactRelationsSortsDeterministically(t *testing.T) {
	facts := []Fact{
		groundedEvolutionFact("a", "doc_a", "A"),
		groundedEvolutionFact("b", "doc_b", "B"),
		groundedEvolutionFact("c", "doc_c", "C"),
	}
	got, err := normalizeFactRelations([]DetectedFactRelation{
		{Type: FactRelationUpdates, FromFactID: "c", ToFactID: "a", Rationale: "c updates a"},
		{Type: FactRelationFinalizes, FromFactID: "b", ToFactID: "a", Rationale: "b finalizes a"},
	}, facts)
	if err != nil { t.Fatal(err) }
	pairs := []string{got[0].FromFactID+"->"+got[0].ToFactID, got[1].FromFactID+"->"+got[1].ToFactID}
	if !reflect.DeepEqual(pairs, []string{"b->a", "c->a"}) {
		t.Fatalf("order=%v", pairs)
	}
}

func TestNormalizeFactRelationsRejectsInvalidRelations(t *testing.T) {
	facts := []Fact{
		groundedEvolutionFact("a", "doc_a", "A"),
		groundedEvolutionFact("b", "doc_b", "B"),
	}
	cases := []struct{
		name string
		item DetectedFactRelation
		want string
	}{
		{"self", DetectedFactRelation{Type: FactRelationUpdates, FromFactID:"a", ToFactID:"a", Rationale:"x"}, "same fact"},
		{"outside", DetectedFactRelation{Type: FactRelationUpdates, FromFactID:"missing", ToFactID:"a", Rationale:"x"}, "outside the event cluster"},
		{"type", DetectedFactRelation{Type:"TRUTH", FromFactID:"b", ToFactID:"a", Rationale:"x"}, "invalid type"},
		{"rationale", DetectedFactRelation{Type:FactRelationUpdates, FromFactID:"b", ToFactID:"a"}, "rationale is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeFactRelations([]DetectedFactRelation{tc.item}, facts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestNormalizeFactRelationsSupportsFinalizesAndRealizes(t *testing.T) {
	facts := []Fact{
		groundedEvolutionFact("prelim", "doc_p", "初步数据为100。"),
		groundedEvolutionFact("final", "doc_f", "最终数据为102。"),
		groundedEvolutionFact("forecast", "doc_g", "预计销量100。"),
		groundedEvolutionFact("actual", "doc_a", "实际销量98。"),
	}
	got, err := normalizeFactRelations([]DetectedFactRelation{
		{Type:FactRelationFinalizes, FromFactID:"final", ToFactID:"prelim", Rationale:"final result resolves preliminary result"},
		{Type:FactRelationRealizes, FromFactID:"actual", ToFactID:"forecast", Rationale:"actual result realizes forecast"},
	}, facts)
	if err != nil { t.Fatal(err) }
	if len(got) != 2 { t.Fatalf("relations=%+v", got) }
}

func groundedEvolutionFact(id, documentID, quote string) Fact {
	return Fact{
		ID:id, Type:EventDisclosure, Headline:id, Summary:quote,
		Groundings:[]EvidenceGrounding{{DocumentID:documentID, Quote:quote, StartByte:0, EndByte:len(quote)}},
		EvidenceDocumentIDs:[]string{documentID}, Confidence:0.9,
	}
}
