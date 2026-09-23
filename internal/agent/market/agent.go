package market

import (
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

type Agent struct {
	runner *adk.TypedRunner[*schema.AgenticMessage]
}

func New(
	ctx context.Context,
	cfg Config,
) (*Agent, error) {
	if cfg.Model == nil {
		return nil, errors.New(
			"market agent model is required",
		)
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 20
	}

	a, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "market_agent",
			Description: "Researches the overall market environment and produces structured market context.",
			Instruction: instruction,
			Model:       cfg.Model,

			Handlers: cfg.Handlers,

			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create market agent: %w",
			err,
		)
	}

	runner := adk.NewTypedRunner(
		adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent: a,

			// Market Agent 最终输出是结构化 JSON，
			// 当前没有必要使用 streaming。
			EnableStreaming: false,
		},
	)

	return &Agent{
		runner: runner,
	}, nil
}

func (a *Agent) Analyze(
	ctx context.Context,
	query string,
	opts ...adk.AgentRunOption,
) (*Context, error) {
	if a == nil || a.runner == nil {
		return nil, errors.New(
			"market agent is not initialized",
		)
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New(
			"market query is required",
		)
	}

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
				"market agent run failed: %w",
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
			// 只保存最后一个有效 assistant 输出。
			//
			// Agent Loop 中间可能发生多轮 Tool Calling，
			// 最后一个 assistant message 才是我们需要的
			// MarketContext JSON。
			output = text
		}
	}

	if output == "" {
		return nil, errors.New(
			"market agent returned empty output",
		)
	}

	result, err := parseContext(output)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func parseContext(
	raw string,
) (*Context, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return nil, errors.New(
			"market context output is empty",
		)
	}

	var result Context

	if err := json.Unmarshal(
		[]byte(raw),
		&result,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid market context JSON: %w: output=%q",
			err,
			raw,
		)
	}

	if err := Validate(&result); err != nil {
		return nil, fmt.Errorf(
			"invalid market context: %w",
			err,
		)
	}

	return &result, nil
}

func assistantText(
	msg *schema.AgenticMessage,
) string {
	if msg == nil {
		return ""
	}

	var b strings.Builder

	for _, block := range msg.ContentBlocks {
		if block == nil ||
			block.AssistantGenText == nil {
			continue
		}

		b.WriteString(
			block.AssistantGenText.Text,
		)
	}

	return b.String()
}
