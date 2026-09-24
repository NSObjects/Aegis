package technical

import (
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
) (*schema.AgenticMessage, error) {
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
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New(
		"stream should not be called",
	)
}

func TestAgentAnalyze(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{
				output: `{
					"symbol": "600519.SH",
					"requested_as_of": "20260923",
					"effective_as_of": "20260923",
					"status": "NORMAL",
					"snapshot": {
						"open": 1250,
						"high": 1260,
						"low": 1248,
						"close": 1251.24,
						"pre_close": 1253.80,
						"change_percent": -0.20,
						"volume": 3000000,
						"amount": 3800000000
					},
					"returns": {
						"return_5d": -0.54
					},
					"moving_averages": {
						"ma5": 1256.3
					},
					"momentum": {
						"rsi14": 42.1,
						"macd": {
							"dif": -8.2,
							"dea": -6.9,
							"hist": -2.6
						}
					},
					"volume": {},
					"evidence": [
						"收盘价低于 MA5"
					],
					"risk": [
						"短期仍可能出现反弹"
					],
					"confidence": 0.70
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
			RequestedAsOf: "20260923",
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

	if result.RequestedAsOf != "20260923" {
		t.Fatalf(
			"unexpected requested_as_of: %s",
			result.RequestedAsOf,
		)
	}

	if result.Status != StatusNormal {
		t.Fatalf(
			"unexpected status: %s",
			result.Status,
		)
	}

	if result.Snapshot == nil {
		t.Fatal("snapshot is nil")
	}

	if result.Confidence != 0.70 {
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
				RequestedAsOf: "20260923",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty requested as of", func(t *testing.T) {
		_, err := a.Analyze(
			ctx,
			Input{
				Symbol: "600519.SH",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAgentAnalyzeInvalidOutput(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "invalid json",
			output: "not json",
		},
		{
			name: "invalid context",
			output: `{
				"symbol": "600519.SH",
				"requested_as_of": "20260923",
				"status": "NORMAL",
				"evidence": [],
				"risk": [],
				"confidence": 0.5
			}`,
		},
		{
			name: "symbol mismatch",
			output: `{
				"symbol": "000001.SZ",
				"requested_as_of": "20260923",
				"effective_as_of": "20260923",
				"status": "NORMAL",
				"snapshot": {
					"open": 1,
					"high": 1,
					"low": 1,
					"close": 1,
					"pre_close": 1,
					"change_percent": 0,
					"volume": 1,
					"amount": 1
				},
				"returns": {},
				"moving_averages": {},
				"momentum": {"macd": {}},
				"volume": {},
				"evidence": ["test"],
				"risk": [],
				"confidence": 0.5
			}`,
		},
		{
			name: "requested as of mismatch",
			output: `{
				"symbol": "600519.SH",
				"requested_as_of": "20260922",
				"effective_as_of": "20260922",
				"status": "NORMAL",
				"snapshot": {
					"open": 1,
					"high": 1,
					"low": 1,
					"close": 1,
					"pre_close": 1,
					"change_percent": 0,
					"volume": 1,
					"amount": 1
				},
				"returns": {},
				"moving_averages": {},
				"momentum": {"macd": {}},
				"volume": {},
				"evidence": ["test"],
				"risk": [],
				"confidence": 0.5
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(
				ctx,
				Config{
					Model: &fakeModel{
						output: tt.output,
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
					RequestedAsOf: "20260923",
				},
			)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
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
			RequestedAsOf: "20260923",
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
