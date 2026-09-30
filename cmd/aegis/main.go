package main

import (
	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	eventcap "Aegis/internal/capability/event"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/research"
	"Aegis/internal/research/planning"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/runtime"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

func main() {
	var (
		configPath  string
		skillsDir   string
		timeZone    string
		symbol      string
		marketQuery   string
		evidenceStore string
		eventLookback time.Duration
	)

	flag.StringVar(
		&configPath,
		"config",
		"configs/config.yaml",
		"application config path",
	)
	flag.StringVar(
		&skillsDir,
		"skills",
		"skills",
		"skills directory",
	)
	flag.StringVar(
		&timeZone,
		"timezone",
		"Asia/Shanghai",
		"runtime timezone",
	)
	flag.StringVar(
		&symbol,
		"symbol",
		"600519.SH",
		"research symbol",
	)
	flag.StringVar(
		&marketQuery,
		"market-query",
		"",
		"optional market research query",
	)
	flag.StringVar(
		&evidenceStore,
		"evidence-store",
		".aegis/evidence",
		"event evidence snapshot store",
	)
	flag.DurationVar(
		&eventLookback,
		"event-lookback",
		7*24*time.Hour,
		"live event publication lookback",
	)
	flag.Parse()

	if err := run(
		context.Background(),
		configPath,
		skillsDir,
		timeZone,
		symbol,
		marketQuery,
		evidenceStore,
		eventLookback,
	); err != nil {
		log.Fatal(err)
	}
}

func run(
	ctx context.Context,
	configPath string,
	skillsDir string,
	timeZone string,
	symbol string,
	marketQuery string,
	evidenceStore string,
	eventLookback time.Duration,
) error {
	appCfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	models := llm.NewManager(
		appCfg.Models,
	)

	marketProfile, err :=
		appCfg.ModelProfileForAgent("market")
	if err != nil {
		return fmt.Errorf(
			"resolve market model profile: %w",
			err,
		)
	}

	technicalProfile, err :=
		appCfg.ModelProfileForAgent("technical")
	if err != nil {
		return fmt.Errorf(
			"resolve technical model profile: %w",
			err,
		)
	}

	tradingProfile, err :=
		appCfg.ModelProfileForAgent("trading")
	if err != nil {
		return fmt.Errorf(
			"resolve trading model profile: %w",
			err,
		)
	}

	eventProfile, err :=
		appCfg.ModelProfileForAgent("event")
	if err != nil {
		return fmt.Errorf(
			"resolve event model profile: %w",
			err,
		)
	}

	synthesisProfile, err :=
		appCfg.ModelProfileForAgent("synthesis")
	if err != nil {
		return fmt.Errorf(
			"resolve synthesis model profile: %w",
			err,
		)
	}

	marketModel, err := models.Get(
		ctx,
		marketProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create market model: %w",
			err,
		)
	}

	technicalModel, err := models.Get(
		ctx,
		technicalProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create technical model: %w",
			err,
		)
	}

	tradingModel, err := models.Get(
		ctx,
		tradingProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create trading model: %w",
			err,
		)
	}

	eventModel, err := models.Get(
		ctx,
		eventProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create event model: %w",
			err,
		)
	}

	eventPlanningModel, err :=
		models.GetToolCalling(
			ctx,
			eventProfile,
		)
	if err != nil {
		return fmt.Errorf(
			"create event planning model: %w",
			err,
		)
	}

	synthesisModel, err := models.Get(
		ctx,
		synthesisProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create synthesis model: %w",
			err,
		)
	}

	rt, err := runtime.New(
		ctx,
		runtime.Config{
			SkillsDir: skillsDir,
			TimeZone:  timeZone,
		},
	)
	if err != nil {
		return err
	}

	marketAgent, err := market.New(
		ctx,
		market.Config{
			Model:         marketModel,
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		return err
	}

	researcher, err := technicalcap.NewScriptResearcher(
		technicalcap.ScriptResearcherConfig{
			ScriptPath: filepath.Join(
				"internal",
				"capability",
				"technical",
				"scripts",
				"stock_research.py",
			),
		},
	)
	if err != nil {
		return fmt.Errorf(
			"create technical researcher: %w",
			err,
		)
	}

	researchTool, err := technicalcap.NewTool(researcher)
	if err != nil {
		return fmt.Errorf(
			"create technical research tool: %w",
			err,
		)
	}

	technicalAgent, err := technical.New(
		ctx,
		technical.Config{
			Model: technicalModel,
			Tools: []tool.BaseTool{
				researchTool,
			},
			MaxIterations: 30,
		},
	)
	if err != nil {
		return err
	}

	tradingAgent, err := trading.New(
		ctx,
		trading.Config{
			Model:         tradingModel,
			MaxIterations: 30,
		},
	)
	if err != nil {
		return err
	}

	if eventLookback <= 0 {
		return fmt.Errorf(
			"event lookback must be positive",
		)
	}

	snapshotStore, err :=
		eventcap.NewFileEvidenceSnapshotStore(
			evidenceStore,
		)
	if err != nil {
		return fmt.Errorf(
			"create event evidence snapshot store: %w",
			err,
		)
	}

	targetResolver :=
		&eventcap.CninfoEvidenceTargetResolver{}
	selector := &eventcap.HybridEvidenceSelector{
		TargetResolver: targetResolver,
		SemanticMatcher:
			&eventcap.AgenticSemanticEvidenceMatcher{
				Model: eventModel,
			},
	}

	officialSnapshots :=
		&eventcap.SnapshotRetriever{
			Store: snapshotStore,
			Name:  "snapshot_official",
			SourceTypes: []eventcap.SourceType{
				eventcap.SourceOfficial,
			},
		}
	breakingSnapshots :=
		&eventcap.SnapshotRetriever{
			Store: snapshotStore,
			Name:  "snapshot_breaking_news",
			SourceTypes: []eventcap.SourceType{
				eventcap.SourceIndustryMedia,
				eventcap.SourceMainstreamMedia,
				eventcap.SourceOther,
			},
		}

	eventAnalysisAgent, err :=
		eventagent.NewAgent(
			ctx,
			eventagent.Config{
				Planning: planning.PlanExecuteConfig{
					PlannerModel:
						eventPlanningModel,
					ReplannerModel:
						eventPlanningModel,
					Executor: planning.
						ResearchRoundExecutorConfig{
						Compiler: planning.Compiler{
							Channels: planning.ChannelBindings{
								OfficialDisclosureProviders:
									[]eventcap.Retriever{
										officialSnapshots,
									},
								BreakingNewsProviders:
									[]eventcap.Retriever{
										breakingSnapshots,
									},
								Selector:
									selector,
							},
						},
						Runner: eventcap.
							AcquisitionPlanRunner{},
					},
					MaxIterations: 4,
				},
				Analyzer: eventagent.Analyzer{
					Extractor:
						&eventagent.AgenticEventFactExtractor{
							Model: eventModel,
						},
					Clusterer:
						&eventagent.AgenticEventClusterer{
							Model: eventModel,
						},
					Contradiction:
						&eventagent.AgenticEventContradictionDetector{
							Model: eventModel,
						},
					Evolution:
						&eventagent.AgenticEventFactEvolutionDetector{
							Model: eventModel,
						},
					Impact:
						&eventagent.AgenticEventImpactMechanismAnalyzer{
							Model: eventModel,
						},
					Materiality:
						&eventagent.AgenticEventMaterialityAnalyzer{
							Model: eventModel,
						},
				},
			},
		)
	if err != nil {
		return fmt.Errorf(
			"create event analysis agent: %w",
			err,
		)
	}

	maxCLSDays :=
		int(eventLookback/(24*time.Hour)) + 2
	if maxCLSDays < 2 {
		maxCLSDays = 2
	}

	liveEventAgent, err :=
		eventagent.NewLiveAgent(
			eventagent.LiveAgentConfig{
				Analysis: eventAnalysisAgent,
				CaptureRunner:
					eventcap.AcquisitionPlanRunner{
						SnapshotStore:
							snapshotStore,
					},
				OfficialRetrievers:
					[]eventcap.Retriever{
						&eventcap.CninfoAnnouncementRetriever{},
					},
				BreakingRetrievers:
					[]eventcap.Retriever{
						&eventcap.ClsNewsRetriever{
							MaxDays: maxCLSDays,
						},
						&eventcap.TavilyWebSearchRetriever{
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
		return fmt.Errorf(
			"create live event agent: %w",
			err,
		)
	}

	researchSynthesizer :=
		&synthesis.AgenticSynthesizer{
			Model: synthesisModel,
		}

	workflow, err := research.New(
		ctx,
		research.Config{
			MarketAgent:    marketAgent,
			TechnicalAgent: technicalAgent,
			EventAgent:     liveEventAgent,
			Synthesizer:    researchSynthesizer,
			TradingAgent:   tradingAgent,
		},
	)
	if err != nil {
		return err
	}

	location, err := time.LoadLocation(
		timeZone,
	)
	if err != nil {
		return fmt.Errorf(
			"load research timezone %q: %w",
			timeZone,
			err,
		)
	}
	eventCutoff := time.Now().In(location)

	workflowInput, err := newLiveResearchInput(
		symbol,
		marketQuery,
		eventCutoff,
		eventLookback,
	)
	if err != nil {
		return err
	}

	result, err := workflow.Run(
		ctx,
		workflowInput,
	)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"marshal research decision: %w",
			err,
		)
	}

	fmt.Println(string(data))

	return nil
}


func newLiveResearchInput(
	symbol string,
	marketQuery string,
	cutoff time.Time,
	lookback time.Duration,
) (research.Input, error) {
	symbol = strings.ToUpper(
		strings.TrimSpace(symbol),
	)
	if symbol == "" {
		return research.Input{}, fmt.Errorf(
			"research symbol is required",
		)
	}
	if cutoff.IsZero() {
		return research.Input{}, fmt.Errorf(
			"event publication cutoff is required",
		)
	}
	if lookback <= 0 {
		return research.Input{}, fmt.Errorf(
			"event lookback must be positive",
		)
	}

	return research.Input{
		Symbol:      symbol,
		MarketQuery: strings.TrimSpace(marketQuery),
		EventInput: eventagent.Input{
			Task: eventcap.ResearchTask{
				ID: fmt.Sprintf(
					"live-event-%s-%d",
					symbol,
					cutoff.UnixNano(),
				),
				Symbol: symbol,
				Objective:
					"研究该标的近期公告、新闻与重要事件，并形成可追溯的事件上下文。",
				InformationNeed:
					eventcap.NeedEventEvidence,
				AsOf: cutoff,
				TimeRange: eventcap.TimeRange{
					Start: cutoff.Add(-lookback),
					End:   cutoff,
				},
				Requirements:
					eventcap.ResearchRequirements{
						PITSafe: true,
						PreferredSourceTypes:
							[]eventcap.SourceType{
								eventcap.SourceOfficial,
								eventcap.SourceIndustryMedia,
								eventcap.SourceMainstreamMedia,
							},
						MinDocuments:          1,
						MinIndependentSources: 1,
					},
			},
			Query:
				"研究请求时点之前的近期公司公告、新闻与重要事件。",
		},
	}, nil
}
