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
	model model.BaseModel[*schema.AgenticMessage]
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
		model: cfg.Model,
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

	candidate, decodeErr := decodeContext(output)
	if decodeErr != nil {
		// A malformed final assistant message gets one formatting-only
		// correction. Source values are preserved; provider/tool failures
		// are not retried.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		repaired, repairErr := a.model.Generate(ctx, []*schema.AgenticMessage{
			schema.SystemAgenticMessage("You are a strict JSON formatting repairer. Return exactly one valid Aegis MarketContext JSON object using only the provided output. Remove commentary, incomplete duplicate objects and markdown. Never invent facts or alter source values. If facts are insufficient, return an empty response."),
			schema.UserAgenticMessage("Correct the following Market Agent output into one complete JSON object only:\n" + output),
		})
		if repairErr != nil {
			return nil, fmt.Errorf("market context correction failed: %w", repairErr)
		}
		output = assistantText(repaired)
		candidate, decodeErr = decodeContext(output)
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid market context after one correction: %w", decodeErr)
		}
	}

	validationErr := Validate(candidate)
	if validationErr == nil {
		return candidate, nil
	}

	// The JSON decoded but a deterministic field value is illegal (enum or
	// range drift, e.g. "RISK-ON" or confidence 85). The formatting repairer
	// is forbidden to alter values, so replay the exact validation error and
	// allow only normalizing the offending value to the nearest legal one.
	// Facts, dates and evidence must never be invented; when that would be
	// required the corrector returns an empty response and the run fails.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	repaired, repairErr := a.model.Generate(ctx, []*schema.AgenticMessage{
		schema.SystemAgenticMessage(valueCorrectionInstruction),
		schema.UserAgenticMessage("确定性校验错误: " + validationErr.Error() + "\n将下列 Market Agent 输出中被校验拒绝的字段值修复为最接近的合法值，并返回完整 JSON 对象：\n" + output),
	})
	if repairErr != nil {
		return nil, fmt.Errorf("market context value correction failed: %w", repairErr)
	}
	normalized, normalizedErr := parseContext(assistantText(repaired))
	if normalizedErr != nil {
		return nil, fmt.Errorf("invalid market context after one value correction: %w", normalizedErr)
	}
	return normalized, nil
}

// valueCorrectionInstruction constrains the value-level repair pass to the
// deterministic Validate rules; it mirrors the enum-correction contract the
// event analyst already uses for impact mechanisms.
const valueCorrectionInstruction = `你是严格的 Aegis MarketContext 值修复器。输入的 JSON 已通过语法解析，但未通过确定性校验。只允许把校验错误指出的非法字段值改为最接近的合法值：regime 只能使用 RISK_ON、MIXED、RISK_OFF；confidence 必须归一到 0 到 1 之间（大于 1 且不超过 100 的数值按百分数换算）。除此之外不得添加、删除或改写任何字段，不得编造日期、证据或数据。如果无法在不编造事实的前提下完成修复，只返回空输出。只输出一个完整 JSON 对象，不要输出 JSON 之外的任何文字，不要使用 Markdown。`

func decodeContext(
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

	return &result, nil
}

func parseContext(
	raw string,
) (*Context, error) {
	result, err := decodeContext(raw)
	if err != nil {
		return nil, err
	}

	if err := Validate(result); err != nil {
		return nil, fmt.Errorf(
			"invalid market context: %w",
			err,
		)
	}

	return result, nil
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
