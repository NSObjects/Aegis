package evidenceledger

import (
	"Aegis/internal/research/synthesis"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestReferenceAlignmentNeverClaimsSemanticEntailment(t *testing.T) {
	market := synthesis.Reference{Domain: synthesis.DomainMarket}
	technical := synthesis.Reference{Domain: synthesis.DomainTechnical}
	eventA := synthesis.Reference{Domain: synthesis.DomainEvent, EventID: "event-a"}
	eventB := synthesis.Reference{Domain: synthesis.DomainEvent, EventID: "event-b"}
	eventDomain := synthesis.Reference{Domain: synthesis.DomainEvent}

	tests := []struct {
		name string
		insight []synthesis.Reference
		argument []synthesis.Reference
		want ReferenceAlignment
	}{
		{"different domains", []synthesis.Reference{technical}, []synthesis.Reference{market}, AlignmentNoSharedReference},
		{"market domain", []synthesis.Reference{market}, []synthesis.Reference{market}, AlignmentSameDomainOnly},
		{"technical domain", []synthesis.Reference{technical}, []synthesis.Reference{technical}, AlignmentSameDomainOnly},
		{"same explicit event", []synthesis.Reference{eventA}, []synthesis.Reference{eventA}, AlignmentSameEventID},
		{"different explicit events", []synthesis.Reference{eventA}, []synthesis.Reference{eventB}, AlignmentNoSharedReference},
		{"event insight context only", []synthesis.Reference{eventDomain}, []synthesis.Reference{eventA}, AlignmentSameDomainOnly},
		{"event argument context only", []synthesis.Reference{eventA}, []synthesis.Reference{eventDomain}, AlignmentSameDomainOnly},
		{"both event context only", []synthesis.Reference{eventDomain}, []synthesis.Reference{eventDomain}, AlignmentSameDomainOnly},
		{"specific event wins over weaker shared domain", []synthesis.Reference{market,eventA}, []synthesis.Reference{market,eventA}, AlignmentSameEventID},
		{"mismatched event not rescued as shared domain", []synthesis.Reference{eventA}, []synthesis.Reference{eventB}, AlignmentNoSharedReference},
		{"unrelated event with market common", []synthesis.Reference{eventA,market}, []synthesis.Reference{eventB,market}, AlignmentSameDomainOnly},
		{"empty references", nil, []synthesis.Reference{market}, AlignmentNoSharedReference},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := alignReferences(tc.insight, tc.argument)
			if got != tc.want {
				t.Fatalf("alignReferences() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLedgerAuditsOnlyExplicitArgumentLinksInStableOrder(t *testing.T) {
	artifact,state,plan := ledgerFixture()
	plan.Opportunities[0].ArgumentIDs = []string{"bear-1", " bull-1 "}
	plan.Risks[0].ArgumentIDs = []string{"bear-1"}
	originalIDs := append([]string(nil), plan.Opportunities[0].ArgumentIDs...)

	got,err:=Build(artifact,state,plan)
	if err!=nil { t.Fatal(err) }
	if len(got.Insights)!=2 { t.Fatalf("unexpected insights: %+v",got.Insights) }
	wantOpportunity := []ArgumentReferenceAudit{
		{ArgumentID:"bear-1", ReferenceAlignment:AlignmentNoSharedReference},
		{ArgumentID:"bull-1", ReferenceAlignment:AlignmentSameDomainOnly},
	}
	wantRisk := []ArgumentReferenceAudit{{ArgumentID:"bear-1", ReferenceAlignment:AlignmentSameDomainOnly}}
	if !reflect.DeepEqual(got.Insights[0].ArgumentReferenceAudits,wantOpportunity) ||
		!reflect.DeepEqual(got.Insights[1].ArgumentReferenceAudits,wantRisk) {
		t.Fatalf("unexpected audit: %+v",got.Insights)
	}
	if !reflect.DeepEqual(plan.Opportunities[0].ArgumentIDs,originalIDs) {
		t.Fatal("Build mutated source Manager argument IDs")
	}
	for n:=0;n<5;n++{
		again,err:=Build(artifact,state,plan)
		if err!=nil {t.Fatal(err)}
		b1,_:=json.Marshal(got);b2,_:=json.Marshal(again)
		if string(b1)!=string(b2){t.Fatal("audit JSON ordering is not deterministic")}
	}
	got.Insights[0].ArgumentReferenceAudits[0].ArgumentID="mutated"
	if plan.Opportunities[0].ArgumentIDs[0]!="bear-1"{
		t.Fatal("output audit aliases source Manager IDs")
	}

	artifact,state,plan=ledgerFixture()
	legacy,err:=Build(artifact,state,plan)
	if err!=nil{t.Fatal(err)}
	raw,err:=json.Marshal(legacy)
	if err!=nil{t.Fatal(err)}
	if strings.Contains(string(raw),"argument_reference_audits") {
		t.Fatalf("legacy JSON changed without argument IDs: %s",raw)
	}
}
