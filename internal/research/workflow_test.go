package research

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/trading"
	"Aegis/internal/decision"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
)

type fakeMarketAgent struct {
	query  string
	result *market.Context
	err    error
	calls  int
}

func (a *fakeMarketAgent) Analyze(
	_ context.Context,
	query string,
	_ ...adk.AgentRunOption,
) (
	*market.Context,
	error,
) {
	a.calls++
	a.query = query

	if a.err != nil {
		return nil, a.err
	}

	return a.result, nil
}

type fakeTradingAgent struct {
	input  trading.Input
	result *decision.Decision
	err    error
	calls  int
}

func (a *fakeTradingAgent) Analyze(
	_ context.Context,
	input trading.Input,
	_ ...adk.AgentRunOption,
) (
	*decision.Decision,
	error,
) {
	a.calls++
	a.input = input

	if a.err != nil {
		return nil, a.err
	}

	return a.result, nil
}

func validMarketContext() *market.Context {
	return &market.Context{
		AsOf:       "20260923",
		Regime:     market.RegimeMixed,
		Evidence:   []string{"市场分化"},
		Risk:       []string{"广度偏弱"},
		Confidence: 0.64,
	}
}

func validDecision() *decision.Decision {
	return &decision.Decision{
		Symbol:     "600519.SH",
		MarketView: "市场分化，个股保持观察",
		Evidence:   []string{"市场为 MIXED"},
		Risk:       []string{"广度偏弱"},
		Action:     decision.ActionWatch,
		Confidence: 0.66,
	}
}

func TestWorkflowRun(t *testing.T) {
	marketContext := validMarketContext()

	marketAgent := &fakeMarketAgent{
		result: marketContext,
	}

	tradingAgent := &fakeTradingAgent{
		result: validDecision(),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent:  marketAgent,
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatalf(
			"create workflow: %v",
			err,
		)
	}

	result, err := workflow.Run(
		context.Background(),
		Input{
			Symbol: "600519.SH",
		},
	)
	if err != nil {
		t.Fatalf(
			"run workflow: %v",
			err,
		)
	}

	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"unexpected decision symbol: %s",
			result.Symbol,
		)
	}

	if marketAgent.calls != 1 {
		t.Fatalf(
			"market calls = %d, want 1",
			marketAgent.calls,
		)
	}

	if marketAgent.query != defaultMarketQuery {
		t.Fatalf(
			"market query = %q, want %q",
			marketAgent.query,
			defaultMarketQuery,
		)
	}

	if tradingAgent.calls != 1 {
		t.Fatalf(
			"trading calls = %d, want 1",
			tradingAgent.calls,
		)
	}

	if tradingAgent.input.Symbol != "600519.SH" {
		t.Fatalf(
			"trading symbol = %q",
			tradingAgent.input.Symbol,
		)
	}

	if tradingAgent.input.MarketContext != marketContext {
		t.Fatal(
			"market context was not passed to trading agent",
		)
	}
}

func TestWorkflowRunCustomMarketQuery(
	t *testing.T,
) {
	marketAgent := &fakeMarketAgent{
		result: validMarketContext(),
	}

	tradingAgent := &fakeTradingAgent{
		result: validDecision(),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent:  marketAgent,
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	const query =
		"分析 2026-09-18 A 股市场环境。"

	_, err = workflow.Run(
		context.Background(),
		Input{
			Symbol:      "600519.SH",
			MarketQuery: query,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if marketAgent.query != query {
		t.Fatalf(
			"market query = %q, want %q",
			marketAgent.query,
			query,
		)
	}
}

func TestWorkflowRunEmptySymbol(t *testing.T) {
	marketAgent := &fakeMarketAgent{
		result: validMarketContext(),
	}

	tradingAgent := &fakeTradingAgent{
		result: validDecision(),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent:  marketAgent,
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = workflow.Run(
		context.Background(),
		Input{},
	)
	if err == nil {
		t.Fatal("expected error")
	}

	if marketAgent.calls != 0 {
		t.Fatalf(
			"market calls = %d, want 0",
			marketAgent.calls,
		)
	}
}

func TestWorkflowMarketErrorStopsFlow(
	t *testing.T,
) {
	marketAgent := &fakeMarketAgent{
		err: errors.New("market failed"),
	}

	tradingAgent := &fakeTradingAgent{
		result: validDecision(),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent:  marketAgent,
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = workflow.Run(
		context.Background(),
		Input{
			Symbol: "600519.SH",
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(
		err.Error(),
		"market research",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if tradingAgent.calls != 0 {
		t.Fatalf(
			"trading calls = %d, want 0",
			tradingAgent.calls,
		)
	}
}

func TestWorkflowTradingError(
	t *testing.T,
) {
	marketAgent := &fakeMarketAgent{
		result: validMarketContext(),
	}

	tradingAgent := &fakeTradingAgent{
		err: errors.New("trading failed"),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent:  marketAgent,
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = workflow.Run(
		context.Background(),
		Input{
			Symbol: "600519.SH",
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(
		err.Error(),
		"trading research",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestNewRequiresAgents(t *testing.T) {
	t.Run("market agent", func(t *testing.T) {
		_, err := New(
			context.Background(),
			Config{
				TradingAgent:
					&fakeTradingAgent{},
			},
		)

		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("trading agent", func(t *testing.T) {
		_, err := New(
			context.Background(),
			Config{
				MarketAgent:
					&fakeMarketAgent{},
			},
		)

		if err == nil {
			t.Fatal("expected error")
		}
	})
}
