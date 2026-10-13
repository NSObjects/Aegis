package market

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeModel struct {
	output  string
	outputs []string
	calls   int
	err     error
}

func (m *fakeModel) Generate(
	_ context.Context,
	_ []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	if m.err != nil {
		return nil, m.err
	}

	m.calls++
	output := m.output
	if len(m.outputs) > 0 {
		if m.calls > len(m.outputs) {
			return nil, errors.New("unexpected extra market model call")
		}
		output = m.outputs[m.calls-1]
	}
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(
				&schema.AssistantGenText{
					Text: output,
				},
			),
		},
	}, nil
}

func (m *fakeModel) Stream(
	_ context.Context,
	_ []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New(
		"stream should not be called",
	)
}

func TestAgent_Analyze(t *testing.T) {
	ctx := context.Background()

	m := &fakeModel{
		output: `{
			"as_of": "20260922",
			"regime": "MIXED",
			"indexes": [
				{
					"name": "上证指数",
					"change_percent": 0.52
				},
				{
					"name": "沪深300",
					"change_percent": 0.31
				}
			],
			"advance_count": 2800,
			"decline_count": 2100,
			"limit_up_count": 61,
			"limit_down_count": 8,
			"turnover": 1850000000000,
			"leading_sectors": [
				{
					"name": "商业航天",
					"change_percent": 4.8
				}
			],
			"evidence": [
				"上涨家数高于下跌家数",
				"部分热点板块表现较强"
			],
			"risk": [
				"指数与板块表现存在一定分化"
			],
			"confidence": 0.72
		}`,
	}

	a, err := New(
		ctx,
		Config{
			Model: m,
		},
	)
	if err != nil {
		t.Fatalf(
			"create agent: %v",
			err,
		)
	}

	result, err := a.Analyze(
		ctx,
		"分析当前 A 股市场环境",
	)
	if err != nil {
		t.Fatalf(
			"analyze: %v",
			err,
		)
	}

	if result.AsOf != "20260922" {
		t.Fatalf(
			"unexpected as_of: %s",
			result.AsOf,
		)
	}

	if result.Regime != RegimeMixed {
		t.Fatalf(
			"unexpected regime: %s",
			result.Regime,
		)
	}

	if result.Confidence != 0.72 {
		t.Fatalf(
			"unexpected confidence: %f",
			result.Confidence,
		)
	}
}

func TestAgent_Analyze_InvalidJSON(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{
				output: "not json",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Analyze(
		ctx,
		"分析市场",
	)

	if err == nil {
		t.Fatal(
			"expected invalid json error",
		)
	}
}

func TestAgent_Analyze_ModelError(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{
				err: errors.New(
					"model unavailable",
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Analyze(
		ctx,
		"分析市场",
	)

	if err == nil {
		t.Fatal(
			"expected model error",
		)
	}
}

func TestAgent_Analyze_EmptyQuery(t *testing.T) {
	ctx := context.Background()

	a, err := New(
		ctx,
		Config{
			Model: &fakeModel{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Analyze(
		ctx,
		" ",
	)

	if err == nil {
		t.Fatal(
			"expected empty query error",
		)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   *Context
		wantErr bool
	}{
		{
			name: "valid",
			input: &Context{
				AsOf:       "20260922",
				Regime:     RegimeMixed,
				Evidence:   []string{"市场存在分化"},
				Risk:       []string{},
				Confidence: 0.72,
			},
		},
		{
			name:    "nil",
			input:   nil,
			wantErr: true,
		},
		{
			name: "missing as_of",
			input: &Context{
				Regime:     RegimeMixed,
				Evidence:   []string{"test"},
				Confidence: 0.5,
			},
			wantErr: true,
		},
		{
			name: "invalid regime",
			input: &Context{
				AsOf:       "20260922",
				Regime:     "BULL",
				Evidence:   []string{"test"},
				Confidence: 0.5,
			},
			wantErr: true,
		},
		{
			name: "empty evidence",
			input: &Context{
				AsOf:       "20260922",
				Regime:     RegimeRiskOn,
				Confidence: 0.7,
			},
			wantErr: true,
		},
		{
			name: "confidence too high",
			input: &Context{
				AsOf:       "20260922",
				Regime:     RegimeRiskOff,
				Evidence:   []string{"test"},
				Confidence: 1.1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				err := Validate(tt.input)

				if tt.wantErr && err == nil {
					t.Fatal(
						"expected error",
					)
				}

				if !tt.wantErr && err != nil {
					t.Fatalf(
						"unexpected error: %v",
						err,
					)
				}
			},
		)
	}
}

func TestAssistantTextIgnoresToolCallMessage(t *testing.T) {
	msg := &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(
				&schema.AssistantGenText{
					Text: "I will call a tool",
				},
			),
			schema.NewContentBlock(
				&schema.FunctionToolCall{
					CallID:    "call-1",
					Name:      "execute",
					Arguments: `{"command":"true"}`,
				},
			),
		},
	}

	if got := assistantText(msg); got != "" {
		t.Fatalf(
			"assistantText() = %q, want empty for tool-call message",
			got,
		)
	}
}

func TestAgentAnalyzeCorrectsMultipleJSONObjectsOnce(t *testing.T) {
	valid := `{"as_of":"20260922","regime":"MIXED","evidence":["index breadth improved"],"confidence":0.72}`
	invalid := `{"as_of":"20260921"}

Let me correct and output the final JSON.

` + valid
	m := &fakeModel{outputs: []string{invalid, valid}}
	a, err := New(context.Background(), Config{Model: m})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Analyze(context.Background(), "analyze market")
	if err != nil || got == nil || got.AsOf != "20260922" || m.calls != 2 {
		t.Fatalf("correction failed: result=%+v calls=%d err=%v", got, m.calls, err)
	}
}

func TestAgentAnalyzeRejectsRepeatedMalformedCorrection(t *testing.T) {
	m := &fakeModel{outputs: []string{"not json", "still not json"}}
	a, err := New(context.Background(), Config{Model: m})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Analyze(context.Background(), "analyze market")
	if err == nil || got != nil || m.calls != 2 {
		t.Fatalf("must reject malformed correction: result=%+v calls=%d err=%v", got, m.calls, err)
	}
}

func TestAgentAnalyzeNormalizesInvalidRegimeOnce(t *testing.T) {
	// JSON decodes but the regime value drifted off the enum; the
	// value-correction pass must repair it without touching facts.
	drifted := `{"as_of":"20261013","regime":"RISK-ON","evidence":["index breadth improved"],"confidence":0.72}`
	normalized := `{"as_of":"20261013","regime":"RISK_ON","evidence":["index breadth improved"],"confidence":0.72}`
	m := &fakeModel{outputs: []string{drifted, normalized}}
	a, err := New(context.Background(), Config{Model: m})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Analyze(context.Background(), "analyze market")
	if err != nil || got == nil || got.Regime != RegimeRiskOn || m.calls != 2 {
		t.Fatalf("value correction failed: result=%+v calls=%d err=%v", got, m.calls, err)
	}
}

func TestAgentAnalyzeNormalizesPercentConfidenceOnce(t *testing.T) {
	drifted := `{"as_of":"20261013","regime":"MIXED","evidence":["turnover expanded"],"confidence":85}`
	normalized := `{"as_of":"20261013","regime":"MIXED","evidence":["turnover expanded"],"confidence":0.85}`
	m := &fakeModel{outputs: []string{drifted, normalized}}
	a, err := New(context.Background(), Config{Model: m})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Analyze(context.Background(), "analyze market")
	if err != nil || got == nil || got.Confidence != 0.85 || m.calls != 2 {
		t.Fatalf("confidence correction failed: result=%+v calls=%d err=%v", got, m.calls, err)
	}
}

func TestAgentAnalyzeRejectsUnrecoverableValueCorrection(t *testing.T) {
	// Empty evidence cannot be repaired without inventing facts; the
	// corrector must return empty and the run must fail with the
	// value-correction error.
	drifted := `{"as_of":"20261013","regime":"MIXED","evidence":[],"confidence":0.5}`
	m := &fakeModel{outputs: []string{drifted, ""}}
	a, err := New(context.Background(), Config{Model: m})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Analyze(context.Background(), "analyze market")
	if err == nil || got != nil || m.calls != 2 {
		t.Fatalf("must reject unrecoverable value: result=%+v calls=%d err=%v", got, m.calls, err)
	}
	if !strings.Contains(err.Error(), "invalid market context after one value correction") {
		t.Fatalf("error = %v, want value-correction failure", err)
	}
}

func TestAgentAnalyzeValueCorrectsAfterFormatRepair(t *testing.T) {
	// Formatting repair (decode failure) followed by a validation failure
	// still gets the value-correction pass before giving up.
	withCommentary := "分析如下：\n" + `{"as_of":"20261013","regime":"NEUTRAL","evidence":["breadth flat"],"confidence":0.6}`
	wellFormed := `{"as_of":"20261013","regime":"NEUTRAL","evidence":["breadth flat"],"confidence":0.6}`
	normalized := `{"as_of":"20261013","regime":"MIXED","evidence":["breadth flat"],"confidence":0.6}`
	m := &fakeModel{outputs: []string{withCommentary, wellFormed, normalized}}
	a, err := New(context.Background(), Config{Model: m})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Analyze(context.Background(), "analyze market")
	if err != nil || got == nil || got.Regime != RegimeMixed || m.calls != 3 {
		t.Fatalf("chained correction failed: result=%+v calls=%d err=%v", got, m.calls, err)
	}
}
