package event

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type semanticMatcherModel struct {
	response *schema.AgenticMessage
	err      error
	input    []*schema.AgenticMessage
}

func (m *semanticMatcherModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.input = input
	return m.response, m.err
}

func (m *semanticMatcherModel) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New("not implemented")
}

func TestAgenticSemanticEvidenceMatcherAcceptsJSONModeWrapper(
	t *testing.T,
) {
	matcher := &AgenticSemanticEvidenceMatcher{
		Model: &semanticMatcherModel{
			response: agenticTextMessage(
				schema.AgenticRoleTypeAssistant,
				`{"type":"json_object","matches":[{"raw_document_id":"candidate-1","relevant":false,"confidence":0.99,"reason":"unrelated"}]}`,
			),
		},
	}

	matches, err := matcher.Match(
		context.Background(),
		EvidenceTarget{
			Symbol: "600519.SH",
			Name:   "贵州茅台",
		},
		[]RawDocument{
			{
				ID: "candidate-1",
			},
		},
	)
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if len(matches) != 1 ||
		matches[0].RawDocumentID != "candidate-1" ||
		matches[0].Relevant {
		t.Fatalf("matches = %+v", matches)
	}
}

func TestAgenticSemanticEvidenceMatcher(t *testing.T) {
	model := &semanticMatcherModel{
		response: agenticTextMessage(
			schema.AgenticRoleTypeAssistant,
			`{"matches":[{"raw_document_id":"candidate-1","relevant":true,"confidence":0.91,"reason":"directly concerns target"}]}`,
		),
	}
	matcher := &AgenticSemanticEvidenceMatcher{
		Model: model,
	}

	matches, err := matcher.Match(
		context.Background(),
		EvidenceTarget{
			Symbol: "600519.SH",
			Name:   "贵州茅台",
		},
		[]RawDocument{
			{
				ID:        "candidate-1",
				Title:     "candidate title",
				Content:   "candidate body",
				Publisher: "财联社",
			},
		},
	)
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf(
			"matches = %d, want 1",
			len(matches),
		)
	}
	if !matches[0].Relevant ||
		matches[0].Confidence != 0.91 {
		t.Fatalf(
			"match = %+v",
			matches[0],
		)
	}
	if len(model.input) != 2 {
		t.Fatalf(
			"model input messages = %d, want 2",
			len(model.input),
		)
	}
	if model.input[0].Role !=
		schema.AgenticRoleTypeSystem {
		t.Fatalf(
			"first role = %q, want system",
			model.input[0].Role,
		)
	}
}

func TestAgenticSemanticEvidenceMatcherRejectsInvalidOutput(
	t *testing.T,
) {
	tests := []struct {
		name     string
		response *schema.AgenticMessage
		err      error
		want     string
	}{
		{
			name: "model error",
			err:  errors.New("provider failed"),
			want: "provider failed",
		},
		{
			name: "empty output",
			response: agenticTextMessage(
				schema.AgenticRoleTypeAssistant,
				"",
			),
			want: "empty output",
		},
		{
			name: "invalid json",
			response: agenticTextMessage(
				schema.AgenticRoleTypeAssistant,
				"not json",
			),
			want: "invalid semantic evidence matcher JSON",
		},
		{
			name: "unsupported response type",
			response: agenticTextMessage(
				schema.AgenticRoleTypeAssistant,
				`{"type":"text","matches":[]}`,
			),
			want: "unsupported type",
		},
		{
			name: "unknown field",
			response: agenticTextMessage(
				schema.AgenticRoleTypeAssistant,
				`{"matches":[],"unexpected":true}`,
			),
			want: "unknown field",
		},
		{
			name: "trailing json",
			response: agenticTextMessage(
				schema.AgenticRoleTypeAssistant,
				`{"matches":[]} {"matches":[]}`,
			),
			want: "multiple JSON values",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher := &AgenticSemanticEvidenceMatcher{
				Model: &semanticMatcherModel{
					response: test.response,
					err:      test.err,
				},
			}

			_, err := matcher.Match(
				context.Background(),
				EvidenceTarget{
					Symbol: "600519.SH",
				},
				[]RawDocument{
					{
						ID: "candidate",
					},
				},
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
