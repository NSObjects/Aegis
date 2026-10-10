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
		) ||
		!strings.Contains(
			materialityPayload,
			`"quote":"公司披露签订合同。"`,
		) ||
		!strings.Contains(
			materialityPayload,
			`"start_byte":0`,
		) {
		t.Fatalf(
			"materiality payload lacks evidence support: %s",
			materialityPayload,
		)
	}
}

func TestAgenticContradictionDetectorReturnsFactRelations(
	t *testing.T,
) {
	model := &eventModel{
		responses: []*schema.AgenticMessage{
			assistantJSON(
				`{"contradictions":[{"left_fact_id":"fact_a","right_fact_id":"fact_b","kind":"VALUE","rationale":"同一合同金额分别为10亿元和12亿元，不能同时成立。"}]}`,
			),
		},
	}
	detector := &AgenticEventContradictionDetector{
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
			groundedContradictionFact(
				"fact_a",
				"doc_a",
				"合同金额为10亿元。",
			),
			groundedContradictionFact(
				"fact_b",
				"doc_b",
				"合同金额为12亿元。",
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contradictions) != 1 ||
		result.Contradictions[0].Kind !=
			ContradictionValue {
		t.Fatalf("result = %+v", result)
	}
	if len(model.inputs) != 1 {
		t.Fatalf(
			"model calls = %d, want 1",
			len(model.inputs),
		)
	}
	system := eventAgenticTestText(
		model.inputs[0][0],
	)
	if !strings.Contains(
		system,
		"Do not decide which fact is true",
	) {
		t.Fatalf(
			"contradiction instruction = %s",
			system,
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
	repairPayload := eventAgenticTestText(model.inputs[1][1])
	if !strings.Contains(repairPayload, `"previous_extraction"`) ||
		!strings.Contains(repairPayload, `"quote":"合同"`) ||
		!strings.Contains(repairPayload, `"content":"合同公告：公司签署合同，合同开始履行。"`) {
		t.Fatalf("repair must see previous candidate and canonical source: %s", repairPayload)
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
						assistantJSON(test.response),
						assistantJSON(test.response),
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

func TestAgenticContradictionDetectorRejectsTruthWinnerField(
	t *testing.T,
) {
	detector := &AgenticEventContradictionDetector{
		Model: &eventModel{
			responses: []*schema.AgenticMessage{
				assistantJSON(
					`{"contradictions":[{"left_fact_id":"fact_a","right_fact_id":"fact_b","kind":"VALUE","rationale":"conflict","winner_fact_id":"fact_a"}]}`,
				),
				assistantJSON(
					`{"contradictions":[{"left_fact_id":"fact_a","right_fact_id":"fact_b","kind":"VALUE","rationale":"conflict","winner_fact_id":"fact_a"}]}`,
				),
			},
		},
	}
	_, err := detector.DetectContradictions(
		context.Background(),
		capevent.ResearchTask{
			Symbol: "600519.SH",
			AsOf: time.Date(
				2026, 9, 29, 15, 0, 0, 0,
				time.UTC,
			),
		},
		[]Fact{
			groundedContradictionFact(
				"fact_a", "doc_a", "A",
			),
			groundedContradictionFact(
				"fact_b", "doc_b", "B",
			),
		},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
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

func TestStrictEventJSONRetriesSyntaxErrorOnce(t *testing.T) {
	m := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"level":"LOW","direction":"NEUTRAL",}`),
		assistantJSON(`{"level":"LOW","direction":"NEUTRAL"}`),
	}}
	var result struct {
		Level string `json:"level"`
		Direction string `json:"direction"`
	}
	err := generateStrictAgenticJSON(context.Background(), m,
		"Return {level,direction}", map[string]string{"symbol": "600519.SH"}, &result)
	if err != nil {
		t.Fatalf("generateStrictAgenticJSON error = %v", err)
	}
	if m.index != 2 || result.Level != "LOW" || result.Direction != "NEUTRAL" {
		t.Fatalf("index=%d result=%+v", m.index, result)
	}
	if !strings.Contains(eventAgenticTestText(m.inputs[1][0]), "only the schema fields") {
		t.Fatalf("retry instruction did not emphasize strict schema")
	}
	if eventAgenticTestText(m.inputs[0][1]) != eventAgenticTestText(m.inputs[1][1]) {
		t.Fatal("retry must use the same original payload")
	}
}

func TestStrictEventJSONRetriesUnknownFieldOnce(t *testing.T) {
	m := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"type":"json_object","level":"LOW"}`),
		assistantJSON(`{"level":"LOW"}`),
	}}
	var result struct { Level string `json:"level"` }
	if err := generateStrictAgenticJSON(context.Background(), m, "only level",
		nil, &result); err != nil {
		t.Fatal(err)
	}
	if result.Level != "LOW" || m.index != 2 {
		t.Fatalf("result=%+v, calls=%d", result, m.index)
	}
}

func TestStrictEventJSONDoesNotLeakRejectedAttemptFields(t *testing.T) {
	m := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"value":"stale","unexpected":true}`),
		assistantJSON(`{"count":2}`),
	}}
	var result struct {
		Value string `json:"value"`
		Count int    `json:"count"`
	}
	if err := generateStrictAgenticJSON(context.Background(), m, "format", nil, &result); err != nil {
		t.Fatal(err)
	}
	if result.Value != "" || result.Count != 2 {
		t.Fatalf("rejected first-attempt state leaked: %+v", result)
	}
}

func TestStrictEventJSONFailsClosedAfterTwoErrors(t *testing.T) {
	m := &eventModel{responses: []*schema.AgenticMessage{
		assistantJSON(`{"level":"LOW","type":"json_object"}`),
		assistantJSON(`{"level":"LOW","type":"json_object"}`),
	}}
	var result struct{ Level string `json:"level"` }
	err := generateStrictAgenticJSON(context.Background(), m, "only level", nil, &result)
	if err == nil || !strings.Contains(err.Error(), "unknown field") ||
		!strings.Contains(err.Error(), "after 2 attempts") {
		t.Fatalf("error = %v", err)
	}
	if m.index != 2 || result.Level != "" {
		t.Fatalf("bad output must not be committed: %+v, calls=%d", result, m.index)
	}
}

func TestStrictEventJSONModelErrorDoesNotRetry(t *testing.T) {
	m := &eventModel{err: errors.New("provider failed")}
	var result struct{ Level string `json:"level"` }
	err := generateStrictAgenticJSON(context.Background(), m, "only level", nil, &result)
	if err == nil || !strings.Contains(err.Error(), "provider failed") || len(m.inputs) != 1 {
		t.Fatalf("error=%v, calls=%d", err, len(m.inputs))
	}
}
