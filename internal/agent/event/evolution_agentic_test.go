package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestAgenticFactEvolutionDetectorUsesGroundedFacts(t *testing.T) {
	model := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"relations":[{"type":"REALIZES","from_fact_id":"actual","to_fact_id":"forecast","rationale":"actual result realizes prior forecast"}]}`),
	}}
	detector := &AgenticEventFactEvolutionDetector{Model:model}
	result, err := detector.DetectFactEvolution(
		context.Background(),
		capevent.ResearchTask{Symbol:"600519.SH", AsOf:time.Date(2026,9,30,0,0,0,0,time.UTC)},
		[]Fact{
			groundedEvolutionFact("forecast","doc_f","预计销量100。"),
			groundedEvolutionFact("actual","doc_a","实际销量98。"),
		},
	)
	if err != nil { t.Fatal(err) }
	if len(result.Relations)!=1 || result.Relations[0].Type!=FactRelationRealizes {
		t.Fatalf("result=%+v", result)
	}
	payload := eventAgenticTestText(model.inputs[0][1])
	if !strings.Contains(payload, `"quote":"预计销量100。"`) ||
		!strings.Contains(payload, `"quote":"实际销量98。"`) {
		t.Fatalf("payload lacks grounded facts: %s", payload)
	}
}

func TestAgenticFactEvolutionDetectorAllowsExplicitEmptyResult(t *testing.T) {
	model := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"relations":[]}`),
	}}
	result, err := (&AgenticEventFactEvolutionDetector{Model:model}).DetectFactEvolution(
		context.Background(),
		capevent.ResearchTask{Symbol:"600519.SH", AsOf:time.Now()},
		[]Fact{
			groundedEvolutionFact("period_a","doc_a","2025年收入100。"),
			groundedEvolutionFact("period_b","doc_b","2026年收入120。"),
		},
	)
	if err != nil { t.Fatal(err) }
	if len(result.Relations)!=0 { t.Fatalf("relations=%+v", result.Relations) }
}

func TestMaterialityPayloadContainsFactRelations(t *testing.T) {
	model := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"level":"MEDIUM","direction":"UNCERTAIN","horizon":"SHORT_TERM","rationale":"later actual is available","risks":[],"confidence":0.8}`),
	}}
	facts:=[]Fact{
		groundedEvolutionFact("forecast","doc_f","预计销量100。"),
		groundedEvolutionFact("actual","doc_a","实际销量98。"),
	}
	_, err := (&AgenticEventMaterialityAnalyzer{Model:model}).AnalyzeMateriality(
		context.Background(),
		capevent.ResearchTask{Symbol:"600519.SH", AsOf:time.Now()},
		MaterialityInput{
			EventID:"event_1", Type:EventGuidance, Facts:facts,
			FactRelations:[]FactRelation{{
				ID:deterministicFactRelationID(FactRelationRealizes,"actual","forecast"),
				Type:FactRelationRealizes, FromFactID:"actual", ToFactID:"forecast",
				Rationale:"actual result realizes prior forecast",
			}},
			EvidenceSupport:capevent.EvidenceSupport{},
		},
	)
	if err != nil { t.Fatal(err) }
	payload:=eventAgenticTestText(model.inputs[0][1])
	if !strings.Contains(payload, `"fact_relations":[{"id":"relation_`) ||
		!strings.Contains(payload, `"type":"REALIZES"`) {
		t.Fatalf("payload=%s", payload)
	}
}
