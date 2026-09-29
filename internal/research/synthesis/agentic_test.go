package synthesis

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type synthesisModel struct {
	response *schema.AgenticMessage
	err      error
	input    []*schema.AgenticMessage
}

func (m *synthesisModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.input = append(
		[]*schema.AgenticMessage(nil),
		input...,
	)
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *synthesisModel) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New("not implemented")
}

func synthesisAssistantMessage(
	text string,
) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(
				&schema.AssistantGenText{
					Text: text,
				},
			),
		},
	}
}

func TestAgenticSynthesizerProducesValidatedArtifact(
	t *testing.T,
) {
	input := validSynthesisInput()
	model := &synthesisModel{
		response: synthesisAssistantMessage(
			`{
				"summary":"市场环境偏中性，技术确认不足，而合同事件提供独立的公司层面信息。",
				"agreements":[
					{
						"claim":"市场与技术上下文都没有形成强风险偏好确认。",
						"references":[
							{"domain":"MARKET"},
							{"domain":"TECHNICAL"}
						]
					}
				],
				"conflicts":[
					{
						"description":"事件解释偏正向，但技术面尚未确认。",
						"references":[
							{"domain":"TECHNICAL"},
							{"domain":"EVENT","event_id":"event_contract"}
						]
					}
				],
				"risks":["合同履约和技术确认仍存在不确定性"],
				"confidence":0.79
			}`,
		),
	}

	synthesizer := &AgenticSynthesizer{
		Model: model,
	}
	artifact, err := synthesizer.Synthesize(
		context.Background(),
		input,
	)
	if err != nil {
		t.Fatalf(
			"Synthesize() error = %v",
			err,
		)
	}
	if artifact.Symbol != "600519.SH" {
		t.Fatalf(
			"symbol = %q",
			artifact.Symbol,
		)
	}
	if artifact.Market != input.Market ||
		artifact.Technical != input.Technical ||
		artifact.Event != input.Event {
		t.Fatal(
			"artifact must retain the original typed upstream contexts",
		)
	}
	if len(artifact.Synthesis.Conflicts) != 1 ||
		artifact.Synthesis.Confidence != 0.79 {
		t.Fatalf(
			"synthesis = %+v",
			artifact.Synthesis,
		)
	}

	if len(model.input) != 2 {
		t.Fatalf(
			"model input messages = %d, want 2",
			len(model.input),
		)
	}
	if model.input[0].Role !=
		schema.AgenticRoleTypeSystem ||
		model.input[1].Role !=
			schema.AgenticRoleTypeUser {
		t.Fatalf(
			"model roles = %q / %q",
			model.input[0].Role,
			model.input[1].Role,
		)
	}
	userText := agenticTestText(
		model.input[1],
	)
	if !strings.Contains(
		userText,
		`"requested_as_of":"2026-09-29T15:00:00Z"`,
	) {
		t.Fatalf(
			"model input does not preserve event temporal precision: %s",
			userText,
		)
	}
}

func TestAgenticSynthesizerRejectsInvalidModelOutput(
	t *testing.T,
) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name: "unknown field",
			response: `{"summary":"x","confidence":0.5,"unknown":true}`,
			want: "unknown field",
		},
		{
			name: "trailing json",
			response: `{"summary":"x","confidence":0.5} {"summary":"y","confidence":0.5}`,
			want: "multiple JSON values",
		},
		{
			name: "unknown event reference",
			response: `{
				"summary":"x",
				"agreements":[
					{
						"claim":"x",
						"references":[
							{"domain":"EVENT","event_id":"event_unknown"}
						]
					}
				],
				"confidence":0.5
			}`,
			want: "does not exist in EventContext",
		},
		{
			name: "single-domain conflict",
			response: `{
				"summary":"x",
				"conflicts":[
					{
						"description":"x",
						"references":[
							{"domain":"MARKET"},
							{"domain":"MARKET"}
						]
					}
				],
				"confidence":0.5
			}`,
			want: "two distinct domains",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synthesizer := &AgenticSynthesizer{
				Model: &synthesisModel{
					response: synthesisAssistantMessage(
						test.response,
					),
				},
			}
			_, err := synthesizer.Synthesize(
				context.Background(),
				validSynthesisInput(),
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					test.want,
				) {
				t.Fatalf(
					"error = %v, want substring %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestAgenticSynthesizerStopsBeforeModelOnInvalidInput(
	t *testing.T,
) {
	input := validSynthesisInput()
	input.Technical.Symbol = "000001.SZ"

	model := &synthesisModel{
		response: synthesisAssistantMessage(
			`{"summary":"must not run","confidence":0.5}`,
		),
	}
	synthesizer := &AgenticSynthesizer{
		Model: model,
	}

	_, err := synthesizer.Synthesize(
		context.Background(),
		input,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"technical context symbol",
		) {
		t.Fatalf(
			"error = %v, want input validation error",
			err,
		)
	}
	if len(model.input) != 0 {
		t.Fatal(
			"model must not run when upstream contexts are invalid",
		)
	}
}

func TestAgenticSynthesizerPropagatesModelError(
	t *testing.T,
) {
	synthesizer := &AgenticSynthesizer{
		Model: &synthesisModel{
			err: errors.New(
				"provider unavailable",
			),
		},
	}

	_, err := synthesizer.Synthesize(
		context.Background(),
		validSynthesisInput(),
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"provider unavailable",
		) {
		t.Fatalf(
			"error = %v, want provider error",
			err,
		)
	}
}

func agenticTestText(
	message *schema.AgenticMessage,
) string {
	if message == nil {
		return ""
	}
	var builder strings.Builder
	for _, block := range message.ContentBlocks {
		if block == nil ||
			block.UserInputText == nil {
			continue
		}
		builder.WriteString(
			block.UserInputText.Text,
		)
	}
	return builder.String()
}
