package risk

import (
	"Aegis/internal/portfolio"
	riskctx "Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type modelStub struct {
	output string
	calls  int
}

func (m *modelStub) Generate(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.AgenticMessage, error) {
	m.calls++
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: m.output}),
		},
	}, nil
}

func (m *modelStub) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("unexpected Stream call")
}

func testIntent() *tradeintent.Intent {
	return &tradeintent.Intent{
		Symbol:     "600519.SH",
		Thesis:     "research thesis",
		Evidence:   []string{"grounded evidence"},
		Risk:       []string{"known downside"},
		Status:     tradeintent.StatusCandidate,
		Confidence: 0.68,
	}
}

func testPositionsPortfolio() *portfolio.Context {
	return &portfolio.Context{
		State: portfolio.StatePositions,
		AsOf:  "2026-10-08T09:00:00+08:00",
		Positions: []portfolio.Position{
			{Symbol: "600519.SH", Quantity: 100},
			{Symbol: "000001.SZ", Quantity: 200},
		},
	}
}

func TestAnalystBuildsGroundedRiskContext(t *testing.T) {
	m := &modelStub{output: `{
		"symbol":"600519.SH",
		"assessment":"CAUTION",
		"summary":"existing position increases the importance of the known downside risk",
		"findings":[
			{
				"claim":"the candidate intent carries a known downside risk",
				"references":[{"domain":"TRADE_INTENT","symbol":"600519.SH"}]
			}
		],
		"constraints":[
			{
				"claim":"the existing position should be considered by the downstream portfolio layer",
				"references":[{"domain":"PORTFOLIO","symbol":"600519.SH"}]
			}
		],
		"confidence":0.72
	}`}
	a := &Analyst{Model: m}

	result, err := a.Review(context.Background(), testIntent(), testPositionsPortfolio())
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Assessment != riskctx.AssessmentCaution {
		t.Fatalf("assessment = %q", result.Assessment)
	}
	if m.calls != 1 {
		t.Fatalf("model calls = %d, want 1", m.calls)
	}
}

func TestAnalystNotProvidedIsDeterministic(t *testing.T) {
	m := &modelStub{}
	a := &Analyst{Model: m}

	result, err := a.Review(
		context.Background(),
		testIntent(),
		&portfolio.Context{State: portfolio.StateNotProvided},
	)
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Assessment != riskctx.AssessmentInsufficientContext {
		t.Fatalf("assessment = %q", result.Assessment)
	}
	if m.calls != 0 {
		t.Fatalf("model calls = %d, want 0", m.calls)
	}
}

func TestAnalystRejectsInventedPositionReference(t *testing.T) {
	m := &modelStub{output: `{
		"symbol":"600519.SH",
		"assessment":"HIGH_RISK",
		"summary":"invented concentration",
		"findings":[
			{
				"claim":"invented position",
				"references":[{"domain":"PORTFOLIO","symbol":"300750.SZ"}]
			}
		],
		"confidence":0.8
	}`}
	a := &Analyst{Model: m}

	_, err := a.Review(context.Background(), testIntent(), testPositionsPortfolio())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnalystRejectsUnknownOutputField(t *testing.T) {
	m := &modelStub{output: `{
		"symbol":"600519.SH",
		"assessment":"CAUTION",
		"summary":"test",
		"confidence":0.5,
		"action":"AVOID"
	}`}
	a := &Analyst{Model: m}

	_, err := a.Review(context.Background(), testIntent(), testPositionsPortfolio())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnalystRequiresModelForObservedPortfolio(t *testing.T) {
	a := &Analyst{}
	_, err := a.Review(
		context.Background(),
		testIntent(),
		&portfolio.Context{
			State: portfolio.StateEmpty,
			AsOf:  "2026-10-08T09:00:00+08:00",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("error = %v", err)
	}
}
