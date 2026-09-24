package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	dsmlCallsPattern = regexp.MustCompile(
		`(?s)<DSML\s*(?:tool_calls|calls)\s*>(.*?)</DSML\s*(?:tool_calls|calls)\s*>`,
	)
	dsmlInvokePattern = regexp.MustCompile(
		`(?s)<DSML\s*invoke\s+name\s*=\s*"([^"]+)"\s*>(.*?)</DSML\s*invoke\s*>`,
	)
	dsmlParameterPattern = regexp.MustCompile(
		`(?s)<DSML\s*parameter\s+name\s*=\s*"([^"]+)"(?:\s+string\s*=\s*"(true|false)")?\s*>(.*?)</DSML\s*parameter\s*>`,
	)
)

const emptyMessageSanitizerText =
	"[System: Empty message content sanitised to satisfy protocol]"

type dsmlNormalizingModel struct {
	base model.BaseModel[*schema.AgenticMessage]
	seq  atomic.Uint64
}

func newDSMLNormalizingModel(
	base model.BaseModel[*schema.AgenticMessage],
) model.BaseModel[*schema.AgenticMessage] {
	return &dsmlNormalizingModel{
		base: base,
	}
}

func (m *dsmlNormalizingModel) Generate(
	ctx context.Context,
	input []*schema.AgenticMessage,
	opts ...model.Option,
) (
	*schema.AgenticMessage,
	error,
) {
	msg, err := m.base.Generate(
		ctx,
		input,
		opts...,
	)
	if err != nil || msg == nil {
		return msg, err
	}

	return m.normalize(msg)
}

func (m *dsmlNormalizingModel) Stream(
	ctx context.Context,
	input []*schema.AgenticMessage,
	opts ...model.Option,
) (
	*schema.StreamReader[*schema.AgenticMessage],
	error,
) {
	// Aegis currently runs Market/Trading agents in non-streaming mode.
	// DSML may span multiple stream chunks, so streaming normalization
	// should only be added when a streaming agent actually needs it.
	return m.base.Stream(
		ctx,
		input,
		opts...,
	)
}

func (m *dsmlNormalizingModel) normalize(
	msg *schema.AgenticMessage,
) (
	*schema.AgenticMessage,
	error,
) {
	if msg == nil {
		return nil, nil
	}

	var normalized []*schema.ContentBlock

	for _, block := range msg.ContentBlocks {
		if block == nil ||
			block.Type != schema.ContentBlockTypeAssistantGenText ||
			block.AssistantGenText == nil {
			normalized = append(normalized, block)
			continue
		}

		blocks, found, err := parseDSMLText(
			block.AssistantGenText.Text,
			func() string {
				return fmt.Sprintf(
					"dsml_call_%d",
					m.seq.Add(1),
				)
			},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"normalize DeepSeek DSML tool call: %w",
				err,
			)
		}

		if !found {
			normalized = append(normalized, block)
			continue
		}

		normalized = append(
			normalized,
			blocks...,
		)
	}

	msg.ContentBlocks = normalized

	return msg, nil
}

func parseDSMLText(
	text string,
	nextCallID func() string,
) (
	[]*schema.ContentBlock,
	bool,
	error,
) {
	normalized := normalizeDSMLMarkers(text)

	matches := dsmlCallsPattern.FindAllStringSubmatchIndex(
		normalized,
		-1,
	)
	if len(matches) == 0 {
		return nil, false, nil
	}

	var blocks []*schema.ContentBlock
	last := 0

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		appendAssistantText(
			&blocks,
			normalized[last:match[0]],
		)

		body := normalized[
			match[2]:match[3]
		]

		invokes := dsmlInvokePattern.FindAllStringSubmatch(
			body,
			-1,
		)
		if len(invokes) == 0 {
			return nil, true, errors.New(
				"DSML tool_calls block has no invoke",
			)
		}

		for _, invoke := range invokes {
			if len(invoke) < 3 {
				continue
			}

			name := strings.TrimSpace(
				invoke[1],
			)
			if name == "" {
				return nil, true, errors.New(
					"DSML invoke tool name is empty",
				)
			}

			args, err := parseDSMLArguments(
				invoke[2],
			)
			if err != nil {
				return nil, true, fmt.Errorf(
					"parse DSML invoke %q: %w",
					name,
					err,
				)
			}

			arguments, err := json.Marshal(args)
			if err != nil {
				return nil, true, fmt.Errorf(
					"marshal DSML invoke %q arguments: %w",
					name,
					err,
				)
			}

			blocks = append(
				blocks,
				&schema.ContentBlock{
					Type: schema.ContentBlockTypeFunctionToolCall,
					FunctionToolCall: &schema.FunctionToolCall{
						CallID:    nextCallID(),
						Name:      name,
						Arguments: string(arguments),
					},
				},
			)
		}

		last = match[1]
	}

	appendAssistantText(
		&blocks,
		normalized[last:],
	)

	return blocks, true, nil
}

func parseDSMLArguments(
	body string,
) (
	map[string]any,
	error,
) {
	args := make(map[string]any)

	matches := dsmlParameterPattern.FindAllStringSubmatch(
		body,
		-1,
	)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		name := strings.TrimSpace(
			match[1],
		)
		if name == "" {
			return nil, errors.New(
				"DSML parameter name is empty",
			)
		}

		isString := match[2] == "" ||
			match[2] == "true"

		if isString {
			args[name] = match[3]
			continue
		}

		var value any
		if err := json.Unmarshal(
			[]byte(strings.TrimSpace(match[3])),
			&value,
		); err != nil {
			return nil, fmt.Errorf(
				"parameter %q is not valid JSON: %w",
				name,
				err,
			)
		}

		args[name] = value
	}

	if strings.Contains(
		body,
		"<DSML",
	) && len(matches) == 0 {
		return nil, errors.New(
			"DSML invoke contains malformed parameter",
		)
	}

	return args, nil
}

func normalizeDSMLMarkers(
	text string,
) string {
	return strings.NewReplacer(
		"｜｜DSML｜｜",
		"DSML",
		"｜DSML｜",
		"DSML",
		"||DSML||",
		"DSML",
	).Replace(text)
}

func appendAssistantText(
	blocks *[]*schema.ContentBlock,
	text string,
) {
	text = strings.ReplaceAll(
		text,
		emptyMessageSanitizerText,
		"",
	)
	text = strings.TrimSpace(text)

	if text == "" {
		return
	}

	*blocks = append(
		*blocks,
		&schema.ContentBlock{
			Type: schema.ContentBlockTypeAssistantGenText,
			AssistantGenText: &schema.AssistantGenText{
				Text: text,
			},
		},
	)
}
