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
				`{"facts":[{"type":"CONTRACT","occurred_at":"2026-09-29T09:00:00Z","headline":"合同事件","summary":"公司披露签订合同。","entities":["目标公司"],"evidence_document_ids":["doc_1"],"confidence":0.95}],"confidence":0.94}`,
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
				ID: "doc_1",
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
				Facts:   facts,
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
		) {
		t.Fatalf(
			"materiality payload lacks evidence support: %s",
			materialityPayload,
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
