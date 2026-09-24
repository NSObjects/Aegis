package trading

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/decision"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	Model model.BaseModel[*schema.AgenticMessage]

	Handlers []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]

	MaxIterations int
}

type Input struct {
	Symbol        string
	MarketContext *market.Context
}

type Agent struct {
	runner *adk.TypedRunner[*schema.AgenticMessage]
}

func New(
	ctx context.Context,
	cfg Config,
) (*Agent, error) {
	if cfg.Model == nil {
		return nil, errors.New(
			"trading agent model is required",
		)
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 20
	}

	a, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "trading_agent",
			Description: "Researches a single symbol using market context and symbol-specific evidence, then produces a structured decision.",
			Instruction: instruction,
			Model:       cfg.Model,
			Handlers:    cfg.Handlers,

			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create trading agent: %w",
			err,
		)
	}

	runner := adk.NewTypedRunner(
		adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent:           a,
			EnableStreaming: false,
		},
	)

	return &Agent{
		runner: runner,
	}, nil
}

func (a *Agent) Analyze(
	ctx context.Context,
	input Input,
	opts ...adk.AgentRunOption,
) (
	*decision.Decision,
	error,
) {
	if a == nil || a.runner == nil {
		return nil, errors.New(
			"trading agent is not initialized",
		)
	}

	symbol := strings.TrimSpace(input.Symbol)
	if symbol == "" {
		return nil, errors.New(
			"trading symbol is required",
		)
	}

	if err := market.Validate(input.MarketContext); err != nil {
		return nil, fmt.Errorf(
			"invalid market context: %w",
			err,
		)
	}

	marketJSON, err := json.Marshal(input.MarketContext)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal market context: %w",
			err,
		)
	}

	query := fmt.Sprintf(
		"研究标的：%s\n\n上游 market_context：\n%s\n\n请基于该市场环境，并自主获取与 as_of 对齐的标的数据，输出最终 Decision。",
		symbol,
		string(marketJSON),
	)

	iter := a.runner.Query(
		ctx,
		query,
		opts...,
	)

	var output string

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if event == nil {
			continue
		}

		if event.Err != nil {
			return nil, fmt.Errorf(
				"trading agent run failed: %w",
				event.Err,
			)
		}

		if event.Output == nil ||
			event.Output.MessageOutput == nil ||
			event.Output.MessageOutput.Message == nil {
			continue
		}

		text := strings.TrimSpace(
			assistantText(
				event.Output.MessageOutput.Message,
			),
		)

		if text != "" {
			output = text
		}
	}

	if output == "" {
		return nil, errors.New(
			"trading agent returned empty output",
		)
	}

	result, err := parseDecision(output)
	if err != nil {
		return nil, err
	}

	if result.Symbol != symbol {
		return nil, fmt.Errorf(
			"decision symbol %q does not match input symbol %q",
			result.Symbol,
			symbol,
		)
	}

	return result, nil
}

func parseDecision(
	raw string,
) (
	*decision.Decision,
	error,
) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New(
			"trading decision output is empty",
		)
	}

	var result decision.Decision

	if err := json.Unmarshal(
		[]byte(raw),
		&result,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid trading decision JSON: %w: output=%q",
			err,
			raw,
		)
	}

	if err := decision.Validate(&result); err != nil {
		return nil, fmt.Errorf(
			"invalid trading decision: %w",
			err,
		)
	}

	return &result, nil
}

func assistantText(
	msg *schema.AgenticMessage,
) string {
	if msg == nil ||
		msg.Role != schema.AgenticRoleTypeAssistant {
		return ""
	}

	for _, block := range msg.ContentBlocks {
		if block != nil &&
			block.Type == schema.ContentBlockTypeFunctionToolCall &&
			block.FunctionToolCall != nil {
			return ""
		}
	}

	var b strings.Builder

	for _, block := range msg.ContentBlocks {
		if block == nil ||
			block.Type != schema.ContentBlockTypeAssistantGenText ||
			block.AssistantGenText == nil {
			continue
		}

		b.WriteString(
			block.AssistantGenText.Text,
		)
	}

	return b.String()
}
