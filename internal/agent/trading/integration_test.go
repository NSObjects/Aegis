package trading

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/runtime"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTradingAgent_Analyze_Integration(t *testing.T) {
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
		3*time.Minute,
	)
	defer cancel()

	skillsDir, err := findSkillsDir()
	if err != nil {
		t.Fatal(err)
	}


	appCfg := config.Config{
		DefaultModel: "default",
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:  config.ProviderDeepSeek,
				Model:     "deepseek-chat",
				APIKeyEnv: "DEEPSEEK_API_KEY",
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

	profile, err := appCfg.ModelProfileForAgent("trading")
	if err != nil {
		t.Fatalf("resolve trading model profile: %v", err)
	}

	if profile != "strong" {
		t.Fatalf(
			"trading model profile = %q, want %q",
			profile,
			"strong",
		)
	}

	models := llm.NewManager(appCfg.Models)

	tradingModel, err := models.Get(ctx, profile)
	if err != nil {
		t.Fatalf("create trading model: %v", err)
	}

	rt, err := runtime.New(
		ctx,
		runtime.Config{
			SkillsDir: skillsDir,
			TimeZone:  "Asia/Shanghai",
		},
	)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	a, err := New(
		ctx,
		Config{
			Model:         tradingModel,
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf("create trading agent: %v", err)
	}

	result, err := a.Analyze(
		ctx,
		Input{
			Symbol:        "600519.SH",
			MarketContext: integrationMarketContext(),
		},
	)
	if err != nil {
		t.Fatalf("analyze trading symbol: %v", err)
	}

	if result == nil {
		t.Fatal("trading decision is nil")
	}

	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"decision symbol = %q, want %q",
			result.Symbol,
			"600519.SH",
		)
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf("marshal trading decision: %v", err)
	}

	t.Logf(
		"trading decision:\n%s",
		string(data),
	)
}

func integrationMarketContext() *market.Context {
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
				Name:          "创业板指",
				ChangePercent: -0.60,
			},
			{
				Name:          "沪深300",
				ChangePercent: -0.60,
			},
			{
				Name:          "中证1000",
				ChangePercent: -0.17,
			},
		},
		AdvanceCount:   1891,
		DeclineCount:   3564,
		LimitUpCount:   51,
		LimitDownCount: 13,
		Turnover:       1_783_128_261_687,
		LeadingSectors: []market.SectorPerformance{
			{
				Name:          "商用载货车",
				ChangePercent: 3.97,
			},
			{
				Name:          "房地产服务",
				ChangePercent: 3.31,
			},
			{
				Name:          "半导体材料",
				ChangePercent: 2.91,
			},
		},
		Evidence: []string{
			"主要宽基指数小幅收跌，市场并非单边风险偏好抬升。",
			"全市场上涨1891家、下跌3564家，市场广度明显偏弱。",
			"成交额约1.78万亿元，较前一交易日缩量约17%。",
			"涨停51家、跌停13家，局部短线情绪仍有活跃度。",
			"电子链和地产链存在结构性热点。",
		},
		Risk: []string{
			"指数跌幅与个股广度存在背离。",
			"主力资金偏流出，结构性行情可能进一步收窄。",
			"市场热点集中，轮动速度较快。",
		},
		Confidence: 0.64,
	}
}

func envOrDefault(
	name string,
	fallback string,
) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func findSkillsDir() (string, error) {
	if dir := os.Getenv("AEGIS_SKILLS_DIR"); dir != "" {
		return filepath.Abs(dir)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goMod := filepath.Join(
			dir,
			"go.mod",
		)

		if _, err := os.Stat(goMod); err == nil {
			return filepath.Join(
				dir,
				"skills",
			), nil
		}

		parent := filepath.Dir(dir)

		if parent == dir {
			return "", os.ErrNotExist
		}

		dir = parent
	}
}

