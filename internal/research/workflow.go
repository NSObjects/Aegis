package research

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	"Aegis/internal/decision"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

const (
	marketNodeKey    = "market"
	technicalNodeKey = "technical"
	tradingNodeKey   = "trading"

	defaultMarketQuery = "分析当前 A 股市场环境。"
)

type Input struct {
	Symbol      string
	MarketQuery string
}

type marketAnalyzer interface {
	Analyze(
		ctx context.Context,
		query string,
		opts ...adk.AgentRunOption,
	) (*market.Context, error)
}

type technicalAnalyzer interface {
	Analyze(
		ctx context.Context,
		input technical.Input,
		opts ...adk.AgentRunOption,
	) (*technical.Context, error)
}

type tradingAnalyzer interface {
	Analyze(
		ctx context.Context,
		input trading.Input,
		opts ...adk.AgentRunOption,
	) (*decision.Decision, error)
}

type Config struct {
	MarketAgent    marketAnalyzer
	TechnicalAgent technicalAnalyzer
	TradingAgent   tradingAnalyzer
}

type Workflow struct {
	runnable compose.Runnable[Input, *decision.Decision]
}

type marketResult struct {
	Symbol        string
	MarketContext *market.Context
}

type technicalResult struct {
	Symbol           string
	MarketContext    *market.Context
	TechnicalContext *technical.Context
}

func New(
	ctx context.Context,
	cfg Config,
) (*Workflow, error) {
	if cfg.MarketAgent == nil {
		return nil, errors.New(
			"research market agent is required",
		)
	}

	if cfg.TechnicalAgent == nil {
		return nil, errors.New(
			"research technical agent is required",
		)
	}

	if cfg.TradingAgent == nil {
		return nil, errors.New(
			"research trading agent is required",
		)
	}

	graph := compose.NewGraph[
		Input,
		*decision.Decision,
	]()

	marketNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			input Input,
		) (
			*marketResult,
			error,
		) {
			symbol := strings.TrimSpace(input.Symbol)
			if symbol == "" {
				return nil, errors.New(
					"research symbol is required",
				)
			}

			query := strings.TrimSpace(
				input.MarketQuery,
			)
			if query == "" {
				query = defaultMarketQuery
			}

			marketContext, err :=
				cfg.MarketAgent.Analyze(
					ctx,
					query,
				)
			if err != nil {
				return nil, fmt.Errorf(
					"market research: %w",
					err,
				)
			}

			return &marketResult{
				Symbol:        symbol,
				MarketContext: marketContext,
			}, nil
		},
	)

	technicalNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			input *marketResult,
		) (
			*technicalResult,
			error,
		) {
			if input == nil {
				return nil, errors.New(
					"market research result is nil",
				)
			}

			if err := market.Validate(input.MarketContext); err != nil {
				return nil, fmt.Errorf(
					"invalid market context before technical research: %w",
					err,
				)
			}

			technicalContext, err :=
				cfg.TechnicalAgent.Analyze(
					ctx,
					technical.Input{
						Symbol: input.Symbol,
						RequestedAsOf:
							input.MarketContext.AsOf,
					},
				)
			if err != nil {
				return nil, fmt.Errorf(
					"technical research: %w",
					err,
				)
			}

			return &technicalResult{
				Symbol:           input.Symbol,
				MarketContext:    input.MarketContext,
				TechnicalContext: technicalContext,
			}, nil
		},
	)

	tradingNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			input *technicalResult,
		) (
			*decision.Decision,
			error,
		) {
			if input == nil {
				return nil, errors.New(
					"technical research result is nil",
				)
			}

			result, err :=
				cfg.TradingAgent.Analyze(
					ctx,
					trading.Input{
						Symbol:           input.Symbol,
						MarketContext:    input.MarketContext,
						TechnicalContext: input.TechnicalContext,
					},
				)
			if err != nil {
				return nil, fmt.Errorf(
					"trading research: %w",
					err,
				)
			}

			return result, nil
		},
	)

	if err := graph.AddLambdaNode(
		marketNodeKey,
		marketNode,
	); err != nil {
		return nil, fmt.Errorf(
			"add market graph node: %w",
			err,
		)
	}

	if err := graph.AddLambdaNode(
		technicalNodeKey,
		technicalNode,
	); err != nil {
		return nil, fmt.Errorf(
			"add technical graph node: %w",
			err,
		)
	}

	if err := graph.AddLambdaNode(
		tradingNodeKey,
		tradingNode,
	); err != nil {
		return nil, fmt.Errorf(
			"add trading graph node: %w",
			err,
		)
	}

	if err := graph.AddEdge(
		compose.START,
		marketNodeKey,
	); err != nil {
		return nil, fmt.Errorf(
			"connect graph start to market: %w",
			err,
		)
	}

	if err := graph.AddEdge(
		marketNodeKey,
		technicalNodeKey,
	); err != nil {
		return nil, fmt.Errorf(
			"connect market to technical: %w",
			err,
		)
	}

	if err := graph.AddEdge(
		technicalNodeKey,
		tradingNodeKey,
	); err != nil {
		return nil, fmt.Errorf(
			"connect technical to trading: %w",
			err,
		)
	}

	if err := graph.AddEdge(
		tradingNodeKey,
		compose.END,
	); err != nil {
		return nil, fmt.Errorf(
			"connect trading to graph end: %w",
			err,
		)
	}

	runnable, err := graph.Compile(
		ctx,
		compose.WithGraphName(
			"aegis_research",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"compile research graph: %w",
			err,
		)
	}

	return &Workflow{
		runnable: runnable,
	}, nil
}

func (w *Workflow) Run(
	ctx context.Context,
	input Input,
) (
	*decision.Decision,
	error,
) {
	if w == nil || w.runnable == nil {
		return nil, errors.New(
			"research workflow is not initialized",
		)
	}

	input.Symbol = strings.TrimSpace(
		input.Symbol,
	)
	if input.Symbol == "" {
		return nil, errors.New(
			"research symbol is required",
		)
	}

	result, err := w.runnable.Invoke(
		ctx,
		input,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"run research workflow: %w",
			err,
		)
	}

	if result == nil {
		return nil, errors.New(
			"research workflow returned nil decision",
		)
	}

	return result, nil
}
