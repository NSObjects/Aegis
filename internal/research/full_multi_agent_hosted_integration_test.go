package research

import (
	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	portfolioagent "Aegis/internal/agent/portfolio"
	riskagent "Aegis/internal/agent/risk"
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/risk"
	eventcap "Aegis/internal/capability/event"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/decision"
	"Aegis/internal/llm"
	"Aegis/internal/research/debate"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/planning"
	"Aegis/internal/research/synthesis"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

func TestWorkflow_Run_FullMultiAgentHostedIntegration(t *testing.T) {
	if os.Getenv("AEGIS_FULL_MULTI_AGENT_E2E") != "1" {
		t.Skip("set AEGIS_FULL_MULTI_AGENT_E2E=1 to run full multi-agent hosted E2E")
	}
	for _, name := range []string{"DEEPSEEK_API_KEY", "TUSHARE_TOKEN", "TAVILY_API_KEY"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("%s is required", name)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	modelName := envOrDefault("DEEPSEEK_MODEL", "deepseek-chat")
	strongName := envOrDefault("DEEPSEEK_STRONG_MODEL", modelName)
	cfg := config.Config{
		DefaultModel: "default",
		Models: map[string]config.ModelConfig{
			"default": {
				Provider: config.ProviderDeepSeek,
				Model: modelName,
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL: os.Getenv("DEEPSEEK_BASE_URL"),
			},
			"strong": {
				Provider: config.ProviderDeepSeek,
				Model: strongName,
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL: os.Getenv("DEEPSEEK_BASE_URL"),
			},
		},
	}

	models := llm.NewManager(cfg.Models)
	defaultModel, err := models.Get(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	strongModel, err := models.Get(ctx, "strong")
	if err != nil {
		t.Fatal(err)
	}
	planningModel, err := models.GetToolCalling(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}

	researcher, err := technicalcap.NewHTTPResearcher(os.Getenv("TUSHARE_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	researchTool, err := technicalcap.NewTool(researcher)
	if err != nil {
		t.Fatal(err)
	}
	technicalAgent, err := technical.New(ctx, technical.Config{
		Model: defaultModel,
		Tools: []tool.BaseTool{researchTool},
		MaxIterations: 30,
	})
	if err != nil {
		t.Fatal(err)
	}

	store, err := eventcap.NewFileEvidenceSnapshotStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resolver := &eventcap.CninfoEvidenceTargetResolver{}
	selector := &eventcap.HybridEvidenceSelector{
		TargetResolver: resolver,
		SemanticMatcher: &eventcap.AgenticSemanticEvidenceMatcher{Model: defaultModel},
	}
	officialSnapshots := &eventcap.SnapshotRetriever{
		Store: store,
		Name: "multi_agent_snapshot_official",
		SourceTypes: []eventcap.SourceType{eventcap.SourceOfficial},
	}
	breakingSnapshots := &eventcap.SnapshotRetriever{
		Store: store,
		Name: "multi_agent_snapshot_breaking",
		SourceTypes: []eventcap.SourceType{
			eventcap.SourceIndustryMedia,
			eventcap.SourceMainstreamMedia,
			eventcap.SourceOther,
		},
	}
	analysis, err := eventagent.NewAgent(ctx, eventagent.Config{
		Planning: planning.PlanExecuteConfig{
			PlannerModel: planningModel,
			ReplannerModel: planningModel,
			Executor: planning.ResearchRoundExecutorConfig{
				Compiler: planning.Compiler{
					Channels: planning.ChannelBindings{
						OfficialDisclosureProviders: []eventcap.Retriever{officialSnapshots},
						BreakingNewsProviders: []eventcap.Retriever{breakingSnapshots},
						Selector: selector,
					},
				},
				Runner: eventcap.AcquisitionPlanRunner{},
			},
			MaxIterations: 4,
		},
		Analyzer: eventagent.Analyzer{
			Extractor: &eventagent.AgenticEventFactExtractor{Model: defaultModel},
			Clusterer: &eventagent.AgenticEventClusterer{Model: defaultModel},
			Contradiction: &eventagent.AgenticEventContradictionDetector{Model: defaultModel},
			Evolution: &eventagent.AgenticEventFactEvolutionDetector{Model: defaultModel},
			Impact: &eventagent.AgenticEventImpactMechanismAnalyzer{Model: defaultModel},
			Materiality: &eventagent.AgenticEventMaterialityAnalyzer{Model: defaultModel},
			Synthesis: &eventagent.AgenticEventSynthesisAnalyzer{Model: defaultModel},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	liveEvent, err := eventagent.NewLiveAgent(eventagent.LiveAgentConfig{
		Analysis: analysis,
		CaptureRunner: eventcap.AcquisitionPlanRunner{SnapshotStore: store},
		OfficialRetrievers: []eventcap.Retriever{
			&eventcap.CninfoAnnouncementRetriever{},
		},
		BreakingRetrievers: []eventcap.Retriever{
			&eventcap.ClsNewsRetriever{MaxDays: 9},
			&eventcap.TavilyWebSearchRetriever{
				APIKey: os.Getenv("TAVILY_API_KEY"),
				TargetResolver: resolver,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().UTC()
	symbol := "600519.SH"

	marketContext := hostedMarketContext()
	technicalContext, err := technicalAgent.Analyze(ctx, technical.Input{
		Symbol: symbol,
		RequestedAsOf: cutoff.Format("20060102"),
	})
	if err != nil {
		t.Fatalf("technical research: %v", err)
	}

	eventContext, err := liveEvent.Analyze(ctx, eventagent.Input{
		Task: eventcap.ResearchTask{
			ID: "full-multi-agent-e2e",
			Symbol: symbol,
			Objective: "研究近期公告、新闻与重要事件并形成可追溯事件上下文",
			InformationNeed: eventcap.NeedEventEvidence,
			AsOf: cutoff,
			TimeRange: eventcap.TimeRange{
				Start: cutoff.Add(-7 * 24 * time.Hour),
				End: cutoff,
			},
			Requirements: eventcap.ResearchRequirements{
				PITSafe: true,
				PreferredSourceTypes: []eventcap.SourceType{
					eventcap.SourceOfficial,
					eventcap.SourceIndustryMedia,
					eventcap.SourceMainstreamMedia,
				},
				MinDocuments: 1,
				MinIndependentSources: 1,
			},
		},
		Query: "研究请求时点之前的近期公司公告、新闻与重要事件；结合官方公告、CLS 与 Tavily Web evidence。",
	})
	if err != nil {
		t.Fatalf("event research: %v", err)
	}

	synthesizer := &synthesis.AgenticSynthesizer{Model: defaultModel}
	artifact, err := synthesizer.Synthesize(ctx, synthesis.Input{
		Symbol: symbol,
		Market: marketContext,
		Technical: technicalContext,
		Event: eventContext,
	})
	if err != nil {
		t.Fatalf("research synthesis: %v", err)
	}
	if err := synthesis.Validate(artifact); err != nil {
		t.Fatalf("validate research artifact: %v", err)
	}


	// This branch extends the same real hosted ResearchArtifact with the V2.10
	// Risk / Portfolio stages. The V2.9 path below is left unchanged as baseline.
	if os.Getenv("AEGIS_FULL_RISK_PORTFOLIO_E2E") == "1" {
		tradingAgent, err := trading.New(ctx, trading.Config{
			Model: strongModel,
			MaxIterations: 30,
		})
		if err != nil {
			t.Fatal(err)
		}
		riskAnalyst := &riskagent.Analyst{Model: strongModel}
		portfolioManager := &portfolioagent.Manager{}
		reviewWorkflow, err := NewPortfolioReview(PortfolioReviewConfig{
			Debate: &debate.Runtime{
				Bull: &debate.BullResearcher{Model: strongModel},
				Bear: &debate.BearResearcher{Model: strongModel},
				MaxRounds: 2,
			},
			ResearchManager:  &manager.ResearchManager{Model: strongModel},
			TradingAgent:     tradingAgent,
			RiskAnalyst:      riskAnalyst,
			PortfolioManager: portfolioManager,
		})
		if err != nil {
			t.Fatal(err)
		}

		// An explicitly supplied TEST FIXTURE, never an inferred live account.
		// The portfolio lacks cash, NAV, exposure and risk budgets by contract.
		book := &portfolio.Context{
			State: portfolio.StatePositions,
			AsOf: cutoff.Format(time.RFC3339),
			Positions: []portfolio.Position{{Symbol: symbol, Quantity: 100}},
		}
		result, err := reviewWorkflow.Run(ctx, artifact, book)
		if err != nil {
			t.Fatalf("run full multi-agent risk / portfolio workflow: %v", err)
		}
		if err := portfoliodecision.Validate(
			result.PortfolioDecision, result.TradeIntent, book, result.RiskContext,
		); err != nil {
			t.Fatalf("validate final portfolio decision: %v", err)
		}
		if len(result.Debate.Turns) != 4 {
			t.Fatalf("debate turns = %d, want 4", len(result.Debate.Turns))
		}
		if result.PortfolioDecision.Symbol != symbol {
			t.Fatalf("final symbol = %q, want %q", result.PortfolioDecision.Symbol, symbol)
		}
		if result.PortfolioDecision.Disposition != portfoliodecision.DispositionNoAction &&
			result.PortfolioDecision.Disposition != portfoliodecision.DispositionBlocked &&
			result.PortfolioDecision.Disposition != portfoliodecision.DispositionReviewRequired {
			t.Fatalf("unexpected portfolio disposition %q", result.PortfolioDecision.Disposition)
		}

		// A second *deterministic* downstream check reuses the real TradeIntent.
		// No repeated Research / Debate / Trading or risk LLM call is required.
		missingBook := &portfolio.Context{State: portfolio.StateNotProvided}
		missingRisk, err := riskAnalyst.Review(ctx, result.TradeIntent, missingBook)
		if err != nil {
			t.Fatalf("risk review without portfolio: %v", err)
		}
		if missingRisk.Assessment != risk.AssessmentInsufficientContext {
			t.Fatalf("missing portfolio risk assessment = %q", missingRisk.Assessment)
		}
		missingDecision, err := portfolioManager.Review(
			ctx, result.TradeIntent, missingBook, missingRisk,
		)
		if err != nil {
			t.Fatalf("portfolio decision without portfolio: %v", err)
		}
		if err := portfoliodecision.Validate(
			missingDecision, result.TradeIntent, missingBook, missingRisk,
		); err != nil {
			t.Fatalf("validate missing portfolio decision: %v", err)
		}
		if result.TradeIntent.Status == "CANDIDATE" &&
			missingDecision.Disposition != portfoliodecision.DispositionReviewRequired {
			t.Fatalf("candidate with no portfolio must require review, got %q", missingDecision.Disposition)
		}

		data, err := json.MarshalIndent(struct {
			ObservedPortfolio *portfolio.Context
			ObservedReview    *PortfolioReviewResult
			MissingPortfolio  *portfoliodecision.Decision
		}{
			ObservedPortfolio: book,
			ObservedReview: result,
			MissingPortfolio: missingDecision,
		}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("full risk/portfolio E2E (portfolio is a test fixture):\n%s", data)
		return
	}

	debateRuntime := &debate.Runtime{
		Bull: &debate.BullResearcher{Model: strongModel},
		Bear: &debate.BearResearcher{Model: strongModel},
		MaxRounds: 2,
	}
	debateState, err := debateRuntime.Run(ctx, artifact)
	if err != nil {
		t.Fatalf("run research debate: %v", err)
	}
	if err := debate.ValidateState(debateState, artifact); err != nil {
		t.Fatalf("validate debate state: %v", err)
	}
	if len(debateState.Turns) != 4 {
		t.Fatalf("debate turns = %d, want 4", len(debateState.Turns))
	}

	researchManager := &manager.ResearchManager{Model: strongModel}
	plan, err := researchManager.BuildPlan(ctx, artifact, debateState)
	if err != nil {
		t.Fatalf("build research plan: %v", err)
	}
	if err := manager.Validate(plan, artifact, debateState); err != nil {
		t.Fatalf("validate research plan: %v", err)
	}

	tradingAgent, err := trading.New(ctx, trading.Config{
		Model: strongModel,
		MaxIterations: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tradingAgent.AnalyzeResearchPlan(ctx, artifact, plan)
	if err != nil {
		t.Fatalf("run research-plan trading decision: %v", err)
	}
	if err := decision.Validate(result); err != nil {
		t.Fatalf("validate final decision: %v", err)
	}

	data, err := json.MarshalIndent(struct {
		Synthesis synthesis.Synthesis
		Debate *debate.State
		Plan *manager.ResearchPlan
		Decision *decision.Decision
	}{
		Synthesis: artifact.Synthesis,
		Debate: debateState,
		Plan: plan,
		Decision: result,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("full multi-agent result:\n%s", data)
}
