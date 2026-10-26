package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type AgenticToolCallingAdapter struct {
	base  model.AgenticModel
	tools []*schema.ToolInfo
}

var _ model.ToolCallingChatModel = (*AgenticToolCallingAdapter)(nil)

func NewAgenticToolCallingAdapter(
	base model.AgenticModel,
) (*AgenticToolCallingAdapter, error) {
	if base == nil {
		return nil, errors.New(
			"agentic tool-calling adapter model is required",
		)
	}
	return &AgenticToolCallingAdapter{
		base: base,
	}, nil
}

func (a *AgenticToolCallingAdapter) WithTools(
	tools []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	if a == nil || a.base == nil {
		return nil, errors.New(
			"agentic tool-calling adapter is not initialized",
		)
	}

	cloned := make(
		[]*schema.ToolInfo,
		len(tools),
	)
	copy(cloned, tools)

	return &AgenticToolCallingAdapter{
		base:  a.base,
		tools: cloned,
	}, nil
}

func (a *AgenticToolCallingAdapter) Generate(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.Message, error) {
	if a == nil || a.base == nil {
		return nil, errors.New(
			"agentic tool-calling adapter is not initialized",
		)
	}

	agenticInput := make(
		[]*schema.AgenticMessage,
		0,
		len(input),
	)
	for index, message := range input {
		converted, err := classicMessageToAgentic(
			message,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"convert classic model input %d: %w",
				index,
				err,
			)
		}
		agenticInput = append(
			agenticInput,
			converted,
		)
	}

	agenticOpts := a.agenticOptions(opts)
	response, err := a.base.Generate(
		ctx,
		agenticInput,
		agenticOpts...,
	)
	if err != nil {
		return nil, err
	}

	return agenticMessageToClassic(response)
}

func (a *AgenticToolCallingAdapter) Stream(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	// The DeepSeek DSML normalizer currently normalizes tool calls only on
	// Generate. Eino PlanExecute internally calls the chat-model Stream API
	// even when its public run is non-streaming. Bridge that API with one
	// normalized Generate result instead of exposing an unnormalized DSML
	// stream.
	response, err := a.Generate(
		ctx,
		input,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return schema.StreamReaderFromArray(
		[]*schema.Message{response},
	), nil
}

func (a *AgenticToolCallingAdapter) agenticOptions(
	opts []model.Option,
) []model.Option {
	common := model.GetCommonOptions(
		nil,
		opts...,
	)

	forwarded := append(
		[]model.Option(nil),
		opts...,
	)
	forwarded = append(
		forwarded,
		model.WithTools(a.tools),
	)

	if common.ToolChoice != nil {
		choice := &schema.AgenticToolChoice{
			Type: *common.ToolChoice,
		}

		allowed := make(
			[]*schema.AllowedTool,
			0,
			len(common.AllowedToolNames),
		)
		for _, name := range common.AllowedToolNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			allowed = append(
				allowed,
				&schema.AllowedTool{
					FunctionName: name,
				},
			)
		}

		switch choice.Type {
		case schema.ToolChoiceAllowed:
			if len(allowed) > 0 {
				choice.Allowed =
					&schema.AgenticAllowedToolChoice{
						Tools: allowed,
					}
			}
		case schema.ToolChoiceForced:
			if len(allowed) > 0 {
				choice.Forced =
					&schema.AgenticForcedToolChoice{
						Tools: allowed,
					}
			}
		}

		forwarded = append(
			forwarded,
			model.WithAgenticToolChoice(choice),
		)
	}

	return forwarded
}

func classicMessageToAgentic(
	message *schema.Message,
) (*schema.AgenticMessage, error) {
	if message == nil {
		return nil, errors.New(
			"classic message is nil",
		)
	}
	if len(message.MultiContent) > 0 ||
		len(message.UserInputMultiContent) > 0 ||
		len(message.AssistantGenMultiContent) > 0 {
		return nil, errors.New(
			"agentic tool-calling adapter supports text-only classic messages",
		)
	}
	if strings.TrimSpace(
		message.ReasoningContent,
	) != "" {
		return nil, errors.New(
			"agentic tool-calling adapter does not accept classic reasoning content",
		)
	}

	switch message.Role {
	case schema.System:
		if len(message.ToolCalls) > 0 {
			return nil, errors.New(
				"system message cannot contain tool calls",
			)
		}
		return schema.SystemAgenticMessage(
			message.Content,
		), nil

	case schema.User:
		if len(message.ToolCalls) > 0 {
			return nil, errors.New(
				"user message cannot contain tool calls",
			)
		}
		return schema.UserAgenticMessage(
			message.Content,
		), nil

	case schema.Assistant:
		blocks := make(
			[]*schema.ContentBlock,
			0,
			1+len(message.ToolCalls),
		)
		if message.Content != "" {
			blocks = append(
				blocks,
				schema.NewContentBlock(
					&schema.AssistantGenText{
						Text: message.Content,
					},
				),
			)
		}
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(
				call.Function.Name,
			) == "" {
				return nil, errors.New(
					"assistant tool call name is required",
				)
			}
			blocks = append(
				blocks,
				schema.NewContentBlock(
					&schema.FunctionToolCall{
						CallID:    call.ID,
						Name:      call.Function.Name,
						Arguments: call.Function.Arguments,
					},
				),
			)
		}
		return &schema.AgenticMessage{
			Role:          schema.AgenticRoleTypeAssistant,
			ContentBlocks: blocks,
		}, nil

	case schema.Tool:
		if strings.TrimSpace(
			message.ToolName,
		) == "" {
			return nil, errors.New(
				"tool message tool_name is required",
			)
		}
		return &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeUser,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(
					&schema.FunctionToolResult{
						CallID: message.ToolCallID,
						Name:   message.ToolName,
						Content: []*schema.FunctionToolResultContentBlock{
							{
								Type: schema.FunctionToolResultContentBlockTypeText,
								Text: &schema.UserInputText{
									Text: message.Content,
								},
							},
						},
					},
				),
			},
		}, nil

	default:
		return nil, fmt.Errorf(
			"unsupported classic message role %q",
			message.Role,
		)
	}
}

func agenticMessageToClassic(
	message *schema.AgenticMessage,
) (*schema.Message, error) {
	if message == nil {
		return nil, errors.New(
			"agentic model returned nil message",
		)
	}

	switch message.Role {
	case schema.AgenticRoleTypeSystem,
		schema.AgenticRoleTypeUser:
		var text strings.Builder
		for _, block := range message.ContentBlocks {
			if block == nil {
				continue
			}
			if block.Type !=
				schema.ContentBlockTypeUserInputText ||
				block.UserInputText == nil {
				return nil, fmt.Errorf(
					"unsupported agentic %s block type %q",
					message.Role,
					block.Type,
				)
			}
			text.WriteString(
				block.UserInputText.Text,
			)
		}
		role := schema.User
		if message.Role ==
			schema.AgenticRoleTypeSystem {
			role = schema.System
		}
		return &schema.Message{
			Role:    role,
			Content: text.String(),
		}, nil

	case schema.AgenticRoleTypeAssistant:
		result := &schema.Message{
			Role: schema.Assistant,
		}
		var text strings.Builder
		for _, block := range message.ContentBlocks {
			if block == nil {
				continue
			}
			switch block.Type {
			case schema.ContentBlockTypeAssistantGenText:
				if block.AssistantGenText == nil {
					return nil, errors.New(
						"assistant text block is nil",
					)
				}
				text.WriteString(
					block.AssistantGenText.Text,
				)

			case schema.ContentBlockTypeFunctionToolCall:
				if block.FunctionToolCall == nil {
					return nil, errors.New(
						"function tool call block is nil",
					)
				}
				result.ToolCalls = append(
					result.ToolCalls,
					schema.ToolCall{
						ID:   block.FunctionToolCall.CallID,
						Type: "function",
						Function: schema.FunctionCall{
							Name: block.
								FunctionToolCall.
								Name,
							Arguments: block.
								FunctionToolCall.
								Arguments,
						},
					},
				)

			case schema.ContentBlockTypeReasoning:
				// Planner/Replanner do not consume reasoning. Keep it out
				// of the classic content instead of changing the structured
				// tool-call protocol.
				continue

			default:
				return nil, fmt.Errorf(
					"unsupported agentic assistant block type %q",
					block.Type,
				)
			}
		}

		if message.ResponseMeta != nil &&
			message.ResponseMeta.TokenUsage != nil {
			result.ResponseMeta =
				&schema.ResponseMeta{
					Usage: message.
						ResponseMeta.
						TokenUsage,
				}
		}

		result.Content = text.String()
		return result, nil

	default:
		return nil, fmt.Errorf(
			"unsupported agentic role %q",
			message.Role,
		)
	}
}
