package event

import (
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/research/planning"
	"context"
	"encoding/json"
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

	selector := &capevent.HybridEvidenceSelector{
		TargetResolver:
			&capevent.CninfoEvidenceTargetResolver{},
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
							OfficialDisclosure:
								&capevent.SnapshotRetriever{
									Store: store,
									Name:
										"hosted_snapshot_official",
									SourceTypes:
										[]capevent.SourceType{
											capevent.SourceOfficial,
										},
								},
							BreakingNews:
								&capevent.SnapshotRetriever{
									Store: store,
									Name:
										"hosted_snapshot_breaking_news",
									SourceTypes:
										[]capevent.SourceType{
											capevent.SourceIndustryMedia,
											capevent.SourceMainstreamMedia,
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
			OfficialRetriever:
				&capevent.CninfoAnnouncementRetriever{},
			BreakingRetriever:
				&capevent.ClsNewsRetriever{
					MaxDays: maxCLSDays,
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

	diagnostic := struct {
		Symbol          string `json:"symbol"`
		RequestedAsOf   string `json:"requested_as_of"`
		EvidenceAsOf    string `json:"evidence_as_of"`
		Status          Status `json:"status"`
		EventCount      int    `json:"event_count"`
		EvidenceCount   int    `json:"evidence_count"`
		RiskCount       int    `json:"risk_count"`
		SnapshotCount   int    `json:"snapshot_file_count"`
	}{
		Symbol:        result.Symbol,
		RequestedAsOf: result.RequestedAsOf,
		EvidenceAsOf:  result.EvidenceAsOf,
		Status:        result.Status,
		EventCount:    len(result.Events),
		EvidenceCount: len(result.Evidence),
		RiskCount:     len(result.Risk),
		SnapshotCount: snapshotCount,
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
