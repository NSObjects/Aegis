package trading

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/decision"
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeModel struct {
	output string
	err    error
}

func (m *fakeModel) Generate(
	_ context.Context,
	_ []*schema.AgenticMessage,
	_ ...model.Option,
) (
	*schema.AgenticMessage,
	error,
) {
	if m.err != nil {
		return nil, m.err
	}

	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(
				&schema.AssistantGenText{
					Text: m.output,
				},
			),
		},
	}, nil
}

func (m *fakeModel) Stream(
	_ context.Context,
	_ []*schema.AgenticMessage,
	_ ...model.Option,
) (
	*schema.StreamReader[*schema.AgenticMessage],
	error,
) {
	return nil, errors.New(
		"stream should not be called",
	)
}

func validMarketContext() *market.Context {
	return &market.Context{
		AsOf:      "20260923",
		Regime:    market.RegimeMixed,
		Evidence:  []string{"市场分化"},
		Risk:      []string{"广度偏弱"},
		Confidence: 0.72,
	}
}

func validTechnicalContext() *technical.Context {
	return &technical.Context{
		Symbol:        "600519.SH",
		RequestedAsOf: "20260923",
		EffectiveAsOf: "20260923",
		Status:        technical.StatusNormal,
		Snapshot: &technical.Snapshot{
			Close: 1251.24,
		},
		Evidence: []string{"收盘价低于主要均线"},
		Risk: []string{"短期弱势仍需后续交易日确认"},
		Confidence: 0.82,
	}
}

func TestAgentAnalyze(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{
				output: `{
					"symbol": "600519.SH",
					"market_view": "市场分化，个股自身趋势仍需确认",
					"evidence": [
						"市场为 MIXED",
						"个股趋势保持"
					],
					"risk": [
						"市场广度偏弱"
					],
					"action": "WATCH",
					"confidence": 0.68
				}`,
			},
		},
	)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	result, err := a.Analyze(
		ctx,
		Input{
			Symbol:        "600519.SH",
			MarketContext:    validMarketContext(),
			TechnicalContext: validTechnicalContext(),
		},
	)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"unexpected symbol: %s",
			result.Symbol,
		)
	}

	if result.Action != decision.ActionWatch {
		t.Fatalf(
			"unexpected action: %s",
			result.Action,
		)
	}

	if result.Confidence != 0.68 {
		t.Fatalf(
			"unexpected confidence: %f",
			result.Confidence,
		)
	}
}

func TestAgentAnalyzeInvalidInput(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("empty symbol", func(t *testing.T) {
		_, err := a.Analyze(
			ctx,
			Input{
				MarketContext: validMarketContext(),
			},
		)

		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid market context", func(t *testing.T) {
		_, err := a.Analyze(
			ctx,
			Input{
				Symbol:        "600519.SH",
				MarketContext:    &market.Context{},
				TechnicalContext: validTechnicalContext(),
			},
		)

		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAgentAnalyzeInvalidTechnicalContext(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing technical context", func(t *testing.T) {
		_, err := a.Analyze(
			ctx,
			Input{
				Symbol:           "600519.SH",
				MarketContext:    validMarketContext(),
				TechnicalContext: validTechnicalContext(),
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("technical symbol mismatch", func(t *testing.T) {
		technicalContext := validTechnicalContext()
		technicalContext.Symbol = "000001.SZ"

		_, err := a.Analyze(
			ctx,
			Input{
				Symbol:           "600519.SH",
				MarketContext:    validMarketContext(),
				TechnicalContext: technicalContext,
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("technical as_of mismatch", func(t *testing.T) {
		technicalContext := validTechnicalContext()
		technicalContext.RequestedAsOf = "20260922"

		_, err := a.Analyze(
			ctx,
			Input{
				Symbol:           "600519.SH",
				MarketContext:    validMarketContext(),
				TechnicalContext: technicalContext,
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAgentAnalyzeInvalidOutput(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid json", func(t *testing.T) {
		a, err := New(
			ctx,
			Config{
				Model: &fakeModel{
					output: "not json",
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = a.Analyze(
			ctx,
			Input{
				Symbol:           "600519.SH",
				MarketContext:    validMarketContext(),
				TechnicalContext: validTechnicalContext(),
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid decision", func(t *testing.T) {
		a, err := New(
			ctx,
			Config{
				Model: &fakeModel{
					output: `{
						"symbol": "600519.SH",
						"market_view": "test",
						"evidence": [],
						"risk": [],
						"action": "WATCH",
						"confidence": 0.5
					}`,
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = a.Analyze(
			ctx,
			Input{
				Symbol:           "600519.SH",
				MarketContext:    validMarketContext(),
				TechnicalContext: validTechnicalContext(),
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("symbol mismatch", func(t *testing.T) {
		a, err := New(
			ctx,
			Config{
				Model: &fakeModel{
					output: `{
						"symbol": "000001.SZ",
						"market_view": "test",
						"evidence": ["test"],
						"risk": [],
						"action": "WATCH",
						"confidence": 0.5
					}`,
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = a.Analyze(
			ctx,
			Input{
				Symbol:           "600519.SH",
				MarketContext:    validMarketContext(),
				TechnicalContext: validTechnicalContext(),
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAgentAnalyzeModelError(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{
				err: errors.New("model unavailable"),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Analyze(
		ctx,
		Input{
			Symbol:        "600519.SH",
			MarketContext:    validMarketContext(),
			TechnicalContext: validTechnicalContext(),
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRequiresModel(t *testing.T) {
	_, err := New(
		context.Background(),
		Config{},
	)

	if err == nil {
		t.Fatal("expected error")
	}
}


func TestAssistantTextIgnoresToolCallMessage(t *testing.T) {
	msg := &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(
				&schema.AssistantGenText{
					Text: "I will call a tool",
				},
			),
			schema.NewContentBlock(
				&schema.FunctionToolCall{
					CallID:    "call-1",
					Name:      "execute",
					Arguments: `{"command":"true"}`,
				},
			),
		},
	}

	if got := assistantText(msg); got != "" {
		t.Fatalf(
			"assistantText() = %q, want empty for tool-call message",
			got,
		)
	}
}
