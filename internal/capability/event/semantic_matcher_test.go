package event

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"math"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type semanticMatcherModel struct {
	response  *schema.AgenticMessage
	responses []*schema.AgenticMessage
	err       error
	input     []*schema.AgenticMessage
	inputs    [][]*schema.AgenticMessage
	calls     int
}

func (m *semanticMatcherModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.input = input
	m.inputs = append(m.inputs, input)
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if len(m.responses) > 0 {
		if m.calls > len(m.responses) {
			return nil, errors.New("unexpected semantic model request")
		}
		return m.responses[m.calls-1], nil
	}
	return m.response, nil
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

func TestSemanticMatcherBatchesAndRetriesOnlyMissingIDs(t *testing.T) {
	model := &semanticMatcherModel{responses: []*schema.AgenticMessage{
		agenticTextMessage(schema.AgenticRoleTypeAssistant,
			`{"matches":[{"raw_document_id":"a","relevant":true,"confidence":0.94,"reason":"direct"},{"raw_document_id":"c","relevant":false,"confidence":0.92,"reason":"other"}]}`),
		agenticTextMessage(schema.AgenticRoleTypeAssistant,
			`{"matches":[{"raw_document_id":"b","relevant":true,"confidence":0.81,"reason":"direct"}]}`),
		agenticTextMessage(schema.AgenticRoleTypeAssistant,
			`{"matches":[{"raw_document_id":"d","relevant":false,"confidence":0.90,"reason":"no target linkage"}]}`),
		agenticTextMessage(schema.AgenticRoleTypeAssistant,
			`{"matches":[{"raw_document_id":"e","relevant":false,"confidence":0.99,"reason":"other"},{"raw_document_id":"f","relevant":true,"confidence":0.85,"reason":"direct"}]}`),
	}}
	selector := &HybridEvidenceSelector{
		TargetResolver: MapEvidenceTargetResolver{
			"600519.SH": {Name: "贵州茅台"},
		},
		SemanticMatcher: &AgenticSemanticEvidenceMatcher{Model: model},
	}
	documents := []RawDocument{
		{ID: "a", Title: "supplier contract"},
		{ID: "b", Title: "supply order"},
		{ID: "c", Title: "industry news"},
		{ID: "d", Title: "market update"},
		{ID: "e", Title: "technology headline"},
		{ID: "f", Title: "customer story"},
	}
	decisions, err := selector.Select(
		context.Background(), ResearchTask{Symbol: "600519.SH"}, documents,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(decisions) != len(documents) {
		t.Fatalf("decisions = %d, want %d", len(decisions), len(documents))
	}
	if model.calls != 4 {
		t.Fatalf("calls = %d, want 4", model.calls)
	}
	wantBatches := [][]string{{"a", "b", "c", "d"}, {"b"}, {"d"}, {"e", "f"}}
	for i, messages := range model.inputs {
		var request semanticEvidencePrompt
		if err := json.Unmarshal([]byte(agenticAssistantOrUserText(messages[1])), &request); err != nil {
			t.Fatalf("decode batch %d input: %v", i, err)
		}
		ids := make([]string, 0, len(request.Documents))
		for _, item := range request.Documents {
			ids = append(ids, item.RawDocumentID)
		}
		if !reflect.DeepEqual(ids, wantBatches[i]) {
			t.Fatalf("batch %d ids=%v, want %v", i, ids, wantBatches[i])
		}
	}
	byID := make(map[string]EvidenceSelectionDecision)
	for _, d := range decisions {
		byID[d.RawDocumentID] = d
	}
	for _, id := range []string{"a", "b", "f"} {
		if !byID[id].Selected {
			t.Fatalf("grounded evidence %q was excluded", id)
		}
	}
	for _, id := range []string{"c", "d", "e"} {
		if byID[id].Selected || byID[id].Method != SelectionSemantic {
			t.Fatalf("rejected evidence %q was accepted: %+v", id, byID[id])
		}
	}
}

func TestSemanticMatcherPersistentOmissionStopsResearch(t *testing.T) {
	model := &semanticMatcherModel{responses: []*schema.AgenticMessage{
		agenticTextMessage(schema.AgenticRoleTypeAssistant,
			`{"matches":[{"raw_document_id":"a","relevant":true,"confidence":0.9,"reason":"direct"}]}`),
		agenticTextMessage(schema.AgenticRoleTypeAssistant,
			`{"matches":[]}`),
	}}
	matcher := &AgenticSemanticEvidenceMatcher{Model: model}
	matches, err := matcher.Match(
		context.Background(), EvidenceTarget{Symbol: "600519.SH"},
		[]RawDocument{{ID: "a"}, {ID: "cls_news_2498111"}},
	)
	if err == nil || !strings.Contains(err.Error(), "research incomplete") ||
		!strings.Contains(err.Error(), "cls_news_2498111") {
		t.Fatalf("error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("incomplete result must not be returned: %+v", matches)
	}
	if model.calls != 2 {
		t.Fatalf("calls = %d, want 2", model.calls)
	}
}

func TestSemanticMatcherRejectsUnknownBatchIDBeforeRetry(t *testing.T) {
	model := &semanticMatcherModel{response: agenticTextMessage(schema.AgenticRoleTypeAssistant,
		`{"matches":[{"raw_document_id":"fabricated","relevant":true,"confidence":0.98,"reason":"fabricated linkage"}]}`)}
	matcher := &AgenticSemanticEvidenceMatcher{Model:model}
	_,err := matcher.Match(context.Background(),EvidenceTarget{Symbol:"600519.SH"},
		[]RawDocument{{ID:"a"},{ID:"b"}})
	if err == nil || !strings.Contains(err.Error(),"unknown raw document") {
		t.Fatalf("error = %v",err)
	}
	if model.calls != 1 {
		t.Fatalf("invalid IDs must fail before retry; calls=%d",model.calls)
	}
}

func agenticAssistantOrUserText(msg *schema.AgenticMessage) string {
	if msg == nil { return "" }
	var b strings.Builder
	for _, block := range msg.ContentBlocks {
		if block!=nil && block.UserInputText!=nil {b.WriteString(block.UserInputText.Text)}
	}
	return b.String()
}

func TestSemanticMatcherRequiresExplicitClassificationFields(t *testing.T) {
	cases := []struct {
		name, output, want string
	}{
		{"missing relevant", `{"matches":[{"raw_document_id":"candidate","confidence":0.9,"reason":"no issuer relation"}]}`, "explicitly provide relevant"},
		{"missing confidence", `{"matches":[{"raw_document_id":"candidate","relevant":false,"reason":"no issuer relation"}]}`, "explicitly provide relevant"},
		{"missing reason", `{"matches":[{"raw_document_id":"candidate","relevant":false,"confidence":0.9}]}`, "explicitly provide relevant"},
		{"null relevant", `{"matches":[{"raw_document_id":"candidate","relevant":null,"confidence":0.9,"reason":"no issuer relation"}]}`, "explicitly provide relevant"},
		{"null confidence", `{"matches":[{"raw_document_id":"candidate","relevant":false,"confidence":null,"reason":"no issuer relation"}]}`, "explicitly provide relevant"},
		{"empty reason", `{"matches":[{"raw_document_id":"candidate","relevant":false,"confidence":0.9,"reason":"  "}]}`, "explicitly provide relevant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &AgenticSemanticEvidenceMatcher{Model: &semanticMatcherModel{
				response: agenticTextMessage(schema.AgenticRoleTypeAssistant, tc.output),
			}}
			result, err := m.Match(context.Background(), EvidenceTarget{Symbol:"600519.SH"},
				[]RawDocument{{ID:"candidate", Title:"industry headlines"}})
			if err == nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("error=%v, want %q",err,tc.want)
			}
			if len(result)>0 {t.Fatalf("malformed relevance must never yield evidence: %+v",result)}
		})
	}
}

func TestSemanticMatcherRejectsNonfiniteAndMissingReasonsFromCustomMatcher(t *testing.T) {
	for _, tc := range []struct {
		name string
		confidence float64
		reason string
		want string
	}{
		{"nan", math.NaN(), "explicitly irrelevant", "between 0 and 1"},
		{"positive infinity", math.Inf(1), "explicitly irrelevant", "between 0 and 1"},
		{"negative infinity", math.Inf(-1), "explicitly irrelevant", "between 0 and 1"},
		{"missing reason", .8, "", "evidence-based reason"},
	} {
		t.Run(tc.name, func(t *testing.T){
			_, err := validateSemanticMatches([]RawDocument{{ID:"candidate"}}, []SemanticEvidenceMatch{{
				RawDocumentID:"candidate", Relevant:false, Confidence:tc.confidence, Reason:tc.reason,
			}}, .8)
			if err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("error=%v, want %q",err,tc.want)
			}
		})
	}
}
