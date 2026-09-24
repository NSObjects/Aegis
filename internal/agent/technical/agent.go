package technical

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	Model model.BaseModel[*schema.AgenticMessage]

	Tools []tool.BaseTool

	Handlers []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]

	MaxIterations int
}

type Input struct {
	Symbol        string
	RequestedAsOf string
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
			"technical agent model is required",
		)
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 20
	}

	a, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "technical_agent",
			Description: "Researches symbol-specific price action and technical state, then produces structured technical context.",
			Instruction: instruction,
			Model:       cfg.Model,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: cfg.Tools,
				},
			},
			Handlers: cfg.Handlers,

			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create technical agent: %w",
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
) (*Context, error) {
	if a == nil || a.runner == nil {
		return nil, errors.New(
			"technical agent is not initialized",
		)
	}

	symbol := strings.TrimSpace(input.Symbol)
	if symbol == "" {
		return nil, errors.New(
			"technical symbol is required",
		)
	}

	requestedAsOf := strings.TrimSpace(input.RequestedAsOf)
	if requestedAsOf == "" {
		return nil, errors.New(
			"technical requested_as_of is required",
		)
	}

	query := fmt.Sprintf(
		"研究标的：%s\nrequested_as_of：%s\n\n请获取与该时间点一致的真实个股技术数据，并输出 TechnicalContext。",
		symbol,
		requestedAsOf,
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
				"technical agent run failed: %w",
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
			"technical agent returned empty output",
		)
	}

	result, err := parseContext(output)
	if err != nil {
		return nil, err
	}

	if result.Symbol != symbol {
		return nil, fmt.Errorf(
			"technical context symbol %q does not match input symbol %q",
			result.Symbol,
			symbol,
		)
	}

	if result.RequestedAsOf != requestedAsOf {
		return nil, fmt.Errorf(
			"technical context requested_as_of %q does not match input requested_as_of %q",
			result.RequestedAsOf,
			requestedAsOf,
		)
	}

	return result, nil
}

func parseContext(
	raw string,
) (*Context, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New(
			"technical context output is empty",
		)
	}

	var result Context

	if err := json.Unmarshal(
		[]byte(raw),
		&result,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid technical context JSON: %w: output=%q",
			err,
			raw,
		)
	}

	if err := Validate(&result); err != nil {
		return nil, fmt.Errorf(
			"invalid technical context: %w",
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
