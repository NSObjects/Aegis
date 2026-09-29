package event

import (
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/research/planning"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type analystPlanningModel struct {
	mu        sync.Mutex
	responses []*schema.Message
	index     int
	tools     []*schema.ToolInfo
}

func (m *analystPlanningModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, errors.New(
		"Generate is not used by PlanExecute tests",
	)
}

func (m *analystPlanningModel) Stream(
	_ context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.responses) == 0 {
		return nil, errors.New(
			"planning model has no response",
		)
	}
	index := m.index
	if index >= len(m.responses) {
		index = len(m.responses) - 1
	}
	response := m.responses[index]
	m.index++

	return schema.StreamReaderFromArray(
		[]*schema.Message{response},
	), nil
}

func (m *analystPlanningModel) WithTools(
	tools []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	return &analystPlanningModel{
		responses: m.responses,
		index:     m.index,
		tools: append(
			[]*schema.ToolInfo(nil),
			tools...,
		),
	}, nil
}

type analystRetriever struct {
	id     string
	method capevent.AcquisitionMethod
	docs   []capevent.RawDocument
	err    error
}

func (r *analystRetriever) ID() string {
	return r.id
}

func (r *analystRetriever) Method() capevent.AcquisitionMethod {
	return r.method
}

func (r *analystRetriever) Retrieve(
	_ context.Context,
	_ capevent.ResearchTask,
) ([]capevent.RawDocument, error) {
	documents := append(
		[]capevent.RawDocument(nil),
		r.docs...,
	)
	return documents, r.err
}

type analystSelector struct{}

func (analystSelector) Select(
	_ context.Context,
	_ capevent.ResearchTask,
	documents []capevent.RawDocument,
) ([]capevent.EvidenceSelectionDecision, error) {
	result := make(
		[]capevent.EvidenceSelectionDecision,
		0,
		len(documents),
	)
	for _, document := range documents {
		result = append(
			result,
			capevent.EvidenceSelectionDecision{
				RawDocumentID: document.ID,
				Selected:      true,
				Method:        capevent.SelectionDeterministic,
				Confidence:    1,
				Reason:        "test selected",
			},
		)
	}
	return result, nil
}

func TestNewsEventAgentTwoRoundResearchProducesNormalContext(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)

	plannerModel := &analystPlanningModel{
		responses: []*schema.Message{
			analystPlanToolCall(
				"initial-plan",
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
	}
	replannerModel := &analystPlanningModel{
		responses: []*schema.Message{
			analystPlanToolCall(
				"second-plan",
				&planning.ResearchPlan{
					Steps: []planning.ResearchPlanStep{
						{
							ID:    "news",
							Need:  capevent.NeedBreakingNews,
							Scope: capevent.AcquisitionScopeCandidate,
						},
					},
				},
			),
			analystRespondToolCall(
				"done",
				"evidence complete",
			),
		},
	}

	official := &analystRetriever{
		id:     "official",
		method: capevent.AcquisitionAPI,
		docs: []capevent.RawDocument{
			{
				ID:           "official-raw",
				Title:        "issuer contract disclosure",
				Content:      "issuer disclosed a contract",
				URL:          "https://issuer.example/contract",
				Publisher:    "issuer",
				SourceType:   capevent.SourceOfficial,
				Authority:    capevent.AuthorityIssuer,
				EvidenceKind: capevent.EvidenceDisclosure,
				IsPrimary:    true,
				PublishedAt:  &published,
				AvailableAt:  &published,
				RetrievedAt:  published,
			},
		},
	}
	news := &analystRetriever{
		id:     "news",
		method: capevent.AcquisitionAPI,
		docs: []capevent.RawDocument{
			{
				ID:           "news-raw",
				Title:        "media confirms contract",
				Content:      "media independently reported the contract",
				URL:          "https://media.example/contract",
				Publisher:    "media",
				SourceType:   capevent.SourceIndustryMedia,
				Authority:    capevent.AuthorityMedia,
				EvidenceKind: capevent.EvidenceNewsReport,
				PublishedAt:  &published,
				AvailableAt:  &published,
				RetrievedAt:  published,
			},
		},
	}

	extractorCalls := 0
	analyst, err := NewAgent(
		context.Background(),
		Config{
			Planning: planning.PlanExecuteConfig{
				PlannerModel:   plannerModel,
				ReplannerModel: replannerModel,
				Executor: planning.ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{
						Channels: planning.ChannelBindings{
							OfficialDisclosure: official,
							BreakingNews:       news,
							Selector:           analystSelector{},
						},
					},
					Runner: capevent.AcquisitionPlanRunner{
						Now: func() time.Time {
							return asOf
						},
					},
				},
				MaxIterations: 4,
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						_ context.Context,
						_ capevent.ResearchTask,
						documents []capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						extractorCalls++
						if len(documents) != 2 {
							t.Fatalf(
								"event analyzer documents = %d, want cumulative 2",
								len(documents),
							)
						}
						ids := []string{
							documents[0].ID,
							documents[1].ID,
						}
						return FactExtractionResult{
							Confidence: 0.95,
							Facts: []ExtractedFact{
								{
									Type:       EventContract,
									OccurredAt: "2026-09-29T14:00:00Z",
									Headline:   "合同事件",
									Summary:    "公司披露合同，媒体进行了独立确认。",
									EvidenceDocumentIDs: ids,
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
						_ context.Context,
						_ capevent.ResearchTask,
						_ MaterialityInput,
					) (Materiality, error) {
						return Materiality{
							Level:      MaterialityMedium,
							Direction:  DirectionUncertain,
							Horizon:    HorizonShortTerm,
							Rationale:  "合同事实已确认，但影响规模仍需更多经营信息。",
							Confidence: 0.78,
						}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	result, err := analyst.Analyze(
		context.Background(),
		Input{
			Task: completeAnalystTask(asOf),
			Query: "研究近期公司事件",
		},
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if extractorCalls != 1 {
		t.Fatalf(
			"extractor calls = %d, want 1 after research loop completes",
			extractorCalls,
		)
	}
	if result.Status != StatusNormal ||
		len(result.Events) != 1 {
		t.Fatalf(
			"result = %+v",
			result,
		)
	}
	if len(result.Events[0].EvidenceDocumentIDs) != 2 {
		t.Fatalf(
			"event evidence = %v, want both accumulated canonical documents",
			result.Events[0].EvidenceDocumentIDs,
		)
	}
}

func TestNewsEventAgentUnsatisfiedResearchProducesInsufficientEvidence(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)

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
					},
				},
			),
		},
	}
	replannerModel := &analystPlanningModel{
		responses: []*schema.Message{
			analystRespondToolCall(
				"stop",
				"evidence remains incomplete",
			),
		},
	}

	extractorCalled := false
	analyst, err := NewAgent(
		context.Background(),
		Config{
			Planning: planning.PlanExecuteConfig{
				PlannerModel:   plannerModel,
				ReplannerModel: replannerModel,
				Executor: planning.ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{
						Channels: planning.ChannelBindings{
							OfficialDisclosure: &analystRetriever{
								id:     "official",
								method: capevent.AcquisitionAPI,
							},
						},
					},
				},
				MaxIterations: 2,
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						context.Context,
						capevent.ResearchTask,
						[]capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						extractorCalled = true
						return FactExtractionResult{}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analyst.Analyze(
		context.Background(),
		Input{
			Task: completeAnalystTask(asOf),
		},
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != StatusInsufficientEvidence {
		t.Fatalf(
			"status = %q, want %q",
			result.Status,
			StatusInsufficientEvidence,
		)
	}
	if extractorCalled {
		t.Fatal(
			"event understanding must not run when evidence remains incomplete",
		)
	}
}

func TestNewsEventAgentSatisfiedResearchCanProduceNoEvent(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)

	analyst, err := NewAgent(
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
							OfficialDisclosure: &analystRetriever{
								id:     "official",
								method: capevent.AcquisitionAPI,
								docs: []capevent.RawDocument{
									{
										ID:          "official",
										Title:       "routine filing",
										URL:         "https://issuer.example/routine",
										Publisher:   "issuer",
										SourceType:  capevent.SourceOfficial,
										IsPrimary:   true,
										PublishedAt: &published,
										AvailableAt: &published,
										RetrievedAt: published,
									},
								},
							},
						},
					},
				},
				MaxIterations: 2,
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						context.Context,
						capevent.ResearchTask,
						[]capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						return FactExtractionResult{
							Confidence: 0.91,
						}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	task := completeAnalystTask(asOf)
	task.Requirements = capevent.ResearchRequirements{
		PITSafe:              true,
		RequiredSourceTypes:  []capevent.SourceType{capevent.SourceOfficial},
		MinDocuments:         1,
		RequirePrimarySource: true,
	}

	result, err := analyst.Analyze(
		context.Background(),
		Input{Task: task},
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Status != StatusNoEvent ||
		result.Confidence != 0.91 {
		t.Fatalf("result = %+v", result)
	}
}

func TestNewsEventAgentStructuralResearchErrorStopsBeforeAnalyzer(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	extractorCalled := false

	analyst, err := NewAgent(
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
										ID:    "news",
										Need:  capevent.NeedBreakingNews,
										Scope: capevent.AcquisitionScopeCandidate,
									},
								},
							},
						),
					},
				},
				ReplannerModel: &analystPlanningModel{
					responses: []*schema.Message{
						analystRespondToolCall(
							"unexpected",
							"unexpected",
						),
					},
				},
				Executor: planning.ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{},
				},
				MaxIterations: 2,
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						context.Context,
						capevent.ResearchTask,
						[]capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						extractorCalled = true
						return FactExtractionResult{}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = analyst.Analyze(
		context.Background(),
		Input{
			Task: completeAnalystTask(asOf),
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"breaking news channel requires at least one retriever binding",
		) {
		t.Fatalf(
			"error = %v, want compiler structural error",
			err,
		)
	}
	if extractorCalled {
		t.Fatal(
			"event analyzer must not run after structural research error",
		)
	}
}

func TestNewsEventAgentConcurrentCallsKeepArtifactsIsolated(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)

	plan := analystPlanToolCall(
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
	)
	response := analystRespondToolCall(
		"done",
		"complete",
	)

	analyst, err := NewAgent(
		context.Background(),
		Config{
			Planning: planning.PlanExecuteConfig{
				PlannerModel: &statelessPlanningModel{
					response: plan,
				},
				ReplannerModel: &statelessPlanningModel{
					response: response,
				},
				Executor: planning.ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{
						Channels: planning.ChannelBindings{
							OfficialDisclosure: &analystRetriever{
								id:     "official",
								method: capevent.AcquisitionAPI,
								docs: []capevent.RawDocument{
									{
										ID:          "official",
										Title:       "filing",
										URL:         "https://issuer.example/filing",
										Publisher:   "issuer",
										SourceType:  capevent.SourceOfficial,
										IsPrimary:   true,
										PublishedAt: &published,
										AvailableAt: &published,
										RetrievedAt: published,
									},
								},
							},
						},
					},
				},
				MaxIterations: 2,
			},
			Analyzer: Analyzer{
				Extractor: extractorFunc(
					func(
						context.Context,
						capevent.ResearchTask,
						[]capevent.CanonicalDocument,
					) (FactExtractionResult, error) {
						return FactExtractionResult{
							Confidence: 1,
						}, nil
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	symbols := []string{
		"600519.SH",
		"000001.SZ",
	}
	results := make(
		[]*Context,
		len(symbols),
	)
	errs := make(
		[]error,
		len(symbols),
	)

	var wg sync.WaitGroup
	for index, symbol := range symbols {
		wg.Add(1)
		go func(index int, symbol string) {
			defer wg.Done()
			task := completeAnalystTask(asOf)
			task.ID = fmt.Sprintf(
				"task-%d",
				index,
			)
			task.Symbol = symbol
			task.Requirements = capevent.ResearchRequirements{
				PITSafe:              true,
				RequiredSourceTypes:  []capevent.SourceType{capevent.SourceOfficial},
				MinDocuments:         1,
				RequirePrimarySource: true,
			}
			results[index], errs[index] =
				analyst.Analyze(
					context.Background(),
					Input{Task: task},
				)
		}(index, symbol)
	}
	wg.Wait()

	for index := range symbols {
		if errs[index] != nil {
			t.Fatalf(
				"Analyze(%s) error = %v",
				symbols[index],
				errs[index],
			)
		}
		if results[index].Symbol != symbols[index] {
			t.Fatalf(
				"result %d symbol = %q, want %q",
				index,
				results[index].Symbol,
				symbols[index],
			)
		}
		if results[index].Status != StatusNoEvent {
			t.Fatalf(
				"result %d = %+v",
				index,
				results[index],
			)
		}
	}
}

type statelessPlanningModel struct {
	response *schema.Message
}

func (m *statelessPlanningModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, errors.New("not used")
}

func (m *statelessPlanningModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray(
		[]*schema.Message{m.response},
	), nil
}

func (m *statelessPlanningModel) WithTools(
	_ []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	return m, nil
}

func completeAnalystTask(
	asOf time.Time,
) capevent.ResearchTask {
	return capevent.ResearchTask{
		ID:              "event-analysis",
		Symbol:          "600519.SH",
		Objective:       "collect sufficient company event evidence",
		InformationNeed: capevent.NeedEventEvidence,
		AsOf:            asOf,
		Requirements: capevent.ResearchRequirements{
			PITSafe: true,
			RequiredSourceTypes: []capevent.SourceType{
				capevent.SourceOfficial,
				capevent.SourceIndustryMedia,
			},
			MinDocuments:          2,
			MinIndependentSources: 2,
			RequirePrimarySource:  true,
		},
	}
}

func analystPlanToolCall(
	id string,
	plan *planning.ResearchPlan,
) *schema.Message {
	data, err := json.Marshal(plan)
	if err != nil {
		panic(err)
	}
	return schema.AssistantMessage(
		"",
		[]schema.ToolCall{
			{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      planning.ResearchPlanToolInfo.Name,
					Arguments: string(data),
				},
			},
		},
	)
}

func analystRespondToolCall(
	id string,
	response string,
) *schema.Message {
	data, err := json.Marshal(
		map[string]string{
			"response": response,
		},
	)
	if err != nil {
		panic(err)
	}
	return schema.AssistantMessage(
		"",
		[]schema.ToolCall{
			{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "respond",
					Arguments: string(data),
				},
			},
		},
	)
}
