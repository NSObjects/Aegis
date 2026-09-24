package research

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/decision"
	"Aegis/internal/llm"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

type hostedMarketAgent struct {
	result *market.Context
}

func (a *hostedMarketAgent) Analyze(
	_ context.Context,
	_ string,
	_ ...adk.AgentRunOption,
) (*market.Context, error) {
	return a.result, nil
}

func TestWorkflow_Run_HostedIntegration(t *testing.T) {
	if os.Getenv("AEGIS_INTEGRATION") != "1" {
		t.Skip("set AEGIS_INTEGRATION=1 to run integration test")
	}

	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Fatal("DEEPSEEK_API_KEY is required")
	}

	if os.Getenv("TUSHARE_TOKEN") == "" {
		t.Fatal("TUSHARE_TOKEN is required")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Minute,
	)
	defer cancel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	appCfg := config.Config{
		DefaultModel: "default",
		Models: map[string]config.ModelConfig{
			"default": {
				Provider: config.ProviderDeepSeek,
				Model: envOrDefault(
					"DEEPSEEK_MODEL",
					"deepseek-chat",
				),
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
			},
			"strong": {
				Provider: config.ProviderDeepSeek,
				Model: envOrDefault(
					"DEEPSEEK_STRONG_MODEL",
					"deepseek-chat",
				),
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
			},
		},
		Agents: map[string]config.AgentConfig{
			"trading": {
				Model: "strong",
			},
		},
	}

	models := llm.NewManager(appCfg.Models)

	technicalProfile, err := appCfg.ModelProfileForAgent("technical")
	if err != nil {
		t.Fatalf("resolve technical model profile: %v", err)
	}

	tradingProfile, err := appCfg.ModelProfileForAgent("trading")
	if err != nil {
		t.Fatalf("resolve trading model profile: %v", err)
	}

	technicalModel, err := models.Get(ctx, technicalProfile)
	if err != nil {
		t.Fatalf("create technical model: %v", err)
	}

	tradingModel, err := models.Get(ctx, tradingProfile)
	if err != nil {
		t.Fatalf("create trading model: %v", err)
	}

	researcher, err := technicalcap.NewScriptResearcher(
		technicalcap.ScriptResearcherConfig{
			ScriptPath: filepath.Join(
				repoRoot,
				"internal",
				"capability",
				"technical",
				"scripts",
				"stock_research.py",
			),
		},
	)
	if err != nil {
		t.Fatalf("create technical researcher: %v", err)
	}

	researchTool, err := technicalcap.NewTool(researcher)
	if err != nil {
		t.Fatalf("create technical research tool: %v", err)
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
		t.Fatalf("create technical agent: %v", err)
	}

	tradingAgent, err := trading.New(
		ctx,
		trading.Config{
			Model:         tradingModel,
			MaxIterations: 10,
		},
	)
	if err != nil {
		t.Fatalf("create trading agent: %v", err)
	}

	workflow, err := New(
		ctx,
		Config{
			MarketAgent: &hostedMarketAgent{
				result: hostedMarketContext(),
			},
			TechnicalAgent: technicalAgent,
			TradingAgent:   tradingAgent,
		},
	)
	if err != nil {
		t.Fatalf("create research workflow: %v", err)
	}

	result, err := workflow.Run(
		ctx,
		Input{
			Symbol: "600519.SH",
		},
	)
	if err != nil {
		t.Fatalf("run hosted research workflow: %v", err)
	}

	if result == nil {
		t.Fatal("research workflow returned nil decision")
	}

	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"decision symbol = %q, want %q",
			result.Symbol,
			"600519.SH",
		)
	}

	if err := decision.Validate(result); err != nil {
		t.Fatalf("validate research decision: %v", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal research decision: %v", err)
	}

	t.Logf(
		"hosted research decision:\n%s",
		string(data),
	)
}

func hostedMarketContext() *market.Context {
	return &market.Context{
		AsOf:   "20260923",
		Regime: market.RegimeMixed,
		Indexes: []market.IndexSnapshot{
			{
				Name:          "上证指数",
				ChangePercent: -0.39,
			},
			{
				Name:          "深证成指",
				ChangePercent: -0.64,
			},
			{
				Name:          "沪深300",
				ChangePercent: -0.60,
			},
		},
		AdvanceCount:   1891,
		DeclineCount:   3564,
		LimitUpCount:   51,
		LimitDownCount: 13,
		Turnover:       1_783_128_261_687,
		LeadingSectors: []market.SectorPerformance{
			{
				Name:          "半导体材料",
				ChangePercent: 2.91,
			},
		},
		Evidence: []string{
			"主要宽基指数小幅收跌，整体市场不是单边强势。",
			"上涨1891家、下跌3564家，市场广度明显偏弱。",
			"涨停51家、跌停13家，局部短线情绪仍有活跃度。",
			"市场存在结构性热点，但热点集中且轮动较快。",
		},
		Risk: []string{
			"指数跌幅与个股广度存在一定背离。",
			"结构性行情可能进一步收窄。",
		},
		Confidence: 0.64,
	}
}
