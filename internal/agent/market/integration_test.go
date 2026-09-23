package market

import (
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

func TestMarketAgent_Analyze_Integration(t *testing.T) {
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
				Model:     envOrDefault(
					"DEEPSEEK_MODEL",
					"deepseek-chat",
				),
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
			},
		},
	}

	profile, err := appCfg.ModelProfileForAgent("market")
	if err != nil {
		t.Fatalf("resolve market model profile: %v", err)
	}

	if profile != "default" {
		t.Fatalf(
			"market model profile = %q, want %q",
			profile,
			"default",
		)
	}

	models := llm.NewManager(appCfg.Models)

	marketModel, err := models.Get(ctx, profile)
	if err != nil {
		t.Fatalf("create market model: %v", err)
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
			Model:         marketModel,
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf("create market agent: %v", err)
	}

	result, err := a.Analyze(
		ctx,
		"分析当前 A 股市场环境。",
	)
	if err != nil {
		t.Fatalf("analyze market: %v", err)
	}

	if result == nil {
		t.Fatal("market context is nil")
	}

	if result.AsOf == "" {
		t.Fatal("market context as_of is empty")
	}

	if len(result.Evidence) == 0 {
		t.Fatal("market context evidence is empty")
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf("marshal market context: %v", err)
	}

	t.Logf(
		"market context:\n%s",
		string(data),
	)
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
