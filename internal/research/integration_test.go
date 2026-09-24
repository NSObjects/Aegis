package research

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/runtime"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

func TestWorkflow_Run_Integration(t *testing.T) {
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

	skillsDir, err := findSkillsDir()
	if err != nil {
		t.Fatal(err)
	}

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

	marketProfile, err := appCfg.ModelProfileForAgent("market")
	if err != nil {
		t.Fatalf("resolve market model profile: %v", err)
	}

	technicalProfile, err := appCfg.ModelProfileForAgent("technical")
	if err != nil {
		t.Fatalf("resolve technical model profile: %v", err)
	}

	tradingProfile, err := appCfg.ModelProfileForAgent("trading")
	if err != nil {
		t.Fatalf("resolve trading model profile: %v", err)
	}

	if marketProfile != "default" {
		t.Fatalf("market model profile = %q, want %q", marketProfile, "default")
	}
	if technicalProfile != "default" {
		t.Fatalf("technical model profile = %q, want %q", technicalProfile, "default")
	}
	if tradingProfile != "strong" {
		t.Fatalf("trading model profile = %q, want %q", tradingProfile, "strong")
	}

	marketModel, err := models.Get(ctx, marketProfile)
	if err != nil {
		t.Fatalf("create market model: %v", err)
	}

	technicalModel, err := models.Get(ctx, technicalProfile)
	if err != nil {
		t.Fatalf("create technical model: %v", err)
	}

	tradingModel, err := models.Get(ctx, tradingProfile)
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

	marketAgent, err := market.New(
		ctx,
		market.Config{
			Model:         marketModel,
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf("create market agent: %v", err)
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
			Handlers:      rt.Handlers(),
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
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf("create trading agent: %v", err)
	}

	workflow, err := New(
		ctx,
		Config{
			MarketAgent:    marketAgent,
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
		t.Fatalf("run research workflow: %v", err)
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

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal research decision: %v", err)
	}

	t.Logf("research decision:\n%s", string(data))
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

	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "skills"), nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
