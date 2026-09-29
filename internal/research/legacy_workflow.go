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

type legacyTradingAnalyzer interface {
	Analyze(
		ctx context.Context,
		input trading.Input,
		opts ...adk.AgentRunOption,
	) (*decision.Decision, error)
}

type LegacyConfig struct {
	MarketAgent    marketAnalyzer
	TechnicalAgent technicalAnalyzer
	TradingAgent   legacyTradingAnalyzer
}

// NewLegacy preserves the pre-V2.7.14 Market -> Technical -> Trading path
// until the production News/Event planner bootstrap is wired. New code should
// use New and the ResearchArtifact path.
func NewLegacy(
	ctx context.Context,
	cfg LegacyConfig,
) (*Workflow, error) {
	if cfg.MarketAgent == nil {
		return nil, errors.New(
			"legacy research market agent is required",
		)
	}
	if cfg.TechnicalAgent == nil {
		return nil, errors.New(
			"legacy research technical agent is required",
		)
	}
	if cfg.TradingAgent == nil {
		return nil, errors.New(
			"legacy research trading agent is required",
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
		) (*marketResult, error) {
			symbol := strings.ToUpper(
				strings.TrimSpace(input.Symbol),
			)
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
		) (*technicalResult, error) {
			if input == nil {
				return nil, errors.New(
					"market research result is nil",
				)
			}
			if err := market.Validate(
				input.MarketContext,
			); err != nil {
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
		) (*decision.Decision, error) {
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
			"add legacy market graph node: %w",
			err,
		)
	}
	if err := graph.AddLambdaNode(
		technicalNodeKey,
		technicalNode,
	); err != nil {
		return nil, fmt.Errorf(
			"add legacy technical graph node: %w",
			err,
		)
	}
	if err := graph.AddLambdaNode(
		tradingNodeKey,
		tradingNode,
	); err != nil {
		return nil, fmt.Errorf(
			"add legacy trading graph node: %w",
			err,
		)
	}

	for _, edge := range [][2]string{
		{compose.START, marketNodeKey},
		{marketNodeKey, technicalNodeKey},
		{technicalNodeKey, tradingNodeKey},
		{tradingNodeKey, compose.END},
	} {
		if err := graph.AddEdge(
			edge[0],
			edge[1],
		); err != nil {
			return nil, fmt.Errorf(
				"connect legacy research graph %s -> %s: %w",
				edge[0],
				edge[1],
				err,
			)
		}
	}

	runnable, err := graph.Compile(
		ctx,
		compose.WithGraphName(
			"aegis_research_legacy",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"compile legacy research graph: %w",
			err,
		)
	}

	return &Workflow{
		runnable:       runnable,
		normalizeInput: normalizeLegacyWorkflowInput,
	}, nil
}
