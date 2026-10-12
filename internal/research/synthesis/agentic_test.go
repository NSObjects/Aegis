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
	responses []*schema.AgenticMessage
	err      error
	input    []*schema.AgenticMessage
	calls int
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
	if m.err != nil { return nil, m.err }
	m.calls++
	if len(m.responses) >= m.calls { return m.responses[m.calls-1], nil }
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

func TestAgenticSynthesizerAcceptsJSONModeTypeMetadata(t *testing.T) {
 synthesizer := &AgenticSynthesizer{Model:&synthesisModel{response:synthesisAssistantMessage(
  `{"type":"json_object","summary":"跨域证据有限。","risks":["技术数据缺失"],"confidence":0.42}`,
 )}}
 artifact,err:=synthesizer.Synthesize(context.Background(),validSynthesisInput())
 if err!=nil { t.Fatalf("Synthesize() error = %v",err) }
 if artifact.Synthesis.Confidence!=0.42 { t.Fatalf("confidence = %v",artifact.Synthesis.Confidence) }
}

func TestAgenticSynthesizerRejectsUnexpectedTypeMetadata(t *testing.T) {
 synthesizer := &AgenticSynthesizer{Model:&synthesisModel{response:synthesisAssistantMessage(
  `{"type":"other","summary":"x","confidence":0.5}`,
 )}}
 _,err:=synthesizer.Synthesize(context.Background(),validSynthesisInput())
 if err==nil || !strings.Contains(err.Error(),"invalid type metadata") { t.Fatalf("error = %v",err) }
}

func TestAgenticSynthesizerRepairsInvalidConflictOnce(t *testing.T) {
 m:=&synthesisModel{responses:[]*schema.AgenticMessage{
  synthesisAssistantMessage(`{"summary":"x","conflicts":[{"description":"bad","references":[{"domain":"EVENT","event_id":"event_contract"},{"domain":"EVENT","event_id":"event_contract"}]}],"confidence":0.5}`),
  synthesisAssistantMessage(`{"summary":"x","conflicts":[{"description":"fixed","references":[{"domain":"TECHNICAL"},{"domain":"EVENT","event_id":"event_contract"}]}],"confidence":0.5}`),
 }}
 artifact,err:=(&AgenticSynthesizer{Model:m}).Synthesize(context.Background(),validSynthesisInput())
 if err!=nil { t.Fatalf("Synthesize() error = %v",err) }
 if m.calls!=2 { t.Fatalf("calls=%d, want 2",m.calls) }
 if artifact.Synthesis.Conflicts[0].Description!="fixed" { t.Fatalf("conflict=%+v",artifact.Synthesis.Conflicts[0]) }
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

func TestAgenticSynthesizerRepairOmitsUnsupportedSingleDomainConflict(t *testing.T) {
 m:=&synthesisModel{responses:[]*schema.AgenticMessage{
  synthesisAssistantMessage(`{"summary":"uncertainty","conflicts":[{"description":"single-domain claim","references":[{"domain":"MARKET"},{"domain":"MARKET"}]}],"confidence":0.5}`),
  synthesisAssistantMessage(`{"summary":"uncertainty","conflicts":[],"risks":["The market-only assertion is not a cross-domain conflict"],"confidence":0.5}`),
 }}
 artifact,err:=(&AgenticSynthesizer{Model:m}).Synthesize(context.Background(),validSynthesisInput())
 if err!=nil{t.Fatal(err)}
 if m.calls!=2 || len(artifact.Synthesis.Conflicts)!=0 {t.Fatalf("must remove unsupported conflict without inventing evidence: calls=%d artifact=%+v",m.calls,artifact.Synthesis)}
 if !strings.Contains(agenticTestText(m.input[len(m.input)-1]),"Do not attach an unrelated domain citation") {
  t.Fatal("repair prompt must forbid fabricated cross-domain support")
 }
}

func TestAgenticSynthesizerOmitsUnsupportedSameDomainAfterRepair(t *testing.T) {
 raw := `{"summary":"original summary","conflicts":[{"description":"same-domain candidate","references":[{"domain":"MARKET"},{"domain":"MARKET"}]},{"description":"valid cross-domain disagreement","references":[{"domain":"TECHNICAL"},{"domain":"EVENT","event_id":"event_contract"}]}],"risks":["original risk"],"confidence":0.5}`
 m := &synthesisModel{response:synthesisAssistantMessage(raw)}
 input:=validSynthesisInput()
 result,err:=(&AgenticSynthesizer{Model:m}).Synthesize(context.Background(),input)
 if err!=nil {t.Fatal(err)}
 if m.calls!=2 {t.Fatalf("calls=%d, want correction attempt",m.calls)}
 if len(result.Synthesis.Conflicts)!=1 || result.Synthesis.Conflicts[0].Description!="valid cross-domain disagreement" {
  t.Fatalf("valid cross-domain conflict must survive, got %+v",result.Synthesis.Conflicts)
 }
 if len(result.Synthesis.Risks)!=2 || result.Synthesis.Risks[0]!="original risk" || !strings.Contains(result.Synthesis.Risks[1],"已经移除") {
  t.Fatalf("omission must be visible as an explicit limitation, risks=%v",result.Synthesis.Risks)
 }
 if result.Market!=input.Market || result.Technical!=input.Technical || result.Event!=input.Event {
  t.Fatal("upstream contexts must remain untouched")
 }
 if err:=Validate(result);err!=nil {t.Fatalf("repaired artifact must pass original validator: %v",err)}
}

func TestAgenticSynthesizerCannotDropInvalidReferenceAsSingleDomain(t *testing.T) {
 raw := `{"summary":"x","conflicts":[{"description":"invalid event reference","references":[{"domain":"EVENT","event_id":"unknown"},{"domain":"EVENT","event_id":"unknown"}]}],"confidence":0.5}`
 m:=&synthesisModel{response:synthesisAssistantMessage(raw)}
 artifact,err:=(&AgenticSynthesizer{Model:m}).Synthesize(context.Background(),validSynthesisInput())
 if err==nil || artifact!=nil || !strings.Contains(err.Error(),"does not exist in EventContext") {
  t.Fatalf("invalid reference cannot be omitted to claim success: artifact=%+v err=%v",artifact,err)
 }
 if m.calls!=2 {t.Fatalf("calls=%d, want no extra retries",m.calls)}
}

func TestAgenticSynthesizerDoesNotOmitMalformedConflict(t *testing.T) {
 raw:=`{"summary":"x","conflicts":[{"description":"","references":[{"domain":"MARKET"},{"domain":"MARKET"}]}],"confidence":0.5}`
 m:=&synthesisModel{response:synthesisAssistantMessage(raw)}
 artifact,err:=(&AgenticSynthesizer{Model:m}).Synthesize(context.Background(),validSynthesisInput())
 if err==nil||artifact!=nil||!strings.Contains(err.Error(),"description is required") {
  t.Fatalf("other invalid contracts must fail closed: artifact=%+v err=%v",artifact,err)
 }
}
