// Package application assembles the same live model/data/agent dependencies
// for all entry points. No user financial state or model output is synthesized.
// This factory only constructs the workflows; actual execution is performed
// by ResearchService.Run after the caller validates its input.
package application

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/agent/fundamental"
	"Aegis/internal/agent/market"
	portfolioagent "Aegis/internal/agent/portfolio"
	riskagent "Aegis/internal/agent/risk"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	eventcap "Aegis/internal/capability/event"
	fundamentalcap "Aegis/internal/capability/fundamental"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/portfolio"
	"Aegis/internal/research"
	"Aegis/internal/research/debate"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/planning"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/runtime"

	"github.com/cloudwego/eino/components/tool"
)

// LiveServiceOptions configures deployment dependencies and caller-authored
// research-only risk policies. It never provides account or market facts.
type LiveServiceOptions struct {
	ConfigPath string
	SkillsDir string
	TimeZone string
	EvidenceStore string
	EventLookback time.Duration
	WithFundamental bool
	PortfolioReview bool
	PortfolioMaxAge time.Duration
	Limits *portfolio.Limits
	Scenario *portfolio.TradeScenario
	Credentials map[string]string // resolved only on server; no browser payloads
}

// NewLiveResearchService constructs the existing production graph for either
// CLI or future HTTP/worker callers. It cannot submit actual trades.
func NewLiveResearchService(ctx context.Context, options LiveServiceOptions) (*ResearchService, error) {
	if ctx == nil { return nil, fmt.Errorf("live research context is nil") }
	if err := ctx.Err(); err != nil {return nil, err}
	if options.EventLookback <= 0 {return nil, fmt.Errorf("event lookback must be positive")}
	if options.PortfolioMaxAge < 0 {return nil, fmt.Errorf("portfolio max age must not be negative")}
	if !options.PortfolioReview && (options.PortfolioMaxAge != 0 || options.Limits != nil || options.Scenario != nil) {
		return nil, fmt.Errorf("portfolio policy requires portfolio review mode")
	}
	if options.Scenario != nil && options.Limits == nil {
		return nil, fmt.Errorf("trade scenario requires current risk limits")
	}
	configPath := options.ConfigPath
	skillsDir := options.SkillsDir
	timeZone := options.TimeZone
	evidenceStore := options.EvidenceStore
	eventLookback := options.EventLookback
	withFundamental := options.WithFundamental
	portfolioReview := options.PortfolioReview
	portfolioMaxAge := options.PortfolioMaxAge
	limits := options.Limits
	scenario := options.Scenario

	appCfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	for profile, modelCfg := range appCfg.Models {
		if value := strings.TrimSpace(options.Credentials[modelCfg.APIKeyEnv]); value != "" {
			modelCfg.APIKey = value
			appCfg.Models[profile] = modelCfg
		}
	}
	models := llm.NewManager(
		appCfg.Models,
	)

	marketProfile, err :=
		appCfg.ModelProfileForAgent("market")
	if err != nil {
		return nil, fmt.Errorf(
			"resolve market model profile: %w",
			err,
		)
	}

	technicalProfile, err :=
		appCfg.ModelProfileForAgent("technical")
	if err != nil {
		return nil, fmt.Errorf(
			"resolve technical model profile: %w",
			err,
		)
	}

	tradingProfile, err :=
		appCfg.ModelProfileForAgent("trading")
	if err != nil {
		return nil, fmt.Errorf(
			"resolve trading model profile: %w",
			err,
		)
	}

	eventProfile, err :=
		appCfg.ModelProfileForAgent("event")
	if err != nil {
		return nil, fmt.Errorf(
			"resolve event model profile: %w",
			err,
		)
	}

	synthesisProfile, err :=
		appCfg.ModelProfileForAgent("synthesis")
	if err != nil {
		return nil, fmt.Errorf(
			"resolve synthesis model profile: %w",
			err,
		)
	}

	marketModel, err := models.Get(
		ctx,
		marketProfile,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create market model: %w",
			err,
		)
	}

	technicalModel, err := models.Get(
		ctx,
		technicalProfile,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create technical model: %w",
			err,
		)
	}

	tradingModel, err := models.Get(
		ctx,
		tradingProfile,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create trading model: %w",
			err,
		)
	}

	eventModel, err := models.Get(
		ctx,
		eventProfile,
	)
	if err != nil {
		return nil, fmt.Errorf(
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
		return nil, fmt.Errorf(
			"create event planning model: %w",
			err,
		)
	}

	synthesisModel, err := models.Get(
		ctx,
		synthesisProfile,
	)
	if err != nil {
		return nil, fmt.Errorf(
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
		return nil, err
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
		return nil, err
	}

	researcher, err := technicalcap.NewHTTPResearcher(
		credentialValue(options.Credentials, "TUSHARE_TOKEN"),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Tushare HTTP technical researcher: %w",
			err,
		)
	}

	researchTool, err := technicalcap.NewTool(researcher)
	if err != nil {
		return nil, fmt.Errorf(
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
		return nil, err
	}

	tradingAgent, err := trading.New(
		ctx,
		trading.Config{
			Model:         tradingModel,
			MaxIterations: 30,
		},
	)
	if err != nil {
		return nil, err
	}

	if eventLookback <= 0 {
		return nil, fmt.Errorf(
			"event lookback must be positive",
		)
	}

	snapshotStore, err :=
		eventcap.NewFileEvidenceSnapshotStore(
			evidenceStore,
		)
	if err != nil {
		return nil, fmt.Errorf(
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
					Synthesis:
						&eventagent.AgenticEventSynthesisAnalyzer{
							Model: eventModel,
						},
				},
			},
		)
	if err != nil {
		return nil, fmt.Errorf(
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
							APIKey: credentialValue(options.Credentials, "TAVILY_API_KEY"),
							TargetResolver:
								targetResolver,
						},
					},
			},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"create live event agent: %w",
			err,
		)
	}

	researchSynthesizer :=
		&synthesis.AgenticSynthesizer{
			Model: synthesisModel,
		}

	researchConfig := research.Config{
		MarketAgent:    marketAgent,
		TechnicalAgent: technicalAgent,
		EventAgent:     liveEventAgent,
		Synthesizer:    researchSynthesizer,
		TradingAgent:   tradingAgent,
	}

	if withFundamental {
		source := &fundamentalcap.IncomeSource{Token:credentialValue(options.Credentials, "TUSHARE_TOKEN")}
		analyst, err := fundamental.New(ctx,source,tradingModel)
		if err != nil {return nil, fmt.Errorf("create fundamental agent: %w",err)}
		researchConfig.FundamentalAgent=analyst
	}


	var decisionRunner DecisionRunner
	var portfolioRunner PortfolioRunner
	if portfolioReview {
		// Reuse the existing validated research and portfolio workflow; the
		// callers share the same production graph and typed contracts.
		upstream, err := research.NewArtifact(ctx, researchConfig)
		if err != nil {
			return nil, fmt.Errorf("create research artifact workflow: %w", err)
		}
		review, err := research.NewPortfolioReview(research.PortfolioReviewConfig{
			Debate: &debate.Runtime{
				Bull: &debate.BullResearcher{Model: tradingModel},
				Bear: &debate.BearResearcher{Model: tradingModel},
				MaxRounds: 2,
			},
			ResearchManager: &manager.ResearchManager{Model: tradingModel},
			TradingAgent: tradingAgent,
			RiskAnalyst: &riskagent.Analyst{Model: tradingModel},
			PortfolioManager: &portfolioagent.Manager{},
		})
		if err != nil {
			return nil, fmt.Errorf("create portfolio review workflow: %w", err)
		}
		portfolioRunner = &research.FullPortfolioWorkflow{
			Research: upstream,
			Review: review,
			MaxPortfolioAge: portfolioMaxAge,
			Limits: limits,
			Scenario: scenario,
		}
	} else {
		workflow, err := research.New(ctx, researchConfig)
		if err != nil {
			return nil, err
		}
		decisionRunner = workflow
	}
	service, err := NewResearchService(decisionRunner, portfolioRunner)
	if err != nil {
		return nil, err
	}

	// Capture the exact profile bindings used to construct this graph, from
	// the same in-memory appCfg. Loading the config a second time after Run()
	// would risk attributing a changed configuration to an older study.
	roles := []modelRoleSelection{
		{Role: "market", Profile: marketProfile},
		{Role: "technical", Profile: technicalProfile},
		{Role: "event", Profile: eventProfile},
		{Role: "event_planning", Profile: eventProfile},
		{Role: "synthesis", Profile: synthesisProfile},
		{Role: "trading", Profile: tradingProfile},
	}
	if withFundamental {
		roles = append(roles, modelRoleSelection{Role: "fundamental", Profile: tradingProfile})
	}
	if portfolioReview {
		roles = append(roles,
			modelRoleSelection{Role: "bull", Profile: tradingProfile},
			modelRoleSelection{Role: "bear", Profile: tradingProfile},
			modelRoleSelection{Role: "research_manager", Profile: tradingProfile},
			modelRoleSelection{Role: "risk_analyst", Profile: tradingProfile},
		)
	}
	metadata, err := buildModelProvenance(appCfg, roles)
	if err != nil { return nil, fmt.Errorf("build model identity: %w", err) }
	if err := publishModelProvenance(ctx, metadata); err != nil {
		return nil, fmt.Errorf("persist model identity: %w", err)
	}
	return service, nil
}

// credentialValue prefers encrypted per-run settings over startup env values.
func credentialValue(overrides map[string]string, name string) string {
	if value := strings.TrimSpace(overrides[name]); value != "" { return value }
	return strings.TrimSpace(os.Getenv(name))
}
