package research

import (
	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/decision"
	"Aegis/internal/research/synthesis"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
)

const (
	marketNodeKey    = "market"
	technicalNodeKey = "technical"
	eventNodeKey     = "event"
	synthesisNodeKey = "synthesis"
	tradingNodeKey   = "trading"

	defaultMarketQuery = "分析当前 A 股市场环境。"
)

type Input struct {
	Symbol      string
	MarketQuery string
	EventInput  eventagent.Input
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

type eventAnalyzer interface {
	Analyze(
		ctx context.Context,
		input eventagent.Input,
		opts ...adk.AgentRunOption,
	) (*eventagent.Context, error)
}

type researchSynthesizer interface {
	Synthesize(
		ctx context.Context,
		input synthesis.Input,
		opts ...model.Option,
	) (*synthesis.Artifact, error)
}

type artifactTradingAnalyzer interface {
	AnalyzeArtifact(
		ctx context.Context,
		artifact *synthesis.Artifact,
		opts ...adk.AgentRunOption,
	) (*decision.Decision, error)
}

type Config struct {
	MarketAgent    marketAnalyzer
	TechnicalAgent technicalAnalyzer
	EventAgent     eventAnalyzer
	Synthesizer    researchSynthesizer
	TradingAgent   artifactTradingAnalyzer
}

type Workflow struct {
	runnable       compose.Runnable[Input, *decision.Decision]
	normalizeInput func(Input) (Input, error)
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

type eventResult struct {
	EventContext *eventagent.Context
}

type synthesisInput struct {
	Symbol           string
	MarketContext    *market.Context
	TechnicalContext *technical.Context
	EventContext     *eventagent.Context
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
	if cfg.EventAgent == nil {
		return nil, errors.New(
			"research event agent is required",
		)
	}
	if cfg.Synthesizer == nil {
		return nil, errors.New(
			"research synthesizer is required",
		)
	}
	if cfg.TradingAgent == nil {
		return nil, errors.New(
			"research artifact trading agent is required",
		)
	}

	workflow := compose.NewWorkflow[
		Input,
		*decision.Decision,
	]()

	marketNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			input Input,
		) (*marketResult, error) {
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
			if err := market.Validate(
				marketContext,
			); err != nil {
				return nil, fmt.Errorf(
					"invalid market context: %w",
					err,
				)
			}

			return &marketResult{
				Symbol:        input.Symbol,
				MarketContext: marketContext,
			}, nil
		},
	)

	eventNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			input Input,
		) (*eventResult, error) {
			eventContext, err :=
				cfg.EventAgent.Analyze(
					ctx,
					input.EventInput,
				)
			if err != nil {
				return nil, fmt.Errorf(
					"event research: %w",
					err,
				)
			}
			if err := eventagent.Validate(
				eventContext,
			); err != nil {
				return nil, fmt.Errorf(
					"invalid event context: %w",
					err,
				)
			}

			return &eventResult{
				EventContext: eventContext,
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
			if err := technical.Validate(
				technicalContext,
			); err != nil {
				return nil, fmt.Errorf(
					"invalid technical context: %w",
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

	synthesisNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			input *synthesisInput,
		) (*synthesis.Artifact, error) {
			if input == nil {
				return nil, errors.New(
					"research synthesis input is nil",
				)
			}

			artifact, err :=
				cfg.Synthesizer.Synthesize(
					ctx,
					synthesis.Input{
						Symbol:    input.Symbol,
						Market:    input.MarketContext,
						Technical: input.TechnicalContext,
						Event:     input.EventContext,
					},
				)
			if err != nil {
				return nil, fmt.Errorf(
					"research synthesis: %w",
					err,
				)
			}
			if err := synthesis.Validate(
				artifact,
			); err != nil {
				return nil, fmt.Errorf(
					"invalid research artifact: %w",
					err,
				)
			}
			return artifact, nil
		},
	)

	tradingNode := compose.InvokableLambda(
		func(
			ctx context.Context,
			artifact *synthesis.Artifact,
		) (*decision.Decision, error) {
			if artifact == nil {
				return nil, errors.New(
					"research artifact is nil",
				)
			}

			result, err :=
				cfg.TradingAgent.AnalyzeArtifact(
					ctx,
					artifact,
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

	workflow.AddLambdaNode(
		marketNodeKey,
		marketNode,
	).AddInput(compose.START)

	workflow.AddLambdaNode(
		eventNodeKey,
		eventNode,
	).AddInput(compose.START)

	workflow.AddLambdaNode(
		technicalNodeKey,
		technicalNode,
	).AddInput(marketNodeKey)

	workflow.AddLambdaNode(
		synthesisNodeKey,
		synthesisNode,
	).
		AddInput(
			technicalNodeKey,
			compose.MapFields(
				"Symbol",
				"Symbol",
			),
			compose.MapFields(
				"MarketContext",
				"MarketContext",
			),
			compose.MapFields(
				"TechnicalContext",
				"TechnicalContext",
			),
		).
		AddInput(
			eventNodeKey,
			compose.MapFields(
				"EventContext",
				"EventContext",
			),
		)

	workflow.AddLambdaNode(
		tradingNodeKey,
		tradingNode,
	).AddInput(synthesisNodeKey)

	workflow.End().AddInput(
		tradingNodeKey,
	)

	runnable, err := workflow.Compile(
		ctx,
		compose.WithGraphName(
			"aegis_research",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"compile research workflow: %w",
			err,
		)
	}

	return &Workflow{
		runnable:       runnable,
		normalizeInput: normalizeWorkflowInput,
	}, nil
}

func (w *Workflow) Run(
	ctx context.Context,
	input Input,
) (*decision.Decision, error) {
	if w == nil || w.runnable == nil {
		return nil, errors.New(
			"research workflow is not initialized",
		)
	}

	normalize := w.normalizeInput
	if normalize == nil {
		normalize = normalizeLegacyWorkflowInput
	}
	normalized, err := normalize(input)
	if err != nil {
		return nil, err
	}

	result, err := w.runnable.Invoke(
		ctx,
		normalized,
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

func normalizeWorkflowInput(
	input Input,
) (Input, error) {
	input.Symbol = strings.ToUpper(
		strings.TrimSpace(input.Symbol),
	)
	if input.Symbol == "" {
		return Input{}, errors.New(
			"research symbol is required",
		)
	}

	eventSymbol := strings.ToUpper(
		strings.TrimSpace(
			input.EventInput.Task.Symbol,
		),
	)
	if eventSymbol == "" {
		return Input{}, errors.New(
			"event research task symbol is required",
		)
	}
	if eventSymbol != input.Symbol {
		return Input{}, fmt.Errorf(
			"event research task symbol %q does not match research symbol %q",
			input.EventInput.Task.Symbol,
			input.Symbol,
		)
	}

	return input, nil
}


func normalizeLegacyWorkflowInput(
	input Input,
) (Input, error) {
	input.Symbol = strings.ToUpper(
		strings.TrimSpace(input.Symbol),
	)
	if input.Symbol == "" {
		return Input{}, errors.New(
			"research symbol is required",
		)
	}
	return input, nil
}
