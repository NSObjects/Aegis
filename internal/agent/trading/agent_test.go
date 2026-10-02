package trading

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/decision"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/research/manager"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeModel struct {
	output string
	err    error
	calls  int
	inputs [][]*schema.AgenticMessage
}

func (m *fakeModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	_ ...model.Option,
) (
	*schema.AgenticMessage,
	error,
) {
	m.calls++
	m.inputs = append(
		m.inputs,
		append(
			[]*schema.AgenticMessage(nil),
			input...,
		),
	)
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
				Symbol:        "600519.SH",
				MarketContext: validMarketContext(),
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


func validResearchArtifact() *synthesis.Artifact {
	return &synthesis.Artifact{
		Symbol:    "600519.SH",
		Market:    validMarketContext(),
		Technical: validTechnicalContext(),
		Event: &event.Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "2026-09-23T15:00:00+08:00",
			Status:        event.StatusNoEvent,
			Confidence:    0.88,
		},
		Synthesis: synthesis.Synthesis{
			Summary: "市场与技术信号偏谨慎，事件层未提取出有效事件。",
			Agreements: []synthesis.Agreement{
				{
					Claim: "市场和技术上下文均未形成强确认。",
					References: []synthesis.Reference{
						{Domain: synthesis.DomainMarket},
						{Domain: synthesis.DomainTechnical},
					},
				},
			},
			Risks: []string{
				"技术趋势仍需确认",
			},
			Confidence: 0.74,
		},
	}
}

func TestAgentAnalyzeArtifact(t *testing.T) {
	model := &fakeModel{
		output: `{
			"symbol": "600519.SH",
			"market_view": "跨域研究结论偏谨慎",
			"evidence": [
				"ResearchArtifact synthesis 指出市场与技术尚未形成强确认"
			],
			"risk": [
				"技术趋势仍需确认"
			],
			"action": "WATCH",
			"confidence": 0.67
		}`,
	}
	a, err := New(
		context.Background(),
		Config{Model: model},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.AnalyzeArtifact(
		context.Background(),
		validResearchArtifact(),
	)
	if err != nil {
		t.Fatalf(
			"AnalyzeArtifact() error = %v",
			err,
		)
	}
	if result.Action != decision.ActionWatch ||
		result.Symbol != "600519.SH" {
		t.Fatalf(
			"decision = %+v",
			result,
		)
	}
	if model.calls != 1 {
		t.Fatalf(
			"model calls = %d, want 1",
			model.calls,
		)
	}

	var userText string
	for _, message := range model.inputs[0] {
		if message == nil ||
			message.Role !=
				schema.AgenticRoleTypeUser {
			continue
		}
		for _, block := range message.ContentBlocks {
			if block != nil &&
				block.UserInputText != nil {
				userText +=
					block.UserInputText.Text
			}
		}
	}
	if !strings.Contains(
		userText,
		"research_artifact",
	) ||
		!strings.Contains(
			userText,
			`"synthesis"`,
		) {
		t.Fatalf(
			"artifact trading input = %s",
			userText,
		)
	}
}

func TestAgentAnalyzeArtifactRejectsInvalidArtifactBeforeModel(
	t *testing.T,
) {
	model := &fakeModel{
		output: `{
			"symbol":"600519.SH",
			"market_view":"unused",
			"evidence":["unused"],
			"risk":[],
			"action":"WATCH",
			"confidence":0.5
		}`,
	}
	a, err := New(
		context.Background(),
		Config{Model: model},
	)
	if err != nil {
		t.Fatal(err)
	}

	artifact := validResearchArtifact()
	artifact.Event.Symbol = "000001.SZ"

	_, err = a.AnalyzeArtifact(
		context.Background(),
		artifact,
	)
	if err == nil {
		t.Fatal(
			"expected invalid research artifact error",
		)
	}
	if model.calls != 0 {
		t.Fatalf(
			"model calls = %d, want 0",
			model.calls,
		)
	}
}


func validResearchPlan() *manager.ResearchPlan {
	return &manager.ResearchPlan{
		Symbol:     "600519.SH",
		Conclusion: manager.ConclusionBalanced,
		Summary:    "bullish technical evidence and bearish market evidence remain in tension",
		Opportunities: []manager.Insight{
			{
				Claim:      "technical setup remains constructive enough to monitor",
				References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
			},
		},
		Risks: []manager.Insight{
			{
				Claim:      "market breadth remains a headwind",
				References: []synthesis.Reference{{Domain: synthesis.DomainMarket}},
			},
		},
		AcceptedArgumentIDs:   []string{"bull-r1-a1"},
		UnresolvedArgumentIDs: []string{"bear-r1-a1"},
		TradingConstraints:    []string{"respect unresolved market weakness"},
		Confidence:            0.64,
	}
}

func TestAgentAnalyzeResearchPlan(t *testing.T) {
	model := &fakeModel{
		output: `{
			"symbol":"600519.SH",
			"market_view":"多 Agent 研究仍存在分歧",
			"evidence":["技术研究仍提供可跟踪依据"],
			"risk":["市场弱势论点尚未解决"],
			"action":"WATCH",
			"confidence":0.61
		}`,
	}
	a, err := New(context.Background(), Config{Model: model})
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.AnalyzeResearchPlan(
		context.Background(),
		validResearchArtifact(),
		validResearchPlan(),
	)
	if err != nil {
		t.Fatalf("AnalyzeResearchPlan() error = %v", err)
	}
	if result.Action != decision.ActionWatch {
		t.Fatalf("action = %q", result.Action)
	}

	var userText string
	for _, message := range model.inputs[0] {
		if message == nil || message.Role != schema.AgenticRoleTypeUser {
			continue
		}
		for _, block := range message.ContentBlocks {
			if block != nil && block.UserInputText != nil {
				userText += block.UserInputText.Text
			}
		}
	}
	if !strings.Contains(userText, "research_artifact") ||
		!strings.Contains(userText, "research_plan") ||
		!strings.Contains(userText, "BALANCED") {
		t.Fatalf("research-plan trading input = %s", userText)
	}
}

func TestAgentAnalyzeResearchPlanRejectsInvalidPlanBeforeModel(t *testing.T) {
	model := &fakeModel{}
	a, err := New(context.Background(), Config{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	plan := validResearchPlan()
	plan.Symbol = "000001.SZ"

	_, err = a.AnalyzeResearchPlan(
		context.Background(),
		validResearchArtifact(),
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
	if model.calls != 0 {
		t.Fatalf("model calls = %d, want 0", model.calls)
	}
}

func TestAgentAnalyzeResearchPlanRejectsInvalidReferenceBeforeModel(t *testing.T) {
	model := &fakeModel{}
	a, err := New(context.Background(), Config{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	plan := validResearchPlan()
	plan.Risks = []manager.Insight{
		{
			Claim:      "invented event",
			References: []synthesis.Reference{{Domain: synthesis.DomainEvent, EventID: "invented"}},
		},
	}

	_, err = a.AnalyzeResearchPlan(
		context.Background(),
		validResearchArtifact(),
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
	if model.calls != 0 {
		t.Fatalf("model calls = %d, want 0", model.calls)
	}
}
