package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

type extractorFunc func(
	context.Context,
	capevent.ResearchTask,
	[]capevent.CanonicalDocument,
) (FactExtractionResult, error)

func (f extractorFunc) Extract(
	ctx context.Context,
	task capevent.ResearchTask,
	documents []capevent.CanonicalDocument,
) (FactExtractionResult, error) {
	return f(ctx, task, documents)
}

type clustererFunc func(
	context.Context,
	capevent.ResearchTask,
	[]Fact,
) (EventClusterResult, error)

func (f clustererFunc) Cluster(
	ctx context.Context,
	task capevent.ResearchTask,
	facts []Fact,
) (EventClusterResult, error) {
	return f(ctx, task, facts)
}

type contradictionDetectorFunc func(
	context.Context,
	capevent.ResearchTask,
	[]Fact,
) (ContradictionDetectionResult, error)

func (f contradictionDetectorFunc) DetectContradictions(
	ctx context.Context,
	task capevent.ResearchTask,
	facts []Fact,
) (ContradictionDetectionResult, error) {
	return f(ctx, task, facts)
}

type noFactEvolutionDetector struct{}

func (noFactEvolutionDetector) DetectFactEvolution(
	context.Context,
	capevent.ResearchTask,
	[]Fact,
) (FactEvolutionDetectionResult, error) {
	return FactEvolutionDetectionResult{Relations: []DetectedFactRelation{}}, nil
}

type noContradictionDetector struct{}

func (noContradictionDetector) DetectContradictions(
	context.Context,
	capevent.ResearchTask,
	[]Fact,
) (ContradictionDetectionResult, error) {
	return ContradictionDetectionResult{
		Contradictions: make(
			[]DetectedContradiction,
			0,
		),
	}, nil
}

type materialityFunc func(
	context.Context,
	capevent.ResearchTask,
	MaterialityInput,
) (Materiality, error)

func (f materialityFunc) AnalyzeMateriality(
	ctx context.Context,
	task capevent.ResearchTask,
	input MaterialityInput,
) (Materiality, error) {
	return f(ctx, task, input)
}

func TestAnalyzerBuildsValidatedEventContext(t *testing.T) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	available := asOf.Add(-time.Hour)

	documents := []capevent.CanonicalDocument{
		{
			ID:      "doc_official",
			Title:   "公司公告",
			Content: "签订重大合同",
			Source: capevent.DocumentSource{
				Publisher: "issuer",
				IsPrimary: true,
			},
			AvailableAt: &available,
		},
		{
			ID:      "doc_media",
			Title:   "媒体报道",
			Content: "报道同一合同",
			Source: capevent.DocumentSource{
				Publisher: "media",
			},
			AvailableAt: &available,
		},
	}

	var capturedFacts []Fact
	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				got []capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				if len(got) != 2 {
					t.Fatalf(
						"extractor documents = %d, want 2",
						len(got),
					)
				}
				return FactExtractionResult{
					Confidence: 0.95,
					Facts: []ExtractedFact{
						{
							Type:       EventContract,
							OccurredAt: "2026-09-29T09:30:00Z",
							Headline:   "签订重大合同",
							Summary:    "公司披露签订重大合同。",
							Entities:   []string{"目标公司"},
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc_official",
									Quote:      "签订重大合同",
								},
							},
							Confidence: 0.96,
						},
						{
							Type:       EventContract,
							OccurredAt: "2026-09-29T09:30:00Z",
							Headline:   "媒体确认合同事件",
							Summary:    "媒体报道同一合同事件。",
							Entities:   []string{"目标公司"},
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc_media",
									Quote:      "报道同一合同",
								},
							},
							Confidence: 0.90,
						},
					},
				}, nil
			},
		),
		Clusterer: clustererFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				facts []Fact,
			) (EventClusterResult, error) {
				capturedFacts = append(
					[]Fact(nil),
					facts...,
				)
				return EventClusterResult{
					Clusters: []EventCluster{
						{
							FactIDs: []string{
								facts[1].ID,
								facts[0].ID,
							},
						},
					},
				}, nil
			},
		),
		Evolution:     noFactEvolutionDetector{},
		Contradiction: noContradictionDetector{},
		Materiality: materialityFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				input MaterialityInput,
			) (Materiality, error) {
				if len(input.Facts) != 2 {
					t.Fatalf(
						"materiality facts = %d, want 2",
						len(input.Facts),
					)
				}
				if input.EvidenceSupport.SupportClass !=
					capevent.EvidenceSupportPrimaryCorroborated {
					t.Fatalf(
						"materiality evidence support = %+v",
						input.EvidenceSupport,
					)
				}
				return Materiality{
					Level:      MaterialityHigh,
					Direction:  DirectionPositive,
					Horizon:    HorizonShortTerm,
					Rationale:  "合同可能影响未来收入确认。",
					Risks:      []string{"合同履约仍存在不确定性"},
					Confidence: 0.82,
				}, nil
			},
		),
	}

	result, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-event",
				Symbol: "600519.SH",
				AsOf:   asOf,
				Requirements: capevent.ResearchRequirements{
					PITSafe: true,
				},
			},
			ResearchStatus: capevent.TaskSatisfied,
			Documents:      documents,
		},
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != StatusNormal {
		t.Fatalf(
			"status = %q, want %q",
			result.Status,
			StatusNormal,
		)
	}
	if len(result.Events) != 1 {
		t.Fatalf(
			"events = %d, want 1",
			len(result.Events),
		)
	}
	item := result.Events[0]
	if !strings.HasPrefix(item.ID, "event_") {
		t.Fatalf("event id = %q", item.ID)
	}
	if item.Type != EventContract {
		t.Fatalf(
			"event type = %q, want %q",
			item.Type,
			EventContract,
		)
	}
	if item.SourceCount != 2 {
		t.Fatalf(
			"source_count = %d, want 2",
			item.SourceCount,
		)
	}
	if !item.HasPrimarySource {
		t.Fatal("event should preserve primary-source provenance")
	}
	if item.EvidenceSupport.IndependentSourceCount != 2 ||
		item.EvidenceSupport.SupportClass !=
			capevent.EvidenceSupportPrimaryCorroborated {
		t.Fatalf(
			"evidence support = %+v",
			item.EvidenceSupport,
		)
	}
	if len(item.EvidenceDocumentIDs) != 2 {
		t.Fatalf(
			"evidence ids = %v",
			item.EvidenceDocumentIDs,
		)
	}
	if len(item.Facts) != 2 ||
		len(item.Facts[0].Groundings) != 1 ||
		len(item.Facts[1].Groundings) != 1 {
		t.Fatalf(
			"grounded event facts = %+v",
			item.Facts,
		)
	}
	for _, fact := range item.Facts {
		for _, grounding := range fact.Groundings {
			document := map[string]capevent.CanonicalDocument{
				"doc_official": documents[0],
				"doc_media":    documents[1],
			}[grounding.DocumentID]
			if document.Content[
				grounding.StartByte:
				grounding.EndByte] != grounding.Quote {
				t.Fatalf(
					"grounding does not round-trip: %+v",
					grounding,
				)
			}
		}
	}
	if item.Materiality.Level != MaterialityHigh {
		t.Fatalf(
			"materiality = %+v",
			item.Materiality,
		)
	}
	if len(capturedFacts) != 2 ||
		capturedFacts[0].ID == "" ||
		capturedFacts[1].ID == "" {
		t.Fatalf(
			"fact IDs were not assigned: %+v",
			capturedFacts,
		)
	}

	// The same factual extraction must produce stable IDs.
	factsAgain, err := normalizeExtractedFacts(
		[]ExtractedFact{
			{
				Type:       EventContract,
				OccurredAt: "2026-09-29T09:30:00Z",
				Headline:   "签订重大合同",
				Summary:    "公司披露签订重大合同。",
				Entities:   []string{"目标公司"},
				EvidenceQuotes: []EvidenceQuote{
					{
						DocumentID: "doc_official",
						Quote:      "签订重大合同",
					},
				},
				Confidence: 0.96,
			},
			{
				Type:       EventContract,
				OccurredAt: "2026-09-29T09:30:00Z",
				Headline:   "媒体确认合同事件",
				Summary:    "媒体报道同一合同事件。",
				Entities:   []string{"目标公司"},
				EvidenceQuotes: []EvidenceQuote{
					{
						DocumentID: "doc_media",
						Quote:      "报道同一合同",
					},
				},
				Confidence: 0.90,
			},
		},
		map[string]capevent.CanonicalDocument{
			"doc_official": documents[0],
			"doc_media":    documents[1],
		},
	)
	if err != nil {
		t.Fatalf("normalizeExtractedFacts() error = %v", err)
	}
	if capturedFacts[0].ID != factsAgain[0].ID ||
		capturedFacts[1].ID != factsAgain[1].ID {
		t.Fatalf(
			"fact IDs are not deterministic: first=%v second=%v",
			capturedFacts,
			factsAgain,
		)
	}
	if deterministicEventID(
		[]string{
			capturedFacts[1].ID,
			capturedFacts[0].ID,
		},
	) != deterministicEventID(
		[]string{
			capturedFacts[0].ID,
			capturedFacts[1].ID,
		},
	) {
		t.Fatal("event ID must be independent of cluster fact order")
	}
}

func TestAnalyzerCarriesContradictionIntoMateriality(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0,
		time.UTC,
	)
	available := asOf.Add(-time.Hour)
	documents := []capevent.CanonicalDocument{
		{
			ID:      "doc_a",
			Content: "合同金额为10亿元。",
			Source: capevent.DocumentSource{
				CanonicalDomain: "a.example",
			},
			AvailableAt: &available,
		},
		{
			ID:      "doc_b",
			Content: "合同金额为12亿元。",
			Source: capevent.DocumentSource{
				CanonicalDomain: "b.example",
			},
			AvailableAt: &available,
		},
	}
	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				return FactExtractionResult{
					Confidence: 0.9,
					Facts: []ExtractedFact{
						{
							Type:       EventContract,
							Headline:   "合同金额",
							Summary:    "合同金额为10亿元。",
							Confidence: 0.9,
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc_a",
									Quote: "合同金额为10亿元。",
								},
							},
						},
						{
							Type:       EventContract,
							Headline:   "合同金额",
							Summary:    "合同金额为12亿元。",
							Confidence: 0.9,
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc_b",
									Quote: "合同金额为12亿元。",
								},
							},
						},
					},
				}, nil
			},
		),
		Clusterer: clustererFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				facts []Fact,
			) (EventClusterResult, error) {
				return EventClusterResult{
					Clusters: []EventCluster{
						{
							FactIDs: []string{
								facts[0].ID,
								facts[1].ID,
							},
						},
					},
				}, nil
			},
		),
		Evolution:     noFactEvolutionDetector{},
		Contradiction: contradictionDetectorFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				facts []Fact,
			) (ContradictionDetectionResult, error) {
				return ContradictionDetectionResult{
					Contradictions: []DetectedContradiction{
						{
							LeftFactID: facts[0].ID,
							RightFactID: facts[1].ID,
							Kind: ContradictionValue,
							Rationale: "同一合同金额不一致",
						},
					},
				}, nil
			},
		),
		Materiality: materialityFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				input MaterialityInput,
			) (Materiality, error) {
				if len(input.Contradictions) != 1 {
					t.Fatalf(
						"materiality contradictions = %+v",
						input.Contradictions,
					)
				}
				if !input.Contradictions[0].
					IndependentEvidenceSides ||
					!input.Contradictions[0].
						IndependentSourceSides {
					t.Fatalf(
						"contradiction provenance = %+v",
						input.Contradictions[0],
					)
				}
				return Materiality{
					Level:      MaterialityMedium,
					Direction:  DirectionUncertain,
					Horizon:    HorizonShortTerm,
					Rationale:  "金额冲突尚未解决。",
					Risks:      []string{"合同金额存在冲突"},
					Confidence: 0.5,
				}, nil
			},
		),
	}
	result, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				Symbol: "600519.SH",
				AsOf:   asOf,
				Requirements: capevent.ResearchRequirements{
					PITSafe: true,
				},
			},
			ResearchStatus: capevent.TaskSatisfied,
			Documents:      documents,
			Corroboration: (capevent.EvidenceCorroborator{}).
				Corroborate(documents),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 ||
		len(result.Events[0].Contradictions) != 1 {
		t.Fatalf("events = %+v", result.Events)
	}
}

func TestAnalyzerNoEventPath(t *testing.T) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)

	clusterCalled := false
	materialityCalled := false
	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				return FactExtractionResult{
					Confidence: 0.88,
				}, nil
			},
		),
		Clusterer: clustererFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]Fact,
			) (EventClusterResult, error) {
				clusterCalled = true
				return EventClusterResult{}, nil
			},
		),
		Evolution:     noFactEvolutionDetector{},
		Contradiction: noContradictionDetector{},
		Materiality: materialityFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				MaterialityInput,
			) (Materiality, error) {
				materialityCalled = true
				return Materiality{}, nil
			},
		),
	}

	result, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-no-event",
				Symbol: "600519.SH",
				AsOf:   asOf,
			},
			ResearchStatus: capevent.TaskSatisfied,
		},
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != StatusNoEvent ||
		result.Confidence != 0.88 {
		t.Fatalf("result = %+v", result)
	}
	if clusterCalled || materialityCalled {
		t.Fatal("NO_EVENT path must not call cluster/materiality stages")
	}
}

func TestAnalyzerInsufficientEvidenceSkipsModels(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	called := false
	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				called = true
				return FactExtractionResult{}, nil
			},
		),
	}

	result, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-partial",
				Symbol: "600519.SH",
				AsOf:   asOf,
			},
			ResearchStatus: capevent.TaskPartial,
		},
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status !=
		StatusInsufficientEvidence {
		t.Fatalf("result = %+v", result)
	}
	if called {
		t.Fatal("insufficient evidence must skip interpretation models")
	}
}

func TestAnalyzerRejectsInvalidFactEvidenceReference(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				return FactExtractionResult{
					Confidence: 1,
					Facts: []ExtractedFact{
						{
							Type:       EventOther,
							Headline:   "fact",
							Summary:    "fact",
							Confidence: 1,
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "unknown",
									Quote:      "fact",
								},
							},
						},
					},
				}, nil
			},
		),
	}

	_, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-invalid-ref",
				Symbol: "600519.SH",
				AsOf:   asOf,
			},
			ResearchStatus: capevent.TaskSatisfied,
			Documents: []capevent.CanonicalDocument{
				{
					ID: "doc_known",
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"unknown canonical document",
		) {
		t.Fatalf(
			"error = %v, want unknown-document error",
			err,
		)
	}
}

func TestAnalyzerRejectsInvalidClusterCoverage(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				return FactExtractionResult{
					Confidence: 1,
					Facts: []ExtractedFact{
						{
							Type:       EventOther,
							Headline:   "one",
							Summary:    "one",
							Confidence: 1,
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc",
									Quote:      "shared evidence",
								},
							},
						},
						{
							Type:       EventOther,
							Headline:   "two",
							Summary:    "two",
							Confidence: 1,
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc",
									Quote:      "shared evidence",
								},
							},
						},
					},
				}, nil
			},
		),
		Clusterer: clustererFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				facts []Fact,
			) (EventClusterResult, error) {
				return EventClusterResult{
					Clusters: []EventCluster{
						{
							FactIDs: []string{
								facts[0].ID,
							},
						},
					},
				}, nil
			},
		),
		Evolution:     noFactEvolutionDetector{},
		Contradiction: noContradictionDetector{},
		Materiality: materialityFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				MaterialityInput,
			) (Materiality, error) {
				return Materiality{}, errors.New(
					"must not be called",
				)
			},
		),
	}

	_, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-cluster",
				Symbol: "600519.SH",
				AsOf:   asOf,
			},
			ResearchStatus: capevent.TaskSatisfied,
			Documents: []capevent.CanonicalDocument{
				{
					ID:      "doc",
					Content: "shared evidence",
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"coverage is missing fact",
		) {
		t.Fatalf(
			"error = %v, want cluster coverage error",
			err,
		)
	}
}

func TestAnalyzerRejectsFuturePITDocument(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	future := asOf.Add(time.Minute)

	_, err := (Analyzer{}).Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-future",
				Symbol: "600519.SH",
				AsOf:   asOf,
				Requirements: capevent.ResearchRequirements{
					PITSafe: true,
				},
			},
			ResearchStatus: capevent.TaskSatisfied,
			Documents: []capevent.CanonicalDocument{
				{
					ID:          "future",
					AvailableAt: &future,
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"available after as_of",
		) {
		t.Fatalf(
			"error = %v, want PIT error",
			err,
		)
	}
}

func TestValidateClustersRequiresExactCoverage(t *testing.T) {
	facts := []Fact{
		{ID: "fact_a"},
		{ID: "fact_b"},
	}

	tests := []struct {
		name     string
		clusters []EventCluster
		want     string
	}{
		{
			name: "unknown fact",
			clusters: []EventCluster{
				{FactIDs: []string{"fact_a", "unknown"}},
				{FactIDs: []string{"fact_b"}},
			},
			want: "unknown fact",
		},
		{
			name: "fact in multiple clusters",
			clusters: []EventCluster{
				{FactIDs: []string{"fact_a"}},
				{FactIDs: []string{"fact_a", "fact_b"}},
			},
			want: "appears in multiple clusters",
		},
		{
			name: "duplicate inside cluster",
			clusters: []EventCluster{
				{FactIDs: []string{"fact_a", "fact_a"}},
				{FactIDs: []string{"fact_b"}},
			},
			want: "contains duplicate fact ids",
		},
		{
			name: "missing fact",
			clusters: []EventCluster{
				{FactIDs: []string{"fact_a"}},
			},
			want: "coverage is missing fact",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateClusters(
				facts,
				test.clusters,
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

func TestAnalyzerRejectsInvalidUpstreamStatus(t *testing.T) {
	_, err := (Analyzer{}).Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-invalid-status",
				Symbol: "600519.SH",
				AsOf: time.Date(
					2026, 9, 29,
					15, 0, 0, 0,
					time.UTC,
				),
			},
			ResearchStatus: capevent.ResearchTaskStatus(
				"unknown",
			),
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"invalid upstream research status",
		) {
		t.Fatalf(
			"error = %v, want invalid-status error",
			err,
		)
	}
}


func TestAnalyzerDoesNotCountReprintsAsIndependentSources(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	available := asOf.Add(-time.Hour)
	body := strings.Repeat(
		"公司公告披露重大合同签署情况以及履约安排",
		10,
	)
	documents := []capevent.CanonicalDocument{
		{
			ID:          "doc_official",
			Content:     body,
			ContentHash: testContentHash(body),
			Source: capevent.DocumentSource{
				Publisher:       "issuer.example",
				CanonicalDomain: "issuer.example",
				Authority:       capevent.AuthorityIssuer,
				EvidenceKind:    capevent.EvidenceDisclosure,
				IsPrimary:       true,
			},
			AvailableAt: &available,
		},
		{
			ID:          "doc_reprint",
			Content:     body,
			ContentHash: testContentHash(body),
			Source: capevent.DocumentSource{
				Publisher:       "media.example",
				CanonicalDomain: "media.example",
				Authority:       capevent.AuthorityMedia,
				EvidenceKind:    capevent.EvidenceNewsReport,
			},
			AvailableAt: &available,
		},
	}

	analyzer := Analyzer{
		Extractor: extractorFunc(
			func(
				context.Context,
				capevent.ResearchTask,
				[]capevent.CanonicalDocument,
			) (FactExtractionResult, error) {
				return FactExtractionResult{
					Confidence: 0.9,
					Facts: []ExtractedFact{
						{
							Type:       EventContract,
							Headline:   "重大合同",
							Summary:    "公司披露重大合同。",
							EvidenceQuotes: []EvidenceQuote{
								{
									DocumentID: "doc_official",
									Quote:      body,
								},
								{
									DocumentID: "doc_reprint",
									Quote:      body,
								},
							},
							Confidence: 0.9,
						},
					},
				}, nil
			},
		),
		Clusterer: clustererFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				facts []Fact,
			) (EventClusterResult, error) {
				return EventClusterResult{
					Clusters: []EventCluster{
						{FactIDs: []string{facts[0].ID}},
					},
				}, nil
			},
		),
		Evolution:     noFactEvolutionDetector{},
		Contradiction: noContradictionDetector{},
		Materiality: materialityFunc(
			func(
				_ context.Context,
				_ capevent.ResearchTask,
				input MaterialityInput,
			) (Materiality, error) {
				if input.EvidenceSupport.
					IndependentSourceCount != 1 ||
					input.EvidenceSupport.
						CollapsedReprintCount != 1 ||
					input.EvidenceSupport.SupportClass !=
						capevent.EvidenceSupportPrimaryOnly {
					t.Fatalf(
						"materiality support = %+v",
						input.EvidenceSupport,
					)
				}
				return Materiality{
					Level:      MaterialityHigh,
					Direction:  DirectionUncertain,
					Horizon:    HorizonShortTerm,
					Rationale:  "合同规模可能具有重要性。",
					Confidence: 0.8,
				}, nil
			},
		),
	}

	result, err := analyzer.Analyze(
		context.Background(),
		AnalyzeInput{
			Task: capevent.ResearchTask{
				ID:     "task-reprint",
				Symbol: "600519.SH",
				AsOf:   asOf,
				Requirements: capevent.ResearchRequirements{
					PITSafe: true,
				},
			},
			ResearchStatus: capevent.TaskSatisfied,
			Documents:      documents,
			Corroboration: (capevent.EvidenceCorroborator{}).
				Corroborate(documents),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 {
		t.Fatalf(
			"events = %d, want 1",
			len(result.Events),
		)
	}
	event := result.Events[0]
	if event.SourceCount != 1 {
		t.Fatalf(
			"source_count = %d, want 1",
			event.SourceCount,
		)
	}
	if event.EvidenceSupport.
		IndependentEvidenceCount != 1 ||
		event.EvidenceSupport.
			CollapsedReprintCount != 1 {
		t.Fatalf(
			"evidence support = %+v",
			event.EvidenceSupport,
		)
	}
}

func testContentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
