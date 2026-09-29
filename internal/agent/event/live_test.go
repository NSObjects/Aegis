package event

import (
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/research/planning"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

type liveRetriever struct {
	id     string
	method capevent.AcquisitionMethod
	docs   []capevent.RawDocument
	err    error
}

func (r *liveRetriever) ID() string {
	return r.id
}

func (r *liveRetriever) Method() capevent.AcquisitionMethod {
	return r.method
}

func (r *liveRetriever) Retrieve(
	context.Context,
	capevent.ResearchTask,
) ([]capevent.RawDocument, error) {
	return append(
		[]capevent.RawDocument(nil),
		r.docs...,
	), r.err
}

func TestLiveAgentCapturesThenResearchesFrozenSnapshots(
	t *testing.T,
) {
	cutoff := time.Date(
		2026, 9, 29, 12, 0, 0, 0, time.UTC,
	)
	capturedAt := cutoff.Add(5 * time.Second)
	published := cutoff.Add(-time.Hour)

	store, err := capevent.NewFileEvidenceSnapshotStore(
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}

	officialLive := &liveRetriever{
		id:     "live_official",
		method: capevent.AcquisitionAPI,
		docs: []capevent.RawDocument{
			{
				ID:           "official_raw",
				Title:        "重大合同公告",
				Content:      "公司披露重大合同。",
				URL:          "https://issuer.example/contract",
				Publisher:    "issuer",
				SourceType:   capevent.SourceOfficial,
				Authority:    capevent.AuthorityIssuer,
				EvidenceKind: capevent.EvidenceDisclosure,
				IsPrimary:    true,
				PublishedAt:  &published,
				AvailableAt:  &published,
				RetrievedAt:  capturedAt,
				RawMetadata: map[string]any{
					"symbol": "600519.SH",
				},
			},
		},
	}
	breakingLive := &liveRetriever{
		id:     "live_news",
		method: capevent.AcquisitionAPI,
		docs: []capevent.RawDocument{
			{
				ID:           "news_raw",
				Title:        "媒体报道合同事件",
				Content:      "媒体独立报道同一合同事件。",
				URL:          "https://media.example/contract",
				Publisher:    "media",
				SourceType:   capevent.SourceIndustryMedia,
				Authority:    capevent.AuthorityMedia,
				EvidenceKind: capevent.EvidenceNewsReport,
				PublishedAt:  &published,
				// The body was first observed only during live capture,
				// after the caller's publication cutoff.
				AvailableAt: &capturedAt,
				RetrievedAt: capturedAt,
			},
		},
	}

	plannerModel := &analystPlanningModel{
		responses: []*schema.Message{
			analystPlanToolCall(
				"plan",
				&planning.ResearchPlan{
					Steps: []planning.ResearchPlanStep{
						{
							ID:    "official",
							Need:  capevent.NeedOfficialDisclosure,
							Scope: capevent.AcquisitionScopeDirect,
						},
						{
							ID:    "news",
							Need:  capevent.NeedBreakingNews,
							Scope: capevent.AcquisitionScopeCandidate,
						},
					},
				},
			),
		},
	}
	replannerModel := &analystPlanningModel{
		responses: []*schema.Message{
			analystRespondToolCall(
				"done",
				"evidence complete",
			),
		},
	}

	analysis, err := NewAgent(
		context.Background(),
		Config{
			Planning: planning.PlanExecuteConfig{
				PlannerModel:   plannerModel,
				ReplannerModel: replannerModel,
				Executor: planning.ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{
						Channels: planning.ChannelBindings{
							OfficialDisclosure: &capevent.SnapshotRetriever{
								Store:       store,
								Name:        "snapshot_official",
								SourceTypes: []capevent.SourceType{
									capevent.SourceOfficial,
								},
							},
							BreakingNews: &capevent.SnapshotRetriever{
								Store: store,
								Name:  "snapshot_news",
								SourceTypes: []capevent.SourceType{
									capevent.SourceIndustryMedia,
								},
							},
							Selector: analystSelector{},
						},
					},
					Runner: capevent.AcquisitionPlanRunner{
						Now: func() time.Time {
							return capturedAt
						},
					},
				},
				MaxIterations: 2,
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						_ context.Context,
						task capevent.ResearchTask,
						documents []capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						if !task.AsOf.Equal(
							capturedAt,
						) {
							t.Fatalf(
								"analysis as_of = %s, want capture completion %s",
								task.AsOf,
								capturedAt,
							)
						}
						if len(documents) != 2 {
							t.Fatalf(
								"documents = %d, want 2 captured snapshots",
								len(documents),
							)
						}
						quotes := []EvidenceQuote{
							{
								DocumentID: documents[0].ID,
								Quote:      documents[0].Content,
							},
							{
								DocumentID: documents[1].ID,
								Quote:      documents[1].Content,
							},
						}
						return FactExtractionResult{
							Confidence: 0.95,
							Facts: []ExtractedFact{
								{
									Type:       EventContract,
									Headline:   "合同事件",
									Summary:    "公司披露合同并获得媒体独立报道。",
									EvidenceQuotes: quotes,
									Confidence: 0.94,
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
				Materiality: materialityFunc(
					func(
						context.Context,
						capevent.ResearchTask,
						MaterialityInput,
					) (Materiality, error) {
						return Materiality{
							Level:      MaterialityMedium,
							Direction:  DirectionUncertain,
							Horizon:    HorizonShortTerm,
							Rationale:  "合同事实已确认。",
							Confidence: 0.8,
						}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	live, err := NewLiveAgent(
		LiveAgentConfig{
			Analysis: analysis,
			CaptureRunner: capevent.AcquisitionPlanRunner{
				Now: func() time.Time {
					return capturedAt
				},
				SnapshotStore: store,
			},
			OfficialRetriever: officialLive,
			BreakingRetriever: breakingLive,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	input := Input{
		Task: capevent.ResearchTask{
			ID:              "live-event",
			Symbol:          "600519.SH",
			Objective:       "研究请求时点之前的近期公司事件",
			InformationNeed: capevent.NeedEventEvidence,
			AsOf:            cutoff,
			TimeRange: capevent.TimeRange{
				Start: cutoff.Add(-7 * 24 * time.Hour),
			},
			Requirements: capevent.ResearchRequirements{
				PITSafe:               true,
				MinDocuments:          2,
				MinIndependentSources: 2,
			},
		},
	}
	result, err := live.Analyze(
		context.Background(),
		input,
	)
	if err != nil {
		t.Fatalf(
			"Analyze() error = %v",
			err,
		)
	}
	if result.Status != StatusNormal ||
		len(result.Events) != 1 {
		t.Fatalf(
			"result = %+v",
			result,
		)
	}
	if result.RequestedAsOf !=
		cutoff.UTC().Format(time.RFC3339) {
		t.Fatalf(
			"requested_as_of = %q, want request cutoff %q",
			result.RequestedAsOf,
			cutoff.UTC().Format(time.RFC3339),
		)
	}
	if result.EvidenceAsOf !=
		capturedAt.UTC().Format(time.RFC3339) {
		t.Fatalf(
			"evidence_as_of = %q, want capture completion %q",
			result.EvidenceAsOf,
			capturedAt.UTC().Format(time.RFC3339),
		)
	}
	if !input.Task.AsOf.Equal(cutoff) {
		t.Fatal(
			"live analysis mutated caller-owned task",
		)
	}

	media, err := store.Query(
		context.Background(),
		capevent.EvidenceSnapshotQuery{
			AsOf: capturedAt,
			TimeRange: capevent.TimeRange{
				Start: cutoff.Add(-7 * 24 * time.Hour),
				End:   cutoff,
			},
			SourceTypes: []capevent.SourceType{
				capevent.SourceIndustryMedia,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(media) != 1 ||
		media[0].AvailableAt == nil ||
		!media[0].AvailableAt.Equal(
			capturedAt,
		) {
		t.Fatalf(
			"captured media snapshot = %+v",
			media,
		)
	}
}

func TestLiveAgentSurfacesProviderCaptureFailureAsRisk(
	t *testing.T,
) {
	cutoff := time.Date(
		2026, 9, 29, 12, 0, 0, 0, time.UTC,
	)
	capturedAt := cutoff.Add(time.Second)
	published := cutoff.Add(-time.Hour)

	store, err := capevent.NewFileEvidenceSnapshotStore(
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}

	analysis, err := NewAgent(
		context.Background(),
		Config{
			Planning: planning.PlanExecuteConfig{
				PlannerModel: &analystPlanningModel{
					responses: []*schema.Message{
						analystPlanToolCall(
							"plan",
							&planning.ResearchPlan{
								Steps: []planning.ResearchPlanStep{
									{
										ID:    "official",
										Need:  capevent.NeedOfficialDisclosure,
										Scope: capevent.AcquisitionScopeDirect,
									},
								},
							},
						),
					},
				},
				ReplannerModel: &analystPlanningModel{
					responses: []*schema.Message{
						analystRespondToolCall(
							"done",
							"complete",
						),
					},
				},
				Executor: planning.ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{
						Channels: planning.ChannelBindings{
							OfficialDisclosure: &capevent.SnapshotRetriever{
								Store: store,
								Name:  "snapshot_official",
								SourceTypes: []capevent.SourceType{
									capevent.SourceOfficial,
								},
							},
						},
					},
					Runner: capevent.AcquisitionPlanRunner{
						Now: func() time.Time {
							return capturedAt
						},
					},
				},
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						context.Context,
						capevent.ResearchTask,
						[]capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						return FactExtractionResult{
							Confidence: 0.9,
						}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	live, err := NewLiveAgent(
		LiveAgentConfig{
			Analysis: analysis,
			CaptureRunner: capevent.AcquisitionPlanRunner{
				Now: func() time.Time {
					return capturedAt
				},
				SnapshotStore: store,
			},
			OfficialRetrievers: []capevent.Retriever{
				&liveRetriever{
					id:     "official",
					method: capevent.AcquisitionAPI,
					docs: []capevent.RawDocument{
						{
							ID:          "official",
							Title:       "例行公告",
							URL:         "https://issuer.example/routine",
							Publisher:   "issuer",
							SourceType:  capevent.SourceOfficial,
							IsPrimary:   true,
							PublishedAt: &published,
							AvailableAt: &published,
							RetrievedAt: capturedAt,
							RawMetadata: map[string]any{
								"symbol": "600519.SH",
							},
						},
					},
				},
			},
			BreakingRetrievers: []capevent.Retriever{
				&liveRetriever{
					id:     "news_ok",
					method: capevent.AcquisitionAPI,
					docs: []capevent.RawDocument{
						{
							ID:           "news-ok",
							Title:        "媒体候选证据",
							Content:      "可用的媒体证据",
							URL:          "https://media.example/ok",
							Publisher:    "media-ok",
							SourceType:   capevent.SourceIndustryMedia,
							Authority:    capevent.AuthorityMedia,
							EvidenceKind: capevent.EvidenceNewsReport,
							PublishedAt:  &published,
							AvailableAt:  &published,
							RetrievedAt:  capturedAt,
						},
					},
				},
				&liveRetriever{
					id:     "news_fail",
					method: capevent.AcquisitionWebSearch,
					err: errors.New(
						"news provider unavailable",
					),
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := live.Analyze(
		context.Background(),
		Input{
			Task: capevent.ResearchTask{
				ID:              "capture-risk",
				Symbol:          "600519.SH",
				Objective:       "event research",
				InformationNeed: capevent.NeedEventEvidence,
				AsOf:            cutoff,
				Requirements: capevent.ResearchRequirements{
					PITSafe:      true,
					MinDocuments: 1,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"Analyze() error = %v",
			err,
		)
	}
	found := false
	for _, risk := range result.Risk {
		if strings.Contains(
			risk,
			"capture_breaking_news@news_fail",
		) &&
			strings.Contains(
				risk,
				"news provider unavailable",
			) {
			found = true
		}
	}
	if !found {
		t.Fatalf(
			"capture failure risk missing: %+v",
			result.Risk,
		)
	}

	media, err := store.Query(
		context.Background(),
		capevent.EvidenceSnapshotQuery{
			AsOf: capturedAt,
			SourceTypes: []capevent.SourceType{
				capevent.SourceIndustryMedia,
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"query successful sibling snapshot: %v",
			err,
		)
	}
	if len(media) != 1 ||
		media[0].ID != "news-ok" {
		t.Fatalf(
			"successful sibling snapshots = %+v",
			media,
		)
	}
}
