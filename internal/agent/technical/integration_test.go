package technical

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

func TestTechnicalAgent_Analyze_Integration(t *testing.T) {
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
				Provider: config.ProviderDeepSeek,
				Model: envOrDefault(
					"DEEPSEEK_MODEL",
					"deepseek-chat",
				),
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
			},
		},
	}

	profile, err := appCfg.ModelProfileForAgent("technical")
	if err != nil {
		t.Fatalf(
			"resolve technical model profile: %v",
			err,
		)
	}

	if profile != "default" {
		t.Fatalf(
			"technical model profile = %q, want %q",
			profile,
			"default",
		)
	}

	models := llm.NewManager(appCfg.Models)

	technicalModel, err := models.Get(
		ctx,
		profile,
	)
	if err != nil {
		t.Fatalf(
			"create technical model: %v",
			err,
		)
	}

	rt, err := runtime.New(
		ctx,
		runtime.Config{
			SkillsDir: skillsDir,
			TimeZone:  "Asia/Shanghai",
		},
	)
	if err != nil {
		t.Fatalf(
			"create runtime: %v",
			err,
		)
	}

	a, err := New(
		ctx,
		Config{
			Model:         technicalModel,
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf(
			"create technical agent: %v",
			err,
		)
	}

	const (
		symbol        = "600519.SH"
		requestedAsOf = "20260923"
	)

	result, err := a.Analyze(
		ctx,
		Input{
			Symbol:        symbol,
			RequestedAsOf: requestedAsOf,
		},
	)
	if err != nil {
		t.Fatalf(
			"analyze technical symbol: %v",
			err,
		)
	}

	if result == nil {
		t.Fatal("technical context is nil")
	}

	if result.Symbol != symbol {
		t.Fatalf(
			"technical symbol = %q, want %q",
			result.Symbol,
			symbol,
		)
	}

	if result.RequestedAsOf != requestedAsOf {
		t.Fatalf(
			"requested_as_of = %q, want %q",
			result.RequestedAsOf,
			requestedAsOf,
		)
	}

	if err := Validate(result); err != nil {
		t.Fatalf(
			"validate technical context: %v",
			err,
		)
	}

	if result.Status == StatusNormal {
		if result.EffectiveAsOf == "" {
			t.Fatal(
				"effective_as_of is empty for NORMAL status",
			)
		}

		if result.EffectiveAsOf > result.RequestedAsOf {
			t.Fatalf(
				"effective_as_of %q is after requested_as_of %q",
				result.EffectiveAsOf,
				result.RequestedAsOf,
			)
		}

		if result.Snapshot == nil {
			t.Fatal(
				"snapshot is nil for NORMAL status",
			)
		}
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf(
			"marshal technical context: %v",
			err,
		)
	}

	t.Logf(
		"technical context:\n%s",
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
