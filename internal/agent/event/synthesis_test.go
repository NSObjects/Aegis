package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func synthesisTestEvent() Event {
	fact:=groundedEvolutionFact("fact_contract","doc_contract","公司签订50亿元合同。")
	mechanism:=ImpactMechanism{ID:deterministicImpactMechanismID(ImpactRevenue,DirectionPositive,[]string{"fact_contract"}),Channel:ImpactRevenue,Direction:DirectionPositive,SupportingFactIDs:[]string{"fact_contract"},Rationale:"contract creates potential revenue"}
	return Event{ID:"event_contract",Type:EventContract,Headline:"签订合同",FactSummary:"公司签订50亿元合同",Facts:[]Fact{fact},ImpactMechanisms:[]ImpactMechanism{mechanism},Materiality:Materiality{Level:MaterialityMedium,Direction:DirectionPositive,Horizon:HorizonMediumTerm,Rationale:"potential revenue contribution",Confidence:0.7}}
}

func TestNormalizeAnalystSynthesisGroundsReferencesAndSorts(t *testing.T) {
	event:=synthesisTestEvent()
	got,err:=normalizeAnalystSynthesis(AnalystSynthesis{Summary:"  合同形成潜在收入来源 ",OverallMateriality:MaterialityMedium,OverallDirection:DirectionPositive,DominantImpactChannels:[]ImpactChannel{ImpactRevenue,ImpactRevenue},KeyEventIDs:[]string{event.ID,event.ID},KeyRisks:[]string{"交付节奏","交付节奏"},Confidence:0.7},[]Event{event})
	if err!=nil{t.Fatal(err)}
	if got.Summary!="合同形成潜在收入来源" || len(got.KeyEventIDs)!=1 || len(got.DominantImpactChannels)!=1 || len(got.KeyRisks)!=1 {t.Fatalf("synthesis=%+v",got)}
}

func TestNormalizeAnalystSynthesisRejectsInventedReferences(t *testing.T) {
	event:=synthesisTestEvent()
	cases:=[]AnalystSynthesis{
		{Summary:"x",OverallMateriality:MaterialityMedium,OverallDirection:DirectionPositive,KeyEventIDs:[]string{"missing"},Confidence:.5},
		{Summary:"x",OverallMateriality:MaterialityMedium,OverallDirection:DirectionPositive,DominantImpactChannels:[]ImpactChannel{ImpactRegulatory},KeyEventIDs:[]string{event.ID},Confidence:.5},
	}
	for _,item:=range cases { if _,err:=normalizeAnalystSynthesis(item,[]Event{event});err==nil{t.Fatalf("expected error for %+v",item)} }
}

func TestAgenticSynthesisReceivesValidatedEventsOnly(t *testing.T) {
	event:=synthesisTestEvent()
	model:=&eventModel{responses:[]*schema.AgenticMessage{assistantJSON(`{"summary":"合同形成潜在收入来源","overall_materiality":"MEDIUM","overall_direction":"POSITIVE","dominant_impact_channels":["REVENUE"],"key_event_ids":["event_contract"],"key_risks":["交付节奏影响确认"],"confidence":0.7}`)}}
	got,err:=(&AgenticEventSynthesisAnalyzer{Model:model}).Synthesize(context.Background(),capevent.ResearchTask{Symbol:"600519.SH",AsOf:time.Date(2026,9,30,0,0,0,0,time.UTC)},[]Event{event})
	if err!=nil{t.Fatal(err)}
	if got.OverallMateriality!=MaterialityMedium{t.Fatalf("got=%+v",got)}
	payload:=eventAgenticTestText(model.inputs[0][1])
	if !strings.Contains(payload,`"event_contract"`) || !strings.Contains(payload,`"impact_mechanisms"`) {t.Fatalf("payload=%s",payload)}
}
