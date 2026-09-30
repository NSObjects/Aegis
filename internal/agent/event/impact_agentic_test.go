package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestAgenticImpactMechanismUsesGroundedFactsAndEventSemantics(t *testing.T) {
	model:=&eventModel{responses:[]*schema.AgenticMessage{
		assistantJSON(`{"mechanisms":[{"channel":"REVENUE","direction":"POSITIVE","supporting_fact_ids":["fact_contract"],"rationale":"contract creates a potential revenue source","uncertainty":"revenue recognition depends on delivery"}]}`),
	}}
	facts:=[]Fact{groundedEvolutionFact("fact_contract","doc_contract","公司签订50亿元合同。")}
	result,err:=(&AgenticEventImpactMechanismAnalyzer{Model:model}).AnalyzeImpactMechanisms(
		context.Background(),
		capevent.ResearchTask{Symbol:"600519.SH",AsOf:time.Date(2026,9,30,0,0,0,0,time.UTC)},
		facts,nil,nil,
	)
	if err!=nil { t.Fatal(err) }
	if len(result.Mechanisms)!=1 || result.Mechanisms[0].Channel!=ImpactRevenue { t.Fatalf("result=%+v",result) }
	payload:=eventAgenticTestText(model.inputs[0][1])
	if !strings.Contains(payload,`"quote":"公司签订50亿元合同。"`) { t.Fatalf("payload lacks grounding: %s",payload) }
}

func TestAgenticImpactMechanismAllowsEmptyResult(t *testing.T) {
	model:=&eventModel{responses:[]*schema.AgenticMessage{assistantJSON(`{"mechanisms":[]}`)}}
	result,err:=(&AgenticEventImpactMechanismAnalyzer{Model:model}).AnalyzeImpactMechanisms(
		context.Background(),
		capevent.ResearchTask{Symbol:"600519.SH",AsOf:time.Now()},
		[]Fact{groundedEvolutionFact("fact_a","doc_a","董事会完成例行换届。")},nil,nil,
	)
	if err!=nil { t.Fatal(err) }
	if len(result.Mechanisms)!=0 { t.Fatalf("mechanisms=%+v",result.Mechanisms) }
}

func TestMaterialityPayloadContainsImpactMechanisms(t *testing.T) {
	model:=&eventModel{responses:[]*schema.AgenticMessage{
		assistantJSON(`{"level":"MEDIUM","direction":"POSITIVE","horizon":"MEDIUM_TERM","rationale":"contract creates future revenue potential","risks":["delivery timing is uncertain"],"confidence":0.8}`),
	}}
	fact:=groundedEvolutionFact("fact_contract","doc_contract","公司签订50亿元合同。")
	mechanism:=ImpactMechanism{
		ID:deterministicImpactMechanismID(ImpactRevenue,ImpactPositive,[]string{"fact_contract"}),
		Channel:ImpactRevenue,Direction:ImpactPositive,SupportingFactIDs:[]string{"fact_contract"},
		Rationale:"contract creates a potential revenue source",Uncertainty:"delivery timing is uncertain",
	}
	_,err:=(&AgenticEventMaterialityAnalyzer{Model:model}).AnalyzeMateriality(
		context.Background(),
		capevent.ResearchTask{Symbol:"600519.SH",AsOf:time.Now()},
		MaterialityInput{EventID:"event_1",Type:EventContract,Facts:[]Fact{fact},ImpactMechanisms:[]ImpactMechanism{mechanism}},
	)
	if err!=nil { t.Fatal(err) }
	payload:=eventAgenticTestText(model.inputs[0][1])
	if !strings.Contains(payload,`"impact_mechanisms":[{"id":"impact_`) ||
		!strings.Contains(payload,`"channel":"REVENUE"`) { t.Fatalf("payload=%s",payload) }
}
