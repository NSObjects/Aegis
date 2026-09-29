package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type eventModel struct {
	responses []*schema.AgenticMessage
	err       error
	inputs    [][]*schema.AgenticMessage
	index     int
}

func (m *eventModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.inputs = append(
		m.inputs,
		append(
			[]*schema.AgenticMessage(nil),
			input...,
		),
	)
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.responses) {
		return nil, errors.New(
			"no scripted response",
		)
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

func (m *eventModel) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New("not implemented")
}

func assistantJSON(text string) *schema.AgenticMessage {
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

func TestAgenticEventUnderstandingStages(t *testing.T) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	available := asOf.Add(-time.Hour)

	model := &eventModel{
		responses: []*schema.AgenticMessage{
			assistantJSON(
				`{"facts":[{"type":"CONTRACT","occurred_at":"2026-09-29T09:00:00Z","headline":"合同事件","summary":"公司披露签订合同。","entities":["目标公司"],"evidence_quotes":[{"document_id":"doc_1","quote":"公司披露签订合同。"}],"confidence":0.95}],"confidence":0.94}`,
			),
			assistantJSON(
				`{"clusters":[{"fact_ids":["fact_placeholder"]}]}`,
			),
			assistantJSON(
				`{"level":"HIGH","direction":"POSITIVE","horizon":"SHORT_TERM","rationale":"合同可能影响后续收入。","risks":["履约不确定性"],"confidence":0.81}`,
			),
		},
	}

	extractor := &AgenticEventFactExtractor{
		Model: model,
	}
	extraction, err := extractor.Extract(
		context.Background(),
		capevent.ResearchTask{
			Symbol: "600519.SH",
			AsOf:   asOf,
		},
		[]capevent.CanonicalDocument{
			{
				ID:      "doc_1",
				Title:   "公告",
				Content: "公司披露签订合同。",
				Source: capevent.DocumentSource{
					Publisher:  "issuer",
					IsPrimary:  true,
				},
				AvailableAt: &available,
			},
		},
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(extraction.Facts) != 1 ||
		extraction.Facts[0].Type != EventContract {
		t.Fatalf(
			"extraction = %+v",
			extraction,
		)
	}

	// Clusterer is tested with a real deterministic fact ID.
	facts, err := normalizeExtractedFacts(
		extraction.Facts,
		map[string]capevent.CanonicalDocument{
			"doc_1": {
				ID:      "doc_1",
				Content: "公司披露签订合同。",
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"normalizeExtractedFacts() error = %v",
			err,
		)
	}
	model.responses[1] = assistantJSON(
		`{"clusters":[{"fact_ids":["` +
			facts[0].ID +
			`"]}]}`,
	)

	clusterer := &AgenticEventClusterer{
		Model: model,
	}
	clusters, err := clusterer.Cluster(
		context.Background(),
		capevent.ResearchTask{
			Symbol: "600519.SH",
			AsOf:   asOf,
		},
		facts,
	)
	if err != nil {
		t.Fatalf("Cluster() error = %v", err)
	}
	if len(clusters.Clusters) != 1 ||
		len(clusters.Clusters[0].FactIDs) != 1 {
		t.Fatalf(
			"clusters = %+v",
			clusters,
		)
	}

	materialityAnalyzer :=
		&AgenticEventMaterialityAnalyzer{
			Model: model,
		}
	materiality, err :=
		materialityAnalyzer.AnalyzeMateriality(
			context.Background(),
			capevent.ResearchTask{
				Symbol: "600519.SH",
				AsOf:   asOf,
			},
			MaterialityInput{
				EventID: "event_1",
				Type:    EventContract,
				Facts: facts,
				Contradictions: []ClaimContradiction{
					{
						ID:   "conflict_test",
						Type: ContradictionValue,
						FactIDs: []string{
							facts[0].ID,
							"fact_other",
						},
						Rationale: "same metric has incompatible values",
					},
				},
				EvidenceSupport: capevent.EvidenceSupport{
					DocumentCount:            1,
					IndependentEvidenceCount: 1,
					IndependentSourceCount:   1,
					PrimarySourcePresent:     true,
					Authorities: []capevent.EvidenceAuthority{
						capevent.AuthorityIssuer,
					},
					EvidenceKinds: []capevent.EvidenceKind{
						capevent.EvidenceDisclosure,
					},
					SupportClass: capevent.
						EvidenceSupportPrimaryOnly,
				},
			},
		)
	if err != nil {
		t.Fatalf(
			"AnalyzeMateriality() error = %v",
			err,
		)
	}
	if materiality.Level != MaterialityHigh ||
		materiality.Direction !=
			DirectionPositive {
		t.Fatalf(
			"materiality = %+v",
			materiality,
		)
	}

	if len(model.inputs) != 3 {
		t.Fatalf(
			"model calls = %d, want 3",
			len(model.inputs),
		)
	}
	for index, input := range model.inputs {
		if len(input) != 2 ||
			input[0].Role !=
				schema.AgenticRoleTypeSystem ||
			input[1].Role !=
				schema.AgenticRoleTypeUser {
			t.Fatalf(
				"model input %d = %+v",
				index,
				input,
			)
		}
	}

	materialityPayload := eventAgenticTestText(
		model.inputs[2][1],
	)
	if !strings.Contains(
		materialityPayload,
		`"support_class":"PRIMARY_ONLY"`,
	) ||
		!strings.Contains(
			materialityPayload,
			`"independent_source_count":1`,
		) ||
		!strings.Contains(
			materialityPayload,
			`"quote":"公司披露签订合同。"`,
		) ||
		!strings.Contains(
			materialityPayload,
			`"start_byte":0`,
		) ||
		!strings.Contains(
			materialityPayload,
			`"type":"VALUE_CONFLICT"`,
		) ||
		!strings.Contains(
			materialityPayload,
			`"conflict_test"`,
		) {
		t.Fatalf(
			"materiality payload lacks evidence support: %s",
			materialityPayload,
		)
	}
}

func TestAgenticClaimContradictionDetector(
	t *testing.T,
) {
	model := &eventModel{
		responses: []*schema.AgenticMessage{
			assistantJSON(
				`{"contradictions":[{"type":"VALUE_CONFLICT","fact_ids":["fact_b","fact_a"],"rationale":"same reporting period has incompatible revenue values"}]}`,
			),
		},
	}
	detector := &AgenticClaimContradictionDetector{
		Model: model,
	}
	result, err := detector.DetectContradictions(
		context.Background(),
		capevent.ResearchTask{
			Symbol: "600519.SH",
			AsOf: time.Date(
				2026, 9, 29, 15, 0, 0, 0,
				time.UTC,
			),
		},
		[]Fact{
			{
				ID:      "fact_a",
				Summary: "2026H1 revenue was 100.",
				Groundings: []EvidenceGrounding{
					{
						DocumentID: "doc_a",
						Quote:      "revenue was 100",
						StartByte:  0,
						EndByte:    len("revenue was 100"),
					},
				},
			},
			{
				ID:      "fact_b",
				Summary: "2026H1 revenue was 120.",
				Groundings: []EvidenceGrounding{
					{
						DocumentID: "doc_b",
						Quote:      "revenue was 120",
						StartByte:  0,
						EndByte:    len("revenue was 120"),
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contradictions) != 1 ||
		result.Contradictions[0].Type !=
			ContradictionValue {
		t.Fatalf(
			"contradictions = %+v",
			result.Contradictions,
		)
	}
	payload := eventAgenticTestText(
		model.inputs[0][1],
	)
	if !strings.Contains(
		payload,
		`"quote":"revenue was 100"`,
	) ||
		!strings.Contains(
			payload,
			`"quote":"revenue was 120"`,
	) {
		t.Fatalf(
			"detector payload lacks grounded facts: %s",
			payload,
		)
	}
}

func TestAgenticClaimContradictionDetectorSkipsSingleFact(
	t *testing.T,
) {
	model := &eventModel{}
	result, err := (&AgenticClaimContradictionDetector{
		Model: model,
	}).DetectContradictions(
		context.Background(),
		capevent.ResearchTask{},
		[]Fact{{ID: "fact_a"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contradictions) != 0 ||
		len(model.inputs) != 0 {
		t.Fatalf(
			"result=%+v model_calls=%d",
			result,
			len(model.inputs),
		)
	}
}

func TestAgenticFactExtractorRepairsAmbiguousGroundingOnce(
	t *testing.T,
) {
	model := &eventModel{
		responses: []*schema.AgenticMessage{
			assistantJSON(
				`{"facts":[{"type":"CONTRACT","headline":"合同","summary":"公司签署合同。","evidence_quotes":[{"document_id":"doc_1","quote":"合同"}],"confidence":0.9}],"confidence":0.9}`,
			),
			assistantJSON(
				`{"facts":[{"type":"CONTRACT","headline":"合同","summary":"公司签署合同。","evidence_quotes":[{"document_id":"doc_1","quote":"公司签署合同"}],"confidence":0.9}],"confidence":0.9}`,
			),
		},
	}

	result, err := (&AgenticEventFactExtractor{
		Model: model,
	}).Extract(
		context.Background(),
		capevent.ResearchTask{
			Symbol: "600519.SH",
			AsOf: time.Date(
				2026, 9, 29, 15, 0, 0, 0,
				time.UTC,
			),
		},
		[]capevent.CanonicalDocument{
			{
				ID: "doc_1",
				Content: "合同公告：公司签署合同，合同开始履行。",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.inputs) != 2 {
		t.Fatalf(
			"model calls = %d, want initial + one repair",
			len(model.inputs),
		)
	}
	if len(result.Facts) != 1 ||
		len(result.Facts[0].EvidenceQuotes) != 1 ||
		result.Facts[0].EvidenceQuotes[0].Quote !=
			"公司签署合同" {
		t.Fatalf(
			"repaired extraction = %+v",
			result,
		)
	}
}

func TestAgenticStagesRejectNonStrictJSON(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name: "unknown field",
			response: `{"facts":[],"confidence":1,"unknown":true}`,
			want: "unknown field",
		},
		{
			name: "legacy evidence document ids",
			response: `{"facts":[{"type":"OTHER","headline":"fact","summary":"fact","evidence_document_ids":["doc_1"],"confidence":1}],"confidence":1}`,
			want: "unknown field",
		},
		{
			name: "trailing json",
			response: `{"facts":[],"confidence":1} {"facts":[],"confidence":1}`,
			want: "multiple JSON values",
		},
		{
			name:     "invalid json",
			response: "not-json",
			want:     "decode strict JSON",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extractor := &AgenticEventFactExtractor{
				Model: &eventModel{
					responses: []*schema.AgenticMessage{
						assistantJSON(
							test.response,
						),
					},
				},
			}

			_, err := extractor.Extract(
				context.Background(),
				capevent.ResearchTask{
					Symbol: "600519.SH",
					AsOf: time.Date(
						2026, 9, 29,
						15, 0, 0, 0,
						time.UTC,
					),
				},
				nil,
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

func TestAgenticStagesPropagateModelError(t *testing.T) {
	extractor := &AgenticEventFactExtractor{
		Model: &eventModel{
			err: errors.New("provider unavailable"),
		},
	}

	_, err := extractor.Extract(
		context.Background(),
		capevent.ResearchTask{
			Symbol: "600519.SH",
			AsOf: time.Date(
				2026, 9, 29,
				15, 0, 0, 0,
				time.UTC,
			),
		},
		nil,
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

func eventAgenticTestText(
	message *schema.AgenticMessage,
) string {
	if message == nil {
		return ""
	}
	var builder strings.Builder
	for _, block := range message.ContentBlocks {
		if block == nil {
			continue
		}
		if block.UserInputText != nil {
			builder.WriteString(
				block.UserInputText.Text,
			)
		}
	}
	return builder.String()
}
