package event

import (
	capevent "Aegis/internal/capability/event"
	"math"
	"strings"
	"testing"
)

func validConfidenceEventContext() *Context {
	quote := "公司发布公告。"
	return &Context{
		Symbol: "600519.SH",
		RequestedAsOf: "2026-09-29T15:00:00Z",
		Status: StatusNormal,
		Events: []Event{{
			ID: "event_1",
			Type: EventDisclosure,
			Headline: "公告",
			FactSummary: quote,
			Facts: []Fact{{
				ID: "fact_1",
				Type: EventDisclosure,
				Headline: "公告",
				Summary: quote,
				Groundings: []EvidenceGrounding{{
					DocumentID: "doc_1", Quote: quote,
					StartByte: 0, EndByte: len(quote),
				}},
				EvidenceDocumentIDs: []string{"doc_1"},
				Confidence: 0.9,
			}},
			EvidenceDocumentIDs: []string{"doc_1"},
			SourceCount: 1,
			HasPrimarySource: true,
			EvidenceSupport: capevent.EvidenceSupport{
				DocumentCount: 1,
				IndependentEvidenceCount: 1,
				IndependentSourceCount: 1,
				PrimarySourcePresent: true,
				SupportClass: capevent.EvidenceSupportPrimaryOnly,
			},
			FactConfidence: 0.9,
			Materiality: Materiality{
				Level: MaterialityMedium,
				Direction: DirectionUncertain,
				Horizon: HorizonShortTerm,
				Rationale: "还需结合更多信息",
				Confidence: 0.7,
			},
		}},
		Evidence: []string{"doc_1"},
		Confidence: 0.8,
	}
}

func TestValidateEveryEventConfidenceFinite(t *testing.T) {
	fields := []struct {
		name string
		set func(*Context, float64)
	}{
		{"context", func(c *Context, n float64) { c.Confidence = n }},
		{"fact", func(c *Context, n float64) { c.Events[0].Facts[0].Confidence = n }},
		{"event fact confidence", func(c *Context, n float64) { c.Events[0].FactConfidence = n }},
		{"materiality", func(c *Context, n float64) { c.Events[0].Materiality.Confidence = n }},
		{"analyst synthesis", func(c *Context, n float64) {
			c.Synthesis = &AnalystSynthesis{
				Summary: "基于已有公告的事件总结",
				OverallMateriality: MaterialityMedium,
				OverallDirection: DirectionUncertain,
				KeyEventIDs: []string{"event_1"},
				Confidence: n,
			}
		}},
	}
	values := []struct {
		name string
		n float64
		valid bool
	}{
		{"zero", 0, true},
		{"one", 1, true},
		{"nan", math.NaN(), false},
		{"positive infinity", math.Inf(1), false},
		{"negative infinity", math.Inf(-1), false},
		{"below zero", -0.01, false},
		{"above one", 1.01, false},
	}
	if err := Validate(validConfidenceEventContext()); err != nil {
		t.Fatalf("invalid baseline fixture: %v", err)
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, tc := range values {
				t.Run(tc.name, func(t *testing.T) {
					ctx := validConfidenceEventContext()
					field.set(ctx, tc.n)
					err := Validate(ctx)
					if tc.valid && err != nil {
						t.Fatalf("valid confidence rejected: %v", err)
					}
					if !tc.valid && (err == nil || !strings.Contains(err.Error(), "confidence out of range")) {
						t.Fatalf("confidence %v unexpectedly accepted or wrong error: %v", tc.n, err)
					}
				})
			}
		})
	}
}
