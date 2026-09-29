package event

import (
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/research/planning"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLiveEventHostedIntegration(t *testing.T) {
	if os.Getenv("AEGIS_HOSTED_EVENT") != "1" {
		t.Skip(
			"set AEGIS_HOSTED_EVENT=1 to run hosted event integration",
		)
	}
	if strings.TrimSpace(
		os.Getenv("DEEPSEEK_API_KEY"),
	) == "" {
		t.Fatal(
			"DEEPSEEK_API_KEY is required for hosted event integration",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Minute,
	)
	defer cancel()

	modelName := strings.TrimSpace(
		os.Getenv("DEEPSEEK_MODEL"),
	)
	if modelName == "" {
		modelName = "deepseek-chat"
	}

	manager := llm.NewManager(
		map[string]config.ModelConfig{
			"event": {
				Provider:  config.ProviderDeepSeek,
				Model:     modelName,
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL: strings.TrimSpace(
					os.Getenv(
						"DEEPSEEK_BASE_URL",
					),
				),
			},
		},
	)

	eventModel, err := manager.Get(
		ctx,
		"event",
	)
	if err != nil {
		t.Fatalf(
			"create hosted event model: %v",
			err,
		)
	}
	planningModel, err := manager.GetToolCalling(
		ctx,
		"event",
	)
	if err != nil {
		t.Fatalf(
			"create hosted event planning model: %v",
			err,
		)
	}

	storeRoot := t.TempDir()
	store, err :=
		capevent.NewFileEvidenceSnapshotStore(
			storeRoot,
		)
	if err != nil {
		t.Fatalf(
			"create hosted snapshot store: %v",
			err,
		)
	}

	targetResolver :=
		&capevent.CninfoEvidenceTargetResolver{}
	selector := &capevent.HybridEvidenceSelector{
		TargetResolver:
			targetResolver,
		SemanticMatcher:
			&capevent.AgenticSemanticEvidenceMatcher{
				Model: eventModel,
			},
	}

	analysis, err := NewAgent(
		ctx,
		Config{
			Planning: planning.PlanExecuteConfig{
				PlannerModel:   planningModel,
				ReplannerModel: planningModel,
				Executor: planning.
					ResearchRoundExecutorConfig{
					Compiler: planning.Compiler{
						Channels: planning.ChannelBindings{
							OfficialDisclosureProviders:
								[]capevent.Retriever{
									&capevent.SnapshotRetriever{
										Store: store,
										Name:
											"hosted_snapshot_official",
										SourceTypes:
											[]capevent.SourceType{
												capevent.SourceOfficial,
											},
									},
								},
							BreakingNewsProviders:
								[]capevent.Retriever{
									&capevent.SnapshotRetriever{
										Store: store,
										Name:
											"hosted_snapshot_breaking_news",
										SourceTypes:
											[]capevent.SourceType{
												capevent.SourceIndustryMedia,
												capevent.SourceMainstreamMedia,
												capevent.SourceOther,
											},
									},
								},
							Selector: selector,
						},
					},
					Runner: capevent.
						AcquisitionPlanRunner{},
				},
				MaxIterations: 4,
			},
			Analyzer: Analyzer{
				Extractor:
					&AgenticEventFactExtractor{
						Model: eventModel,
					},
				Clusterer:
					&AgenticEventClusterer{
						Model: eventModel,
					},
				Contradictions:
					&AgenticClaimContradictionDetector{
						Model: eventModel,
					},
				Materiality:
					&AgenticEventMaterialityAnalyzer{
						Model: eventModel,
					},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"create hosted event analysis agent: %v",
			err,
		)
	}

	lookback := hostedEventLookback(t)
	maxCLSDays :=
		int(lookback/(24*time.Hour)) + 2
	if maxCLSDays < 2 {
		maxCLSDays = 2
	}

	live, err := NewLiveAgent(
		LiveAgentConfig{
			Analysis: analysis,
			CaptureRunner:
				capevent.AcquisitionPlanRunner{
					SnapshotStore: store,
				},
			OfficialRetrievers:
				[]capevent.Retriever{
					&capevent.CninfoAnnouncementRetriever{},
				},
			BreakingRetrievers:
				[]capevent.Retriever{
					&capevent.ClsNewsRetriever{
						MaxDays: maxCLSDays,
					},
					&capevent.TavilyWebSearchRetriever{
						APIKey: os.Getenv(
							"TAVILY_API_KEY",
						),
						TargetResolver:
							targetResolver,
					},
				},
		},
	)
	if err != nil {
		t.Fatalf(
			"create hosted live event agent: %v",
			err,
		)
	}

	symbol := strings.ToUpper(
		strings.TrimSpace(
			os.Getenv(
				"AEGIS_HOSTED_EVENT_SYMBOL",
			),
		),
	)
	if symbol == "" {
		symbol = "600519.SH"
	}

	cutoff := time.Now().UTC()
	result, err := live.Analyze(
		ctx,
		Input{
			Task: capevent.ResearchTask{
				ID: "hosted-event-" +
					strconv.FormatInt(
						cutoff.UnixNano(),
						10,
					),
				Symbol: symbol,
				Objective:
					"研究标的近期公告、新闻与重要事件，验证 hosted News/Event production pipeline。",
				InformationNeed:
					capevent.NeedEventEvidence,
				AsOf: cutoff,
				TimeRange: capevent.TimeRange{
					Start: cutoff.Add(
						-lookback,
					),
					End: cutoff,
				},
				Requirements:
					capevent.ResearchRequirements{
						PITSafe: true,
						PreferredSourceTypes:
							[]capevent.SourceType{
								capevent.SourceOfficial,
								capevent.SourceIndustryMedia,
								capevent.SourceMainstreamMedia,
							},
						MinDocuments:          1,
						MinIndependentSources: 1,
					},
			},
			Query:
				"研究请求时点之前的近期公司公告、新闻与重要事件；优先获取足以判断是否存在公司级事件的最小证据集合。",
		},
	)
	if err != nil {
		t.Fatalf(
			"run hosted News/Event pipeline: %v",
			err,
		)
	}

	if err := Validate(result); err != nil {
		t.Fatalf(
			"validate hosted EventContext: %v",
			err,
		)
	}
	if result.Symbol != symbol {
		t.Fatalf(
			"event context symbol = %q, want %q",
			result.Symbol,
			symbol,
		)
	}

	requestedAsOf, err := time.Parse(
		time.RFC3339,
		result.RequestedAsOf,
	)
	if err != nil {
		t.Fatalf(
			"parse requested_as_of %q: %v",
			result.RequestedAsOf,
			err,
		)
	}
	if !requestedAsOf.Equal(
		cutoff.Truncate(time.Second),
	) {
		t.Fatalf(
			"requested_as_of = %s, want request cutoff %s",
			requestedAsOf,
			cutoff.Truncate(time.Second),
		)
	}

	if strings.TrimSpace(
		result.EvidenceAsOf,
	) == "" {
		t.Fatal(
			"hosted live EventContext must expose evidence_as_of",
		)
	}
	evidenceAsOf, err := time.Parse(
		time.RFC3339,
		result.EvidenceAsOf,
	)
	if err != nil {
		t.Fatalf(
			"parse evidence_as_of %q: %v",
			result.EvidenceAsOf,
			err,
		)
	}
	if evidenceAsOf.Before(requestedAsOf) {
		t.Fatalf(
			"evidence_as_of %s is before requested_as_of %s",
			evidenceAsOf,
			requestedAsOf,
		)
	}

	snapshotCount, err := hostedSnapshotFileCount(
		storeRoot,
	)
	if err != nil {
		t.Fatalf(
			"count hosted snapshots: %v",
			err,
		)
	}
	if snapshotCount == 0 &&
		!hasHostedCaptureFailureRisk(
			result.Risk,
		) {
		t.Fatal(
			"hosted capture produced no snapshots and no concrete capture-failure risk",
		)
	}

	supportClasses := make(
		map[capevent.EvidenceSupportClass]int,
	)
	maxIndependentSources := 0
	maxIndependentEvidence := 0
	groundedFactCount := 0
	groundingSpanCount := 0
	for _, item := range result.Events {
		supportClasses[
			item.EvidenceSupport.SupportClass,
		]++
		if item.EvidenceSupport.
			IndependentSourceCount >
			maxIndependentSources {
			maxIndependentSources =
				item.EvidenceSupport.
					IndependentSourceCount
		}
		if item.EvidenceSupport.
			IndependentEvidenceCount >
			maxIndependentEvidence {
			maxIndependentEvidence =
				item.EvidenceSupport.
					IndependentEvidenceCount
		}
		groundedFactCount += len(item.Facts)
		for _, fact := range item.Facts {
			groundingSpanCount +=
				len(fact.Groundings)
		}
	}

	diagnostic := struct {
		Symbol                 string `json:"symbol"`
		RequestedAsOf          string `json:"requested_as_of"`
		EvidenceAsOf           string `json:"evidence_as_of"`
		Status                 Status `json:"status"`
		EventCount             int    `json:"event_count"`
		EvidenceCount          int    `json:"evidence_count"`
		RiskCount              int    `json:"risk_count"`
		SnapshotCount          int    `json:"snapshot_file_count"`
		SupportClasses         map[capevent.EvidenceSupportClass]int `json:"event_support_classes,omitempty"`
		MaxIndependentSources  int `json:"max_event_independent_source_count"`
		MaxIndependentEvidence int `json:"max_event_independent_evidence_count"`
		GroundedFactCount       int `json:"grounded_fact_count"`
		GroundingSpanCount      int `json:"grounding_span_count"`
	}{
		Symbol:        result.Symbol,
		RequestedAsOf: result.RequestedAsOf,
		EvidenceAsOf:  result.EvidenceAsOf,
		Status:        result.Status,
		EventCount:    len(result.Events),
		EvidenceCount: len(result.Evidence),
		RiskCount:              len(result.Risk),
		SnapshotCount:          snapshotCount,
		SupportClasses:         supportClasses,
		MaxIndependentSources:  maxIndependentSources,
		MaxIndependentEvidence: maxIndependentEvidence,
		GroundedFactCount:       groundedFactCount,
		GroundingSpanCount:      groundingSpanCount,
	}
	data, err := json.MarshalIndent(
		diagnostic,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf(
			"marshal hosted diagnostic: %v",
			err,
		)
	}
	t.Logf(
		"hosted News/Event diagnostic:\n%s",
		string(data),
	)
}

func hostedEventLookback(
	t *testing.T,
) time.Duration {
	t.Helper()

	raw := strings.TrimSpace(
		os.Getenv(
			"AEGIS_HOSTED_EVENT_LOOKBACK",
		),
	)
	if raw == "" {
		return 72 * time.Hour
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		t.Fatalf(
			"parse AEGIS_HOSTED_EVENT_LOOKBACK %q: %v",
			raw,
			err,
		)
	}
	if value <= 0 {
		t.Fatalf(
			"AEGIS_HOSTED_EVENT_LOOKBACK must be positive: %s",
			value,
		)
	}
	return value
}

func hostedSnapshotFileCount(
	root string,
) (int, error) {
	count := 0
	snapshotRoot := filepath.Join(
		root,
		"snapshots",
	)

	err := filepath.WalkDir(
		snapshotRoot,
		func(
			_ string,
			entry os.DirEntry,
			walkErr error,
		) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.EqualFold(
				filepath.Ext(entry.Name()),
				".json",
			) {
				count++
			}
			return nil
		},
	)
	if os.IsNotExist(err) {
		return 0, nil
	}
	return count, err
}

func hasHostedCaptureFailureRisk(
	risks []string,
) bool {
	for _, risk := range risks {
		risk = strings.ToLower(
			strings.TrimSpace(risk),
		)
		if strings.Contains(
			risk,
			"live evidence capture",
		) &&
			(strings.Contains(
				risk,
				"failed",
			) ||
				strings.Contains(
					risk,
					"partial",
				) ||
				strings.Contains(
					risk,
					"timeout",
				)) {
			return true
		}
	}
	return false
}


func TestLiveEventProvidersHostedIntegration(
	t *testing.T,
) {
	if os.Getenv(
		"AEGIS_HOSTED_EVENT_PROVIDERS",
	) != "1" {
		t.Skip(
			"set AEGIS_HOSTED_EVENT_PROVIDERS=1 to run hosted provider integration",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Minute,
	)
	defer cancel()

	symbol := strings.ToUpper(
		strings.TrimSpace(
			os.Getenv(
				"AEGIS_HOSTED_EVENT_SYMBOL",
			),
		),
	)
	if symbol == "" {
		symbol = "600519.SH"
	}
	lookback := hostedEventLookback(t)
	cutoff := time.Now().UTC()

	storeRoot := t.TempDir()
	store, err :=
		capevent.NewFileEvidenceSnapshotStore(
			storeRoot,
		)
	if err != nil {
		t.Fatal(err)
	}

	maxCLSDays :=
		int(lookback/(24*time.Hour)) + 2
	if maxCLSDays < 2 {
		maxCLSDays = 2
	}

	cninfoLookback := lookback
	if cninfoLookback < 180*24*time.Hour {
		cninfoLookback = 180 * 24 * time.Hour
	}

	providerTargetResolver :=
		&capevent.CninfoEvidenceTargetResolver{}

	root := capevent.ResearchTask{
		ID:              "hosted-provider-capture",
		Symbol:          symbol,
		Objective:       "hosted provider verification",
		InformationNeed: capevent.NeedEventEvidence,
		AsOf:            cutoff,
		TimeRange: capevent.TimeRange{
			Start: cutoff.Add(-lookback),
			End:   cutoff,
		},
		Requirements: capevent.ResearchRequirements{
			PITSafe: false,
		},
	}
	output, err := (capevent.AcquisitionPlanRunner{
		SnapshotStore: store,
	}).Run(
		ctx,
		root,
		capevent.AcquisitionPlan{
			Steps: []capevent.AcquisitionStep{
				{
					ID: "hosted_cninfo",
					Scope: capevent.
						AcquisitionScopeDirect,
					Task: capevent.ResearchTask{
						Symbol: symbol,
						InformationNeed:
							capevent.NeedOfficialDisclosure,
						TimeRange: capevent.TimeRange{
							Start: cutoff.Add(
								-cninfoLookback,
							),
							End: cutoff,
						},
					},
					Retriever:
						&capevent.CninfoAnnouncementRetriever{
							PageSize: 3,
							MaxPages: 1,
						},
				},
				{
					ID: "hosted_cls",
					Scope: capevent.
						AcquisitionScopeDirect,
					Task: capevent.ResearchTask{
						InformationNeed:
							capevent.NeedBreakingNews,
					},
					Retriever:
						&capevent.ClsNewsRetriever{
							MaxDays: maxCLSDays,
						},
				},
				{
					ID: "hosted_tavily",
					Scope: capevent.
						AcquisitionScopeDirect,
					Task: capevent.ResearchTask{
						InformationNeed:
							capevent.NeedBreakingNews,
					},
					Retriever:
						&capevent.TavilyWebSearchRetriever{
							APIKey: os.Getenv(
								"TAVILY_API_KEY",
							),
							TargetResolver:
								providerTargetResolver,
							MaxResults: 3,
						},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"run hosted provider capture: %v",
			err,
		)
	}

	statuses := make(
		map[string]capevent.AcquisitionStatus,
		len(output.Steps),
	)
	cninfoDocumentCount := 0
	cninfoEnrichedCount := 0
	cninfoBodyRiskCount := 0
	cninfoResolvedCount := 0
	clsResolvedCount := 0
	tavilyDocumentCount := 0
	tavilyResolvedCount := 0

	for _, step := range output.Steps {
		statuses[step.StepID] =
			step.Run.Attempt.Status

		if step.StepID == "hosted_tavily" {
			tavilyDocumentCount =
				len(step.Run.RawDocuments)
			for _, document :=
				range step.Run.RawDocuments {
				if strings.TrimSpace(
					document.CanonicalDomain,
				) != "" &&
					strings.TrimSpace(
						document.ResolutionReason,
					) != "" {
					tavilyResolvedCount++
				}
			}
		}

		if step.StepID == "hosted_cls" {
			for _, document :=
				range step.Run.RawDocuments {
				if document.CanonicalDomain ==
					"cls.cn" {
					clsResolvedCount++
				}
			}
		}

		if step.StepID == "hosted_cninfo" {
			cninfoDocumentCount =
				len(step.Run.RawDocuments)
			for _, document :=
				range step.Run.RawDocuments {
				if document.CanonicalDomain ==
					"cninfo.com.cn" {
					cninfoResolvedCount++
				}
				enriched, _ :=
					document.RawMetadata["body_enriched"].(bool)
				switch {
				case enriched &&
					strings.TrimSpace(
						document.Content,
					) != "":
					cninfoEnrichedCount++
				case !enriched &&
					strings.TrimSpace(
						fmt.Sprint(
							document.RawMetadata["body_error"],
						),
					) != "":
					cninfoBodyRiskCount++
				}
			}
		}
		switch step.Run.Attempt.Status {
		case capevent.AcquisitionSucceeded,
			capevent.AcquisitionPartial:
		default:
			t.Fatalf(
				"hosted provider %s status=%s error=%q",
				step.StepID,
				step.Run.Attempt.Status,
				step.Run.Attempt.ErrorMessage,
			)
		}
	}

	if cninfoResolvedCount != cninfoDocumentCount {
		t.Fatalf(
			"hosted Cninfo source identity resolved %d/%d documents",
			cninfoResolvedCount,
			cninfoDocumentCount,
		)
	}
	if clsResolvedCount == 0 {
		t.Fatal(
			"hosted CLS source identity did not resolve cls.cn",
		)
	}
	if tavilyResolvedCount != tavilyDocumentCount {
		t.Fatalf(
			"hosted Tavily source identity resolved %d/%d documents",
			tavilyResolvedCount,
			tavilyDocumentCount,
		)
	}

	if len(output.Corroboration.Clusters) == 0 {
		t.Fatal(
			"hosted corroboration produced no evidence clusters",
		)
	}
	if output.Corroboration.IndependentSourceCount == 0 {
		t.Fatal(
			"hosted corroboration produced no independent sources",
		)
	}
	if output.Corroboration.IndependentSourceCount >
		len(output.Canonicalization.Documents) {
		t.Fatalf(
			"hosted independent sources=%d exceeds canonical documents=%d",
			output.Corroboration.IndependentSourceCount,
			len(output.Canonicalization.Documents),
		)
	}

	if tavilyDocumentCount == 0 {
		t.Fatalf(
			"hosted Tavily capture returned no full-content web evidence; status=%s error=%q",
			statuses["hosted_tavily"],
			func() string {
				for _, step := range output.Steps {
					if step.StepID ==
						"hosted_tavily" {
						return step.Run.Attempt.ErrorMessage
					}
				}
				return ""
			}(),
		)
	}

	if cninfoDocumentCount == 0 {
		t.Fatalf(
			"hosted Cninfo capture returned no announcements in %s; cannot verify PDF full-text enrichment",
			cninfoLookback,
		)
	}
	if cninfoEnrichedCount == 0 &&
		cninfoBodyRiskCount == 0 {
		t.Fatal(
			"hosted Cninfo capture produced announcements but neither enriched PDF text nor explicit PDF enrichment risk",
		)
	}

	resolver :=
		&capevent.CninfoEvidenceTargetResolver{}
	target, err := resolver.Resolve(
		ctx,
		capevent.ResearchTask{
			Symbol: symbol,
		},
	)
	if err != nil {
		t.Fatalf(
			"resolve hosted Cninfo target: %v",
			err,
		)
	}
	if target.Symbol != symbol ||
		strings.TrimSpace(target.Name) == "" {
		t.Fatalf(
			"hosted target = %+v",
			target,
		)
	}

	snapshotCount, err := hostedSnapshotFileCount(
		storeRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotCount == 0 {
		t.Fatalf(
			"hosted providers produced no persisted snapshots; statuses=%v",
			statuses,
		)
	}

	diagnostic := struct {
		Symbol              string                    `json:"symbol"`
		TargetName          string                    `json:"target_name"`
		CninfoStatus        capevent.AcquisitionStatus `json:"cninfo_status"`
		CLSStatus           capevent.AcquisitionStatus `json:"cls_status"`
		TavilyStatus        capevent.AcquisitionStatus `json:"tavily_status"`
		TavilyDocuments     int                       `json:"tavily_document_count"`
		TavilyResolved      int                       `json:"tavily_resolved_count"`
		CninfoDocuments     int                       `json:"cninfo_document_count"`
		CninfoResolved      int                       `json:"cninfo_resolved_count"`
		CLSResolved         int                       `json:"cls_resolved_count"`
		CninfoEnriched      int                       `json:"cninfo_enriched_count"`
		CninfoBodyRiskCount int                       `json:"cninfo_body_risk_count"`
		SnapshotCount       int                       `json:"snapshot_file_count"`
		CanonicalDocuments  int                       `json:"canonical_document_count"`
		EvidenceClusters    int                       `json:"evidence_cluster_count"`
		IndependentSources  int                       `json:"independent_source_count"`
		ExactRelations      int                       `json:"exact_reprint_relation_count"`
		NearRelations       int                       `json:"near_reprint_relation_count"`
	}{
		Symbol:              symbol,
		TargetName:          target.Name,
		CninfoStatus:        statuses["hosted_cninfo"],
		CLSStatus:           statuses["hosted_cls"],
		TavilyStatus:        statuses["hosted_tavily"],
		TavilyDocuments:     tavilyDocumentCount,
		TavilyResolved:      tavilyResolvedCount,
		CninfoDocuments:     cninfoDocumentCount,
		CninfoResolved:      cninfoResolvedCount,
		CLSResolved:         clsResolvedCount,
		CninfoEnriched:      cninfoEnrichedCount,
		CninfoBodyRiskCount: cninfoBodyRiskCount,
		SnapshotCount:       snapshotCount,
		CanonicalDocuments:  len(output.Canonicalization.Documents),
		EvidenceClusters:    len(output.Corroboration.Clusters),
		IndependentSources:  output.Corroboration.IndependentSourceCount,
		ExactRelations: func() int {
			count := 0
			for _, relation := range output.Corroboration.Relations {
				if relation.Kind == capevent.EvidenceRelationExactReprint {
					count++
				}
			}
			return count
		}(),
		NearRelations: func() int {
			count := 0
			for _, relation := range output.Corroboration.Relations {
				if relation.Kind == capevent.EvidenceRelationNearReprint {
					count++
				}
			}
			return count
		}(),
	}
	data, err := json.MarshalIndent(
		diagnostic,
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"hosted provider diagnostic:\n%s",
		string(data),
	)
}
